package interactive

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/LuGG000/switchyard/internal/config"
	"github.com/LuGG000/switchyard/internal/state"
)

// playMod stands in for the mod: it polls like `switchyard decision` (a
// heartbeat each round) and, once a question is pending, calls answer with it.
// It stops when the test ends, and the test waits for it so that it writes nothing
// while the temp directory is removed.
func (f *fixture) playMod(t *testing.T, answer func(d *state.Decision) *state.Answer) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		<-done
	})
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			_ = f.store.TouchHeartbeat(time.Now())
			_ = f.store.Update(func(st *state.State) error {
				if d := st.Decision; d != nil && d.Answer == nil {
					d.Answer = answer(d)
				}
				return nil
			})
			time.Sleep(10 * time.Millisecond)
		}
	}()
}

func askFixture(t *testing.T, modWait time.Duration) *fixture {
	t.Helper()
	f := newFixture(t, "", "a-limited", "b")
	f.runner.Mode = config.ModeAsk
	f.runner.CarryContext = true
	f.runner.ModWait = modWait
	f.runner.ModStale = time.Second
	return f
}

func TestModButtonsAnswerALimitWithoutTheTerminal(t *testing.T) {
	f := askFixture(t, 20*time.Second)
	var asked state.Decision
	f.playMod(t, func(d *state.Decision) *state.Answer {
		asked = *d
		fresh := false
		return &state.Answer{Action: state.ActionSwitch, Target: "b", Carry: &fresh}
	})

	code, err := f.run(t)
	if err != nil || code != 0 {
		t.Fatalf("Run = %d, %v", code, err)
	}
	if !slices.Equal(f.launches, []string{"a-limited", "b"}) {
		t.Fatalf("launches = %v", f.launches)
	}
	if args, _ := f.lastReport(t); resumed(args) {
		t.Errorf("the answer asked for a new conversation, args %v", args)
	}
	if strings.Contains(f.out.String(), "Switch to b?") {
		t.Errorf("the terminal was asked as well: %q", f.out.String())
	}
	if asked.Profile != "a-limited" || !slices.Equal(asked.Options, []string{"b"}) || !asked.Carry {
		t.Errorf("question = %+v", asked)
	}
	st, _ := f.store.Read()
	if st.Decision != nil {
		t.Errorf("the decision %+v was not cleared", st.Decision)
	}
	if !st.Profiles["a-limited"].CooldownUntil.After(time.Now()) {
		t.Error("the limited profile is not in cooldown")
	}
}

func TestModAnswerFollowsCarryContextWhenItNamesNone(t *testing.T) {
	f := askFixture(t, 20*time.Second)
	f.playMod(t, func(*state.Decision) *state.Answer {
		return &state.Answer{Action: state.ActionSwitch, Target: "b"}
	})
	if code, err := f.run(t); err != nil || code != 0 {
		t.Fatalf("Run = %d, %v", code, err)
	}
	if args, _ := f.lastReport(t); !resumed(args) {
		t.Errorf("carry_context is on, so the conversation continues; args %v", args)
	}
}

func TestStayKeepsClaudeRunning(t *testing.T) {
	f := askFixture(t, 20*time.Second)
	f.playMod(t, func(*state.Decision) *state.Answer { return &state.Answer{Action: state.ActionStay} })

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	_, _ = f.runner.Run(ctx, f.runner.Profiles[0], nil)
	if !slices.Equal(f.launches, []string{"a-limited"}) {
		t.Errorf("stay must not start another profile, launches = %v", f.launches)
	}
	if strings.Contains(f.out.String(), "Switch to") {
		t.Errorf("stay must not ask in the terminal: %q", f.out.String())
	}
}

func TestUnansweredButtonsFallBackToTheTerminal(t *testing.T) {
	f := askFixture(t, 300*time.Millisecond)
	f.runner.In = strings.NewReader("y\n")
	f.playMod(t, func(*state.Decision) *state.Answer { return nil })

	if code, err := f.run(t); err != nil || code != 0 {
		t.Fatalf("Run = %d, %v", code, err)
	}
	if !slices.Contains(f.launches, "b") || !strings.Contains(f.out.String(), "Switch to b?") {
		t.Errorf("launches %v, output %q: the terminal question should follow", f.launches, f.out.String())
	}
}

func TestWithoutAModTheTerminalAsksAtOnce(t *testing.T) {
	f := askFixture(t, 20*time.Second)
	f.runner.In = strings.NewReader("y\n")

	start := time.Now()
	if code, err := f.run(t); err != nil || code != 0 {
		t.Fatalf("Run = %d, %v", code, err)
	}
	if time.Since(start) > 10*time.Second || !strings.Contains(f.out.String(), "Switch to b?") {
		t.Errorf("took %v, output %q", time.Since(start), f.out.String())
	}
}

func TestAMoDThatStopsPollingDoesNotBlockTheQuestion(t *testing.T) {
	f := askFixture(t, 20*time.Second)
	f.runner.In = strings.NewReader("y\n")
	// A single heartbeat shortly before the limit, then silence.
	_ = f.store.TouchHeartbeat(time.Now().Add(time.Second))

	start := time.Now()
	if code, err := f.run(t); err != nil || code != 0 {
		t.Fatalf("Run = %d, %v", code, err)
	}
	if time.Since(start) > 10*time.Second || !strings.Contains(f.out.String(), "Switch to b?") {
		t.Errorf("took %v, output %q", time.Since(start), f.out.String())
	}
}

func TestAutoModeNeverUsesTheButtons(t *testing.T) {
	f := askFixture(t, 20*time.Second)
	f.runner.Mode = config.ModeAuto
	asked := false
	f.playMod(t, func(*state.Decision) *state.Answer { asked = true; return nil })
	if code, err := f.run(t); err != nil || code != 0 {
		t.Fatalf("Run = %d, %v", code, err)
	}
	if asked || !slices.Contains(f.launches, "b") {
		t.Errorf("asked = %v, launches %v", asked, f.launches)
	}
}
