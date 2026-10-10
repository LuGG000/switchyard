package interactive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/LuGG000/switchyard/internal/breaker"
	"github.com/LuGG000/switchyard/internal/config"
	"github.com/LuGG000/switchyard/internal/hooks"
	"github.com/LuGG000/switchyard/internal/launcher"
	"github.com/LuGG000/switchyard/internal/profiles"
	"github.com/LuGG000/switchyard/internal/selector"
	"github.com/LuGG000/switchyard/internal/state"
)

var fakeClaude, switchyardBin string

// TestMain builds fakeclaude and switchyard once for all tests. The hooks that
// fakeclaude calls are the real ones.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "interactive")
	if err != nil {
		panic(err)
	}
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	fakeClaude = filepath.Join(dir, "fakeclaude"+ext)
	switchyardBin = filepath.Join(dir, "switchyard"+ext)
	for out, pkg := range map[string]string{fakeClaude: "../../fakeclaude", switchyardBin: "../../cmd/switchyard"} {
		if b, err := exec.Command("go", "build", "-o", out, pkg).CombinedOutput(); err != nil {
			panic(string(b))
		}
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

type fixture struct {
	runner *Runner
	store  *state.Store
	stdout *bytes.Buffer
	out    *bytes.Buffer
	// launches records the profile of every launch.
	launches []string
}

// newFixture sets up one profile per name below a data dir. A name containing
// "limited" hits a simulated limit, one containing "thr" reports 96% usage.
// configToml is written as the config file.
func newFixture(t *testing.T, configToml string, names ...string) *fixture {
	t.Helper()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	if err := os.Mkdir(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(configToml), 0o600); err != nil {
		t.Fatal(err)
	}
	var list []profiles.Profile
	for _, n := range names {
		dir := filepath.Join(root, "profiles", n)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		list = append(list, profiles.Profile{Name: n, Dir: dir})
	}
	f := &fixture{
		store:  state.NewStore(filepath.Join(root, "state.json")),
		stdout: &bytes.Buffer{},
		out:    &bytes.Buffer{},
	}
	f.runner = &Runner{
		Launcher: launcher.Launcher{
			Claude: fakeClaude,
			Environ: append(os.Environ(),
				"SWITCHYARD_DATA_DIR="+root,
				"SWITCHYARD_CONFIG_DIR="+configDir,
				"FAKE_LIMIT_DIR=limited",
				"FAKE_THRESHOLD_DIR=thr"),
			Stdout: f.stdout,
			Stderr: f.out,
		},
		Store:    f.store,
		Profiles: list,
		Mode:     config.ModeAuto,
		Strategy: config.StrategySequential,
		Prepare: func(p profiles.Profile, args []string) ([]string, error) {
			f.launches = append(f.launches, p.Name)
			settings, err := hooks.Settings(switchyardBin, p.Name)
			if err != nil {
				return nil, err
			}
			return append([]string{"--settings", settings}, args...), nil
		},
		Now:       time.Now,
		Poll:      20 * time.Millisecond,
		StopGrace: 2 * time.Second,
		In:        strings.NewReader(""),
		Out:       f.out,
	}
	return f
}

// lastReport decodes the report fakeclaude printed on its last normal run.
func (f *fixture) lastReport(t *testing.T) (args []string, configDir string) {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(f.stdout.String()), "\n")
	var r struct {
		Args      []string `json:"args"`
		ConfigDir string   `json:"configDir"`
	}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &r); err != nil {
		t.Fatalf("decode report from %q: %v", f.stdout.String(), err)
	}
	return r.Args, r.ConfigDir
}

func (f *fixture) run(t *testing.T, args ...string) (int, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return f.runner.Run(ctx, f.runner.Profiles[0], args)
}

// resumed reports whether args resume the fake session, and whether the
// conversation was continued with the given prompt as the last argument.
func resumed(args []string) bool {
	i := slices.Index(args, "--resume")
	return i >= 0 && i+1 < len(args) && args[i+1] == "fake-session"
}

func TestAutoSwitchOnLimitCarriesContext(t *testing.T) {
	f := newFixture(t, "", "a-limited", "b")
	f.runner.CarryContext = true
	f.runner.ContinuePrompt = "please continue"
	code, err := f.run(t, "--model", "haiku")
	if err != nil || code != 0 {
		t.Fatalf("Run = %d, %v", code, err)
	}
	args, dir := f.lastReport(t)
	if !resumed(args) || args[len(args)-1] != "please continue" || !slices.Contains(args, "haiku") {
		t.Errorf("args of the second launch = %v", args)
	}
	if dir != f.runner.Profiles[1].Dir {
		t.Errorf("second launch used %q, want %q", dir, f.runner.Profiles[1].Dir)
	}
	if !slices.Equal(f.launches, []string{"a-limited", "b"}) {
		t.Errorf("launches = %v", f.launches)
	}
	if !strings.Contains(f.out.String(), "a-limited reached its limit, continuing with b") || !strings.Contains(f.out.String(), "cold prompt cache") {
		t.Errorf("output = %q", f.out.String())
	}
	st, _ := f.store.Read()
	if !st.Profiles["a-limited"].CooldownUntil.After(time.Now()) {
		t.Error("the limited profile is not in cooldown")
	}
	if st.SwitchRequest != nil {
		t.Errorf("switch request %+v was not cleared", st.SwitchRequest)
	}
}

func TestAutoSwitchWithoutCarryContextStartsFresh(t *testing.T) {
	f := newFixture(t, "", "a-limited", "b")
	f.runner.CarryContext = false
	f.runner.ContinuePrompt = "please continue"
	if code, err := f.run(t, "--model", "haiku"); err != nil || code != 0 {
		t.Fatalf("Run = %d, %v", code, err)
	}
	args, _ := f.lastReport(t)
	if slices.Contains(args, "--resume") || slices.Contains(args, "--continue") || slices.Contains(args, "please continue") {
		t.Errorf("args of the second launch = %v, want a fresh conversation", args)
	}
	if strings.Contains(f.out.String(), "cold prompt cache") {
		t.Errorf("cold cache hint shown without carrying context: %q", f.out.String())
	}
}

func TestAutoSwitchOnThreshold(t *testing.T) {
	f := newFixture(t, "proactive_threshold = 90\n", "a-thr", "b")
	f.runner.CarryContext = true
	if code, err := f.run(t); err != nil || code != 0 {
		t.Fatalf("Run = %d, %v", code, err)
	}
	args, _ := f.lastReport(t)
	if !resumed(args) {
		t.Errorf("args of the second launch = %v", args)
	}
	if !strings.Contains(f.out.String(), "a-thr reached its usage threshold, continuing with b") {
		t.Errorf("output = %q", f.out.String())
	}
}

func TestAutoWaitsForTheEarliestResetAndContinues(t *testing.T) {
	f := newFixture(t, "", "a-limited", "b")
	reset := time.Now().Add(20 * time.Minute)
	err := f.store.Update(func(st *state.State) error {
		st.Profiles["b"] = state.Profile{CooldownUntil: reset}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Now()
	f.runner.Now = func() time.Time { return clock }
	var waited []time.Time
	f.runner.Wait = func(_ context.Context, until time.Time) error {
		waited = append(waited, until)
		clock = until
		return nil
	}

	if code, err := f.run(t); err != nil || code != 0 {
		t.Fatalf("Run = %d, %v", code, err)
	}
	if len(waited) != 1 || !waited[0].Equal(reset) {
		t.Errorf("waited for %v, want once for %v", waited, reset)
	}
	if !slices.Equal(f.launches, []string{"a-limited", "b"}) {
		t.Errorf("launches = %v, want the run to continue in b after the wait", f.launches)
	}
	if !strings.Contains(f.out.String(), "every profile is at its limit; waiting until") {
		t.Errorf("output = %q", f.out.String())
	}
}

func TestStoppingTheWaitEndsTheRunWithoutAnError(t *testing.T) {
	f := newFixture(t, "", "a-limited", "b-limited")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.runner.Wait = func(ctx context.Context, _ time.Time) error {
		cancel()
		return ctx.Err()
	}
	code, err := f.runner.Run(ctx, f.runner.Profiles[0], nil)
	if err != nil || code != 0 {
		t.Fatalf("Run = %d, %v", code, err)
	}
	if !strings.Contains(f.out.String(), "stopped waiting") {
		t.Errorf("output = %q", f.out.String())
	}
}

func TestNormalExitDoesNotSwitch(t *testing.T) {
	f := newFixture(t, "", "a-limited", "b")
	code, err := f.runner.Run(context.Background(), f.runner.Profiles[1], nil)
	if err != nil || code != 0 {
		t.Fatalf("Run = %d, %v", code, err)
	}
	if !slices.Equal(f.launches, []string{"b"}) || f.out.Len() != 0 {
		t.Errorf("launches = %v, output = %q", f.launches, f.out.String())
	}
}

func TestAskAnswers(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		carryDefault bool
		wantSwitch   bool
		wantResume   bool
	}{
		{"yes continues the conversation", "y\n", false, true, true},
		{"f starts a new conversation", "f\n", true, true, false},
		{"Enter follows carry_context on", "\n", true, true, true},
		{"Enter follows carry_context off", "\n", false, true, false},
		{"quit", "q\n", true, false, false},
		{"end of input quits", "", true, false, false},
		{"unknown answer quits", "x\n", true, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, "", "a-limited", "b")
			f.runner.Mode = config.ModeAsk
			f.runner.CarryContext = tt.carryDefault
			f.runner.In = strings.NewReader(tt.input)
			code, err := f.run(t)
			if err != nil || code != 0 {
				t.Fatalf("Run = %d, %v", code, err)
			}
			if got := slices.Contains(f.launches, "b"); got != tt.wantSwitch {
				t.Fatalf("switched = %v, want %v (launches %v)", got, tt.wantSwitch, f.launches)
			}
			if tt.wantSwitch {
				if args, _ := f.lastReport(t); resumed(args) != tt.wantResume {
					t.Errorf("resumed = %v, want %v (args %v)", resumed(args), tt.wantResume, args)
				}
			}
			if !strings.Contains(f.out.String(), "Switch to b?") || !strings.Contains(f.out.String(), "cold prompt cache") {
				t.Errorf("question = %q", f.out.String())
			}
		})
	}
}

func TestAskWhenAllLockedOffersWaitAndQuit(t *testing.T) {
	f := newFixture(t, "", "a-limited", "b-limited")
	f.runner.Mode = config.ModeAsk
	f.runner.In = strings.NewReader("y\nq\n")
	// The first limit asks about b-limited, the second finds everything locked.
	if code, err := f.run(t); err != nil || code != 0 {
		t.Fatalf("Run = %d, %v", code, err)
	}
	if !strings.Contains(f.out.String(), "All profiles are at their limit") || !strings.Contains(f.out.String(), "[w] wait  [q] quit") {
		t.Errorf("output = %q", f.out.String())
	}
}

func TestThresholdRequestIsIgnoredWithoutAlternative(t *testing.T) {
	f := newFixture(t, "", "a", "b")
	now := time.Now()
	err := f.store.Update(func(st *state.State) error {
		st.Profiles["b"] = state.Profile{CooldownUntil: now.Add(time.Hour)}
		st.SwitchRequest = &state.SwitchRequest{Profile: "a", Reason: state.ReasonThreshold, RequestedAt: now.Add(time.Minute)}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, ok := f.runner.watch(ctx, f.runner.Profiles[0], now); ok {
		t.Error("a threshold request ended the session although no other profile is available")
	}
	st, _ := f.store.Read()
	if st.SwitchRequest == nil {
		t.Error("the request was cleared although it was not used")
	}
}

func TestStaleRequestIsIgnored(t *testing.T) {
	f := newFixture(t, "", "a", "b")
	now := time.Now()
	err := f.store.Update(func(st *state.State) error {
		st.SwitchRequest = &state.SwitchRequest{Profile: "a", Reason: state.ReasonRateLimit, RequestedAt: now.Add(-time.Minute)}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, ok := f.runner.watch(ctx, f.runner.Profiles[0], now); ok {
		t.Error("a request from before the launch ended the session")
	}
}

// handoffAfter writes a manual switch request for profile once claude has had
// time to start.
func (f *fixture) handoffAfter(t *testing.T, profile string, req state.SwitchRequest) {
	t.Helper()
	go func() {
		time.Sleep(500 * time.Millisecond)
		req.Profile, req.Reason, req.RequestedAt = profile, state.ReasonManual, time.Now().UTC()
		_ = f.store.Update(func(st *state.State) error { st.SwitchRequest = &req; return nil })
	}()
}

func TestManualHandoffSwitchesWithoutAsking(t *testing.T) {
	for _, carry := range []bool{true, false} {
		t.Run(map[bool]string{true: "carry", false: "fresh"}[carry], func(t *testing.T) {
			// a-thr stays at its prompt (no threshold is configured), b runs to the end.
			f := newFixture(t, "", "a-thr", "b")
			f.runner.Mode = config.ModeAsk
			f.runner.In = strings.NewReader("") // asking would end the run as a quit
			f.handoffAfter(t, "a-thr", state.SwitchRequest{Target: "b", SessionID: "s1", Carry: &carry})

			code, err := f.run(t)
			if err != nil || code != 0 {
				t.Fatalf("Run = %d, %v", code, err)
			}
			args, dir := f.lastReport(t)
			if dir != f.runner.Profiles[1].Dir {
				t.Fatalf("second launch used %q, want %q", dir, f.runner.Profiles[1].Dir)
			}
			if got := slices.Contains(args, "--resume") && slices.Contains(args, "s1"); got != carry {
				t.Errorf("resumed = %v, want %v (args %v)", got, carry, args)
			}
			st, _ := f.store.Read()
			if !st.Profiles["a-thr"].CooldownUntil.IsZero() {
				t.Error("a manual handoff must not put the profile into cooldown")
			}
			if strings.Contains(f.out.String(), "Switch to") {
				t.Errorf("a manual handoff asked: %q", f.out.String())
			}
		})
	}
}

func TestManualHandoffToUnknownProfileFails(t *testing.T) {
	f := newFixture(t, "", "a-thr", "b")
	f.handoffAfter(t, "a-thr", state.SwitchRequest{Target: "zzz"})
	if _, err := f.run(t); !errors.Is(err, profiles.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestSettingsAreReloadedAtEveryLimit(t *testing.T) {
	f := newFixture(t, "", "a-limited", "b")
	f.runner.Mode = config.ModeAsk
	f.runner.In = strings.NewReader("") // asking would end the run as a quit
	f.runner.Reload = func() (config.Config, error) {
		cfg := config.Default()
		cfg.Mode, cfg.CarryContext = config.ModeAuto, false
		return cfg, nil
	}
	if code, err := f.run(t); err != nil || code != 0 {
		t.Fatalf("Run = %d, %v", code, err)
	}
	args, _ := f.lastReport(t)
	if slices.Contains(args, "--resume") || strings.Contains(f.out.String(), "Switch to") {
		t.Errorf("the changed settings were not used: args %v, output %q", args, f.out.String())
	}
}

func TestUnreadableConfigKeepsTheSettingsInUse(t *testing.T) {
	f := newFixture(t, "", "a-limited", "b")
	f.runner.Mode = config.ModeAuto
	f.runner.Reload = func() (config.Config, error) { return config.Config{}, errors.New("broken") }
	if code, err := f.run(t); err != nil || code != 0 {
		t.Fatalf("Run = %d, %v", code, err)
	}
	if !slices.Contains(f.launches, "b") {
		t.Errorf("launches = %v", f.launches)
	}
}

// seedSwitches records automatic switches the given number of minutes ago, oldest first.
func (f *fixture) seedSwitches(t *testing.T, minutesAgo ...int) {
	t.Helper()
	err := f.store.Update(func(st *state.State) error {
		for _, m := range minutesAgo {
			st.AutoSwitches = append(st.AutoSwitches, time.Now().Add(-time.Duration(m)*time.Minute))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAutoSwitchIsRecorded(t *testing.T) {
	f := newFixture(t, "", "a-limited", "b")
	f.runner.Limits = breaker.Limits{MinInterval: 10 * time.Minute, MaxPerDay: 6}
	if code, err := f.run(t); err != nil || code != 0 {
		t.Fatalf("Run = %d, %v", code, err)
	}
	if st, _ := f.store.Read(); len(st.AutoSwitches) != 1 {
		t.Errorf("AutoSwitches = %v, want one entry", st.AutoSwitches)
	}
}

func TestAutoSwitchAsksWhileTheLimitsAreReached(t *testing.T) {
	tests := []struct {
		name   string
		limits breaker.Limits
		seed   []int
		answer string
		paused string
	}{
		{"minimum interval, quit", breaker.Limits{MinInterval: 10 * time.Minute}, []int{2}, "q\n", "the last automatic switch was"},
		{"minimum interval, switch anyway", breaker.Limits{MinInterval: 10 * time.Minute}, []int{2}, "y\n", "the last automatic switch was"},
		{"daily maximum, quit", breaker.Limits{MaxPerDay: 2}, []int{300, 120}, "q\n", "2 automatic switches in the last 24 hours (maximum 2)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, "", "a-limited", "b")
			f.runner.Limits = tt.limits
			f.runner.In = strings.NewReader(tt.answer)
			f.seedSwitches(t, tt.seed...)
			if code, err := f.run(t); err != nil || code != 0 {
				t.Fatalf("Run = %d, %v", code, err)
			}
			out := f.out.String()
			if !strings.Contains(out, "automatic switching is paused: "+tt.paused) || !strings.Contains(out, "Switch to b?") {
				t.Errorf("output = %q, want the pause and the question", out)
			}
			switched := slices.Contains(f.launches, "b")
			if switched != (tt.answer == "y\n") {
				t.Errorf("switched = %v with answer %q (launches %v)", switched, tt.answer, f.launches)
			}
			if st, _ := f.store.Read(); len(st.AutoSwitches) != len(tt.seed) {
				t.Errorf("a switch the user chose was recorded: %v", st.AutoSwitches)
			}
		})
	}
}

func TestAutoSwitchProceedsOnceTheIntervalHasPassed(t *testing.T) {
	f := newFixture(t, "", "a-limited", "b")
	f.runner.Limits = breaker.Limits{MinInterval: 10 * time.Minute}
	f.seedSwitches(t, 30)
	if code, err := f.run(t); err != nil || code != 0 {
		t.Fatalf("Run = %d, %v", code, err)
	}
	if !slices.Equal(f.launches, []string{"a-limited", "b"}) || strings.Contains(f.out.String(), "paused") {
		t.Errorf("launches = %v, output = %q", f.launches, f.out.String())
	}
}

func TestThresholdRequestIsIgnoredWhenTheOtherProfileIsOverTheThresholdToo(t *testing.T) {
	f := newFixture(t, "", "a", "b")
	f.runner.Thresholds = selector.Thresholds{FiveHour: 50}
	now := time.Now()
	err := f.store.Update(func(st *state.State) error {
		st.Profiles["b"] = state.Profile{FiveHour: &state.Usage{UsedPercent: 70, ResetsAt: now.Add(time.Hour)}}
		st.SwitchRequest = &state.SwitchRequest{Profile: "a", Reason: state.ReasonThreshold, RequestedAt: now.Add(time.Minute)}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, ok := f.runner.watch(ctx, f.runner.Profiles[0], now); ok {
		t.Error("a threshold request ended the session although the other profile is over the threshold as well")
	}
}
