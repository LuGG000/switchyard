// Package headless runs a non-interactive claude (`claude -p`) and moves it to
// the next profile when the current one hits its subscription limit.
package headless

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/LuGG000/switchyard/internal/breaker"
	"github.com/LuGG000/switchyard/internal/detector"
	"github.com/LuGG000/switchyard/internal/hooks"
	"github.com/LuGG000/switchyard/internal/launcher"
	"github.com/LuGG000/switchyard/internal/profiles"
	"github.com/LuGG000/switchyard/internal/selector"
	"github.com/LuGG000/switchyard/internal/state"
)

// Runner runs claude headless over a set of profiles.
type Runner struct {
	// Launcher is the template for every attempt; its Stdin, Stdout and Stderr are the caller's.
	Launcher launcher.Launcher
	Store    *state.Store
	// Profiles are the profiles a run may move to.
	Profiles []profiles.Profile
	// Strategy chooses the next profile, see the selector package.
	Strategy string
	// CarryContext resumes the conversation in the next profile instead of starting over.
	CarryContext bool
	// Limits bound the automatic switches; a run that would go over them stops.
	Limits breaker.Limits
	// Thresholds keep a profile that is over them from being chosen while another is below.
	Thresholds selector.Thresholds
	// Prepare records p as in use and returns the final claude arguments for it.
	Prepare func(p profiles.Profile, args []string) ([]string, error)
	// Now returns the current time.
	Now func() time.Time
	// WaitForReset makes a run that finds every profile at its limit wait for the first
	// reset and go on, instead of stopping.
	WaitForReset bool
	// Wait blocks until the given time; nil waits on the clock. Tests replace it.
	Wait func(ctx context.Context, until time.Time) error
	// Notice receives a line for every switch.
	Notice io.Writer
}

// Run runs claude with args for start. If the profile hits its limit, the
// profile is put into cooldown and the run continues in the next one until it
// succeeds, fails for another reason, or no profile is left. It returns claude's
// exit code of the last attempt.
//
// Output of an attempt that hit the limit has already been passed on when the
// next attempt starts, so a consumer of stream-json sees both attempts.
func (r *Runner) Run(ctx context.Context, start profiles.Profile, args []string) (int, error) {
	if len(r.Profiles) == 0 {
		return 0, selector.ErrNoCandidates
	}
	stdin := newReplayableStdin(r.Launcher.Stdin)
	current, err := r.startProfile(start)
	if err != nil {
		return 0, err
	}
	attemptArgs := args
	var sessionID string
	// Every profile gets one attempt; a wait for a reset starts the count again.
	budget := len(r.Profiles)
	for budget > 0 {
		budget--
		claudeArgs, err := r.Prepare(current, attemptArgs)
		if err != nil {
			return 0, err
		}
		var det detector.Detector
		l := r.Launcher
		l.Stdin = stdin.reader()
		l.Stdout = io.MultiWriter(r.Launcher.Stdout, &det)
		l.Stderr = io.MultiWriter(r.Launcher.Stderr, &det)
		code, err := l.Run(ctx, current, claudeArgs)
		if err != nil {
			return code, err
		}
		result := det.Result()
		if err := r.recordUsage(current, result); err != nil {
			return code, err
		}
		if code == 0 {
			return 0, nil
		}
		if !result.Limited {
			return code, nil
		}
		if result.SessionID != "" {
			sessionID = result.SessionID
		}
		next, waited, err := r.markLimitedAndChoose(ctx, current)
		if err != nil {
			return code, err
		}
		if waited {
			budget = len(r.Profiles)
		}
		if err := breaker.Allowed(r.Store, r.Limits, r.Now()); err != nil {
			return code, fmt.Errorf("profile %s reached its limit; automatic switching is paused, so the run stops: %w", current.Name, err)
		}
		if err := breaker.Record(r.Store, r.Now()); err != nil {
			return code, err
		}
		attemptArgs = args
		if r.CarryContext {
			attemptArgs = launcher.ResumeArgs(args, sessionID)
		}
		_, _ = fmt.Fprintf(r.Notice, "switchyard: profile %s reached its limit, continuing with %s\n", current.Name, next.Name)
		current = next
	}
	return 0, fmt.Errorf("giving up after %d profiles hit their limit", len(r.Profiles))
}

// recordUsage stores the rate limit windows the output reported for p.
func (r *Runner) recordUsage(p profiles.Profile, result detector.Result) error {
	if result.FiveHour == nil && result.SevenDay == nil {
		return nil
	}
	now := r.Now().UTC()
	return r.Store.Update(func(st *state.State) error {
		entry := st.Profiles[p.Name]
		if w := result.FiveHour; w != nil {
			entry.FiveHour = &state.Usage{UsedPercent: w.UsedPercent, ResetsAt: w.ResetsAt, UpdatedAt: now}
		}
		if w := result.SevenDay; w != nil {
			entry.SevenDay = &state.Usage{UsedPercent: w.UsedPercent, ResetsAt: w.ResetsAt, UpdatedAt: now}
		}
		st.Profiles[p.Name] = entry
		return nil
	})
}

// markLimitedAndChoose puts current into cooldown and returns the profile to continue
// with. When every profile is at its limit and WaitForReset is set, it waits for the
// first reset (and says so), which waited reports; otherwise the selector's error
// ends the run.
func (r *Runner) markLimitedAndChoose(ctx context.Context, current profiles.Profile) (next profiles.Profile, waited bool, err error) {
	h := hooks.Handler{Store: r.Store, Profile: current.Name, Now: r.Now}
	if err := h.MarkLimited(); err != nil {
		return profiles.Profile{}, false, err
	}
	for {
		next, err := selector.NextProfile(r.Store, r.Profiles, r.Strategy, r.Thresholds, current.Name, r.Now())
		var locked *selector.AllLockedError
		if !errors.As(err, &locked) || !r.WaitForReset {
			return next, waited, err
		}
		_, _ = fmt.Fprintf(r.Notice, "switchyard: every profile is at its limit; waiting until %s (Ctrl+C stops)\n", locked.EarliestReset.Local().Format("15:04"))
		wait := r.Wait
		if wait == nil {
			wait = func(ctx context.Context, until time.Time) error { return selector.WaitUntil(ctx, until, r.Now) }
		}
		if err := wait(ctx, locked.EarliestReset); err != nil {
			return profiles.Profile{}, waited, err
		}
		waited = true
	}
}

// replayableStdin hands every attempt the same input. A terminal is passed
// through untouched; anything else is recorded while the first attempt reads it
// and replayed for later attempts.
type replayableStdin struct {
	in       io.Reader
	recorded *bytes.Buffer
}

func newReplayableStdin(in io.Reader) *replayableStdin {
	if in == nil {
		return &replayableStdin{}
	}
	if f, ok := in.(*os.File); ok {
		if info, err := f.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
			return &replayableStdin{in: in}
		}
	}
	return &replayableStdin{in: in, recorded: &bytes.Buffer{}}
}

func (s *replayableStdin) reader() io.Reader {
	switch {
	case s.in == nil:
		return nil
	case s.recorded == nil:
		return s.in
	}
	replay := bytes.NewReader(bytes.Clone(s.recorded.Bytes()))
	return io.MultiReader(replay, io.TeeReader(s.in, s.recorded))
}

// startProfile moves a run off a start profile that is over a usage threshold when a
// profile below the thresholds is available and the limits on automatic switches allow
// it. Anything else keeps the start profile.
func (r *Runner) startProfile(start profiles.Profile) (profiles.Profile, error) {
	st, err := r.Store.Read()
	now := r.Now()
	if err != nil || !r.Thresholds.Over(st.Profiles[start.Name], now) {
		return start, nil
	}
	next, err := selector.NextProfile(r.Store, r.Profiles, r.Strategy, r.Thresholds, start.Name, now)
	if err != nil || next.Name == start.Name || r.Thresholds.Over(st.Profiles[next.Name], now) {
		return start, nil
	}
	if breaker.Allowed(r.Store, r.Limits, now) != nil {
		return start, nil
	}
	if err := breaker.Record(r.Store, now); err != nil {
		return start, err
	}
	_, _ = fmt.Fprintf(r.Notice, "switchyard: profile %s is over its usage threshold, starting with %s\n", start.Name, next.Name)
	return next, nil
}
