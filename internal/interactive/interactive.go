// Package interactive runs claude in the terminal and moves to the next
// profile when the current one hits its limit or reaches the usage threshold.
//
// The hooks claude calls (see the hooks package) write a switch request into
// the state file. The runner polls for it, ends claude, decides according to
// the mode (automatically or by asking in the terminal) and starts claude again
// in the next profile, resuming the conversation if wanted.
package interactive

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/LuGG000/switchyard/internal/breaker"
	"github.com/LuGG000/switchyard/internal/config"
	"github.com/LuGG000/switchyard/internal/hooks"
	"github.com/LuGG000/switchyard/internal/launcher"
	"github.com/LuGG000/switchyard/internal/profiles"
	"github.com/LuGG000/switchyard/internal/selector"
	"github.com/LuGG000/switchyard/internal/state"
)

// ColdCacheHint warns that a resumed conversation is processed again by the new account.
const ColdCacheHint = "Note: the new account processes the whole conversation again (cold prompt cache) and it counts against its limit."

// Runner runs claude interactively over a set of profiles.
type Runner struct {
	// Launcher is the template for every launch; its Stop and StopGrace are set per launch.
	Launcher launcher.Launcher
	Store    *state.Store
	// Profiles are the profiles a run may move to.
	Profiles []profiles.Profile
	// Mode is config.ModeAuto or config.ModeAsk.
	Mode string
	// Strategy chooses the next profile, see the selector package.
	Strategy string
	// CarryContext is the default for resuming the conversation in the next profile.
	CarryContext bool
	// Limits bound the automatic switches; over them, automatic mode asks like ask mode.
	Limits breaker.Limits
	// Thresholds keep a profile that is over them from being chosen while another is below.
	Thresholds selector.Thresholds
	// ContinuePrompt is sent as the first message of a resumed conversation. Empty sends nothing.
	ContinuePrompt string
	// Prepare records p as in use and returns the final claude arguments for it.
	Prepare func(p profiles.Profile, args []string) ([]string, error)
	// Reload, if set, returns the current config. It is read at every limit, so a
	// change of mode, strategy or carry_context applies to a running session.
	Reload func() (config.Config, error)
	// Now returns the current time.
	Now func() time.Time
	// Wait blocks until the given time; nil waits on the clock. Tests replace it.
	Wait func(ctx context.Context, until time.Time) error
	// Poll is how often the state is checked for a switch request.
	Poll time.Duration
	// StopGrace is how long claude gets to exit when it is ended.
	StopGrace time.Duration
	// ModWait is how long a limit waits for an answer from the mod's buttons while claude
	// keeps running, before the question moves to the terminal. Zero turns the buttons off.
	ModWait time.Duration
	// ModStale is how old the mod's last poll may be for it to count as present.
	ModStale time.Duration
	// In and Out are the terminal used for questions and notices.
	In  io.Reader
	Out io.Writer

	prompt *bufio.Reader
}

// Run runs claude with args for start. It returns claude's exit code when claude
// ends by itself, 0 when the user quits at a question, and an error if no
// profile is left in automatic mode.
func (r *Runner) Run(ctx context.Context, start profiles.Profile, args []string) (int, error) {
	r.prompt = bufio.NewReader(r.In)
	current, attemptArgs := start, args
	for {
		req, answer, code, err := r.launch(ctx, current, attemptArgs)
		if err != nil || req == nil {
			return code, err
		}
		next, carry, err := r.next(ctx, current, *req, answer)
		if err != nil {
			return 0, err
		}
		if next == nil {
			return 0, nil
		}
		current, attemptArgs = *next, args
		if carry {
			attemptArgs = launcher.ResumeArgs(args, req.SessionID)
			if r.ContinuePrompt != "" {
				attemptArgs = append(attemptArgs, r.ContinuePrompt)
			}
		}
	}
}

// outcome is what ended a launch: the switch request and, if the mod's buttons
// were asked while claude kept running, the answer.
type outcome struct {
	req    state.SwitchRequest
	answer *state.Answer
}

// launch runs claude for p until it exits or a switch request for p ends it. The
// request, and the answer from the mod if there was one, are returned in the
// second case.
func (r *Runner) launch(ctx context.Context, p profiles.Profile, args []string) (*state.SwitchRequest, *state.Answer, int, error) {
	claudeArgs, err := r.Prepare(p, args)
	if err != nil {
		return nil, nil, 0, err
	}
	stop := make(chan struct{})
	l := r.Launcher
	l.Stop, l.StopGrace = stop, r.StopGrace

	since := r.Now()
	watchCtx, cancel := context.WithCancel(ctx)
	found := make(chan outcome, 1)
	var wg sync.WaitGroup
	wg.Go(func() {
		if out, ok := r.watch(watchCtx, p, since); ok {
			found <- out
			close(stop)
		}
	})
	code, err := l.Run(ctx, p, claudeArgs)
	cancel()
	wg.Wait()

	select {
	case out := <-found:
		return &out.req, out.answer, code, err
	default:
		return nil, nil, code, err
	}
}

// watch waits for a switch request for p made after since, clears it and
// returns it. A threshold request is only honored if there is a profile to move
// to, so a working session is not ended for nothing. A limit in ask mode is put
// to the mod's buttons first when a mod is polling; claude keeps running while
// it waits.
func (r *Runner) watch(ctx context.Context, p profiles.Profile, since time.Time) (outcome, bool) {
	ticker := time.NewTicker(r.Poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return outcome{}, false
		case <-ticker.C:
		}
		st, err := r.Store.Read()
		req := st.SwitchRequest
		if err != nil || req == nil || req.Profile != p.Name || req.RequestedAt.Before(since) {
			continue
		}
		if req.Reason == state.ReasonThreshold && !r.hasAlternative(st, p) {
			continue
		}
		var taken *state.SwitchRequest
		_ = r.Store.Update(func(st *state.State) error {
			if st.SwitchRequest != nil && st.SwitchRequest.RequestedAt.Equal(req.RequestedAt) && st.SwitchRequest.Profile == req.Profile {
				taken, st.SwitchRequest = st.SwitchRequest, nil
			}
			return nil
		})
		if taken == nil {
			continue
		}
		answer := r.askMod(ctx, p, *taken, since)
		if ctx.Err() != nil {
			return outcome{}, false
		}
		if answer != nil && answer.Action == state.ActionStay {
			continue
		}
		return outcome{req: *taken, answer: answer}, true
	}
}

// askMod puts a limit to the mod's buttons and waits for the answer. It returns
// nil when there is nobody to ask or nobody answered in time, and the terminal
// question follows then.
func (r *Runner) askMod(ctx context.Context, p profiles.Profile, req state.SwitchRequest, since time.Time) *state.Answer {
	if r.ModWait <= 0 || req.Reason != state.ReasonRateLimit {
		return nil
	}
	r.reload()
	if auto, _ := r.automatic(); auto || !r.modPresent(since) {
		return nil
	}
	st, err := r.Store.Read()
	if err != nil {
		return nil
	}
	now := r.Now()
	var options []string
	for _, other := range r.Profiles {
		if other.Name != p.Name && !st.Profiles[other.Name].CooldownUntil.After(now) {
			options = append(options, other.Name)
		}
	}
	if len(options) == 0 {
		return nil
	}
	asked := state.Decision{Profile: p.Name, Reason: req.Reason, Options: options, Carry: r.CarryContext, AskedAt: now.UTC(), ExpiresAt: now.Add(r.ModWait).UTC()}
	if err := r.Store.Update(func(st *state.State) error { st.Decision = &asked; return nil }); err != nil {
		return nil
	}
	defer func() {
		_ = r.Store.Update(func(st *state.State) error { st.Decision = nil; return nil })
	}()

	ticker := time.NewTicker(r.Poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		st, err := r.Store.Read()
		if err != nil {
			continue
		}
		if st.Decision == nil {
			return nil
		}
		if st.Decision.Answer != nil {
			return st.Decision.Answer
		}
		if !r.Now().Before(asked.ExpiresAt) || !r.modPresent(since) {
			return nil
		}
	}
}

// modPresent reports whether a mod polled recently and since the launch, so its
// buttons can be expected to show a question.
func (r *Runner) modPresent(since time.Time) bool {
	seen := r.Store.HeartbeatAt()
	stale := r.ModStale
	if stale <= 0 {
		stale = 10 * time.Second
	}
	return !seen.Before(since) && r.Now().Sub(seen) <= stale
}

// hasAlternative reports whether a profile other than p is out of cooldown and under the usage thresholds.
func (r *Runner) hasAlternative(st state.State, p profiles.Profile) bool {
	now := r.Now()
	for _, other := range r.Profiles {
		if other.Name != p.Name && !st.Profiles[other.Name].CooldownUntil.After(now) && !r.Thresholds.Over(st.Profiles[other.Name], now) {
			return true
		}
	}
	return false
}

// decide chooses the profile to continue with, and whether to carry the
// conversation over. A nil profile means the user quit.
func (r *Runner) decide(ctx context.Context, current profiles.Profile, req state.SwitchRequest) (*profiles.Profile, bool, error) {
	for {
		next, err := selector.NextProfile(r.Store, r.Profiles, r.Strategy, r.Thresholds, current.Name, r.Now())
		var locked *selector.AllLockedError
		if err != nil && !errors.As(err, &locked) {
			return nil, false, err
		}
		if r.Mode == config.ModeAuto {
			if locked != nil {
				r.printf("switchyard: every profile is at its limit; waiting until %s (Ctrl+C stops)\n", locked.EarliestReset.Local().Format("15:04"))
				if quit, err := r.waitUntil(ctx, locked.EarliestReset); quit || err != nil {
					return nil, false, err
				}
				continue
			}
			if auto, paused := r.automatic(); auto {
				if err := breaker.Record(r.Store, r.Now()); err != nil {
					return nil, false, err
				}
				r.printf("switchyard: profile %s %s, continuing with %s\n", current.Name, describe(req.Reason), next.Name)
				if r.CarryContext {
					r.printf("%s\n", ColdCacheHint)
				}
				return &next, r.CarryContext, nil
			} else if paused != nil {
				r.printf("switchyard: automatic switching is paused: %v\n", paused)
			}
		}

		answer, err := r.ask(ctx, current, req, next, locked)
		if err != nil {
			return nil, false, err
		}
		switch answer {
		case "y":
			return &next, true, nil
		case "f":
			return &next, false, nil
		case "w":
			until := r.waitTarget(current, locked)
			r.printf("switchyard: waiting until %s (Ctrl+C stops)\n", until.Local().Format("15:04"))
			if quit, err := r.waitUntil(ctx, until); quit || err != nil {
				return nil, false, err
			}
		default:
			return nil, false, nil
		}
	}
}

// automatic reports whether a switch may happen without asking: the mode is auto and the
// limits on automatic switches are not reached. When the limits are the reason it is not,
// the second result says which.
func (r *Runner) automatic() (bool, error) {
	if r.Mode != config.ModeAuto {
		return false, nil
	}
	if err := breaker.Allowed(r.Store, r.Limits, r.Now()); err != nil {
		return false, err
	}
	return true, nil
}

// ask shows the situation and returns the answer: "y" switch and carry the
// conversation, "f" switch with a new conversation, "w" wait, "q" quit. Without
// a usable answer (end of input, unknown key) it returns "q".
func (r *Runner) ask(ctx context.Context, current profiles.Profile, req state.SwitchRequest, next profiles.Profile, locked *selector.AllLockedError) (string, error) {
	r.printf("\nswitchyard: profile %s %s.\n", current.Name, describe(req.Reason))
	if locked != nil {
		r.printf("All profiles are at their limit; the first one is available again at %s.\n[w] wait  [q] quit: ",
			locked.EarliestReset.Local().Format("15:04"))
	} else {
		r.printf("%s\n", ColdCacheHint)
		r.printf("Switch to %s? [y] continue the conversation  [f] new conversation  [w] wait for %s  [q] quit (Enter = %s): ",
			next.Name, current.Name, r.defaultAnswer())
	}

	line := make(chan string, 1)
	go func() {
		s, _ := r.prompt.ReadString('\n')
		line <- s
	}()
	var s string
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case s = <-line:
	}
	answer := strings.ToLower(strings.TrimSpace(s))
	if answer == "" && strings.HasSuffix(s, "\n") && locked == nil {
		answer = r.defaultAnswer()
	}
	if answer == "w" || (locked == nil && (answer == "y" || answer == "f")) {
		return answer, nil
	}
	return "q", nil
}

func (r *Runner) defaultAnswer() string {
	if r.CarryContext {
		return "y"
	}
	return "f"
}

// waitTarget is when waiting ends: the first profile available again if all
// are locked, otherwise the end of the current profile's cooldown.
func (r *Runner) waitTarget(current profiles.Profile, locked *selector.AllLockedError) time.Time {
	if locked != nil {
		return locked.EarliestReset
	}
	st, _ := r.Store.Read()
	return st.Profiles[current.Name].CooldownUntil
}

// waitUntil waits for t. A stop with Ctrl+C is the user quitting: quit is true and there is no error.
func (r *Runner) waitUntil(ctx context.Context, t time.Time) (quit bool, err error) {
	wait := r.Wait
	if wait == nil {
		wait = func(ctx context.Context, until time.Time) error { return selector.WaitUntil(ctx, until, r.Now) }
	}
	if err := wait(ctx, t); err != nil {
		if ctx.Err() != nil {
			r.printf("switchyard: stopped waiting\n")
			return true, nil
		}
		return false, err
	}
	return false, nil
}

func describe(reason string) string {
	if reason == state.ReasonThreshold {
		return "reached its usage threshold"
	}
	return "reached its limit"
}

func (r *Runner) printf(format string, a ...any) {
	_, _ = fmt.Fprintf(r.Out, format, a...)
}

// next resolves a switch request into the profile to continue with and whether
// to carry the conversation. A nil profile means the user quit.
func (r *Runner) next(ctx context.Context, current profiles.Profile, req state.SwitchRequest, answer *state.Answer) (*profiles.Profile, bool, error) {
	r.reload()
	if req.Reason == state.ReasonManual {
		return r.handoff(current, req)
	}
	h := hooks.Handler{Store: r.Store, Profile: current.Name, Now: r.Now}
	if err := h.MarkLimited(); err != nil {
		return nil, false, err
	}
	if answer != nil && answer.Action == state.ActionSwitch {
		// The mod's buttons asked; the user has decided.
		req.Target, req.Carry = answer.Target, answer.Carry
		return r.handoff(current, req)
	}
	return r.decide(ctx, current, req)
}

// handoff follows a request to continue in a named profile. The user has
// decided, so nothing is asked and the profile is not put into cooldown.
func (r *Runner) handoff(current profiles.Profile, req state.SwitchRequest) (*profiles.Profile, bool, error) {
	i := slices.IndexFunc(r.Profiles, func(p profiles.Profile) bool { return p.Name == req.Target })
	if i < 0 {
		return nil, false, fmt.Errorf("switch to %q: %w", req.Target, profiles.ErrNotFound)
	}
	carry := r.CarryContext
	if req.Carry != nil {
		carry = *req.Carry
	}
	r.printf("switchyard: switching from %s to %s\n", current.Name, req.Target)
	if carry {
		r.printf("%s\n", ColdCacheHint)
	}
	return &r.Profiles[i], carry, nil
}

// reload takes the settings of the current config; a config that cannot be read
// leaves the ones in use.
func (r *Runner) reload() {
	if r.Reload == nil {
		return
	}
	cfg, err := r.Reload()
	if err != nil {
		return
	}
	r.Mode, r.Strategy, r.CarryContext, r.ContinuePrompt = cfg.Mode, cfg.Strategy, cfg.CarryContext, cfg.ContinuePrompt
	r.Limits = breaker.FromConfig(cfg)
	r.Thresholds = selector.ThresholdsFromConfig(cfg)
}
