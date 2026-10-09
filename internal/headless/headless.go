// Package headless runs a non-interactive claude (`claude -p`) and moves it to
// the next profile when the current one hits its subscription limit.
package headless

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"time"

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
	// Prepare records p as in use and returns the final claude arguments for it.
	Prepare func(p profiles.Profile, args []string) ([]string, error)
	// Now returns the current time.
	Now func() time.Time
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
	stdin := newReplayableStdin(r.Launcher.Stdin)
	current := start
	attemptArgs := args
	var sessionID string
	for range len(r.Profiles) {
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
		if err != nil || code == 0 {
			return code, err
		}
		result := det.Result()
		if !result.Limited {
			return code, nil
		}
		if result.SessionID != "" {
			sessionID = result.SessionID
		}
		next, err := r.markLimited(current, result)
		if err != nil {
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

// markLimited records the usage seen in the output, puts current into cooldown
// and returns the profile to continue with.
func (r *Runner) markLimited(current profiles.Profile, result detector.Result) (profiles.Profile, error) {
	now := r.Now()
	err := r.Store.Update(func(st *state.State) error {
		p := st.Profiles[current.Name]
		if w := result.FiveHour; w != nil {
			p.FiveHour = &state.Usage{UsedPercent: w.UsedPercent, ResetsAt: w.ResetsAt, UpdatedAt: now.UTC()}
		}
		if w := result.SevenDay; w != nil {
			p.SevenDay = &state.Usage{UsedPercent: w.UsedPercent, ResetsAt: w.ResetsAt, UpdatedAt: now.UTC()}
		}
		st.Profiles[current.Name] = p
		return nil
	})
	if err != nil {
		return profiles.Profile{}, err
	}
	h := hooks.Handler{Store: r.Store, Profile: current.Name, Now: r.Now}
	if err := h.MarkLimited(); err != nil {
		return profiles.Profile{}, err
	}
	return selector.NextProfile(r.Store, r.Profiles, r.Strategy, current.Name, now)
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
