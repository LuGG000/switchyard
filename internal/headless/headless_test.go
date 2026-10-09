package headless

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/LuGG000/switchyard/internal/config"
	"github.com/LuGG000/switchyard/internal/launcher"
	"github.com/LuGG000/switchyard/internal/profiles"
	"github.com/LuGG000/switchyard/internal/selector"
	"github.com/LuGG000/switchyard/internal/state"
)

var fakeClaude string

// TestMain builds fakeclaude once for all tests.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fakeclaude")
	if err != nil {
		panic(err)
	}
	name := "fakeclaude"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	fakeClaude = filepath.Join(dir, name)
	if out, err := exec.Command("go", "build", "-o", fakeClaude, "../../fakeclaude").CombinedOutput(); err != nil {
		panic(string(out))
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

type fixture struct {
	runner *Runner
	store  *state.Store
	stdout *bytes.Buffer
	notice *bytes.Buffer
	// attempts records the profile and arguments of every attempt.
	attempts []attempt
}

type attempt struct {
	profile string
	args    []string
}

// newFixture sets up profiles "a-limited" and "b" (plus "c-limited" when
// withThird is set). Profiles whose directory name contains "limited" hit a
// simulated limit.
func newFixture(t *testing.T, withThird bool) *fixture {
	t.Helper()
	root := t.TempDir()
	names := []string{"a-limited", "b"}
	if withThird {
		names = append(names, "c-limited")
	}
	var list []profiles.Profile
	for _, n := range names {
		dir := filepath.Join(root, n)
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		list = append(list, profiles.Profile{Name: n, Dir: dir})
	}
	f := &fixture{
		store:  state.NewStore(filepath.Join(root, "state.json")),
		stdout: &bytes.Buffer{},
		notice: &bytes.Buffer{},
	}
	f.runner = &Runner{
		Launcher: launcher.Launcher{
			Claude:  fakeClaude,
			Environ: append(os.Environ(), "FAKE_LIMIT_DIR=limited"),
			Stdout:  f.stdout,
			Stderr:  io.Discard,
		},
		Store:        f.store,
		Profiles:     list,
		Strategy:     config.StrategySequential,
		CarryContext: true,
		Prepare: func(p profiles.Profile, args []string) ([]string, error) {
			f.attempts = append(f.attempts, attempt{p.Name, slices.Clone(args)})
			return args, nil
		},
		Now:    time.Now,
		Notice: f.notice,
	}
	return f
}

// lastReport decodes the report fakeclaude printed on its last successful run.
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

func TestFailoverResumesSessionFromStreamJSON(t *testing.T) {
	f := newFixture(t, false)
	args := []string{"-p", "--output-format", "stream-json", "hello"}
	code, err := f.runner.Run(context.Background(), f.runner.Profiles[0], args)
	if err != nil || code != 0 {
		t.Fatalf("Run = %d, %v", code, err)
	}

	got, dir := f.lastReport(t)
	want := []string{"--resume", "fake-session", "-p", "--output-format", "stream-json", "hello"}
	if !slices.Equal(got, want) {
		t.Errorf("second attempt args = %v, want %v", got, want)
	}
	if dir != f.runner.Profiles[1].Dir {
		t.Errorf("second attempt used config dir %q, want %q", dir, f.runner.Profiles[1].Dir)
	}
	if !strings.Contains(f.notice.String(), "a-limited reached its limit, continuing with b") {
		t.Errorf("notice = %q", f.notice.String())
	}

	st, err := f.store.Read()
	if err != nil {
		t.Fatal(err)
	}
	limited := st.Profiles["a-limited"]
	if !limited.CooldownUntil.After(time.Now()) {
		t.Errorf("cooldown_until = %v, want a future time", limited.CooldownUntil)
	}
	if limited.FiveHour == nil || limited.FiveHour.UsedPercent != 100 {
		t.Errorf("five_hour = %+v, want the usage from the rate limit event", limited.FiveHour)
	}
}

func TestFailoverContinuesMostRecentConversationWithoutSessionID(t *testing.T) {
	f := newFixture(t, false)
	if code, err := f.runner.Run(context.Background(), f.runner.Profiles[0], []string{"-p", "hello"}); err != nil || code != 0 {
		t.Fatalf("Run = %d, %v", code, err)
	}
	got, _ := f.lastReport(t)
	if want := []string{"--continue", "-p", "hello"}; !slices.Equal(got, want) {
		t.Errorf("second attempt args = %v, want %v", got, want)
	}
}

func TestFailoverWithoutCarryContextRepeatsTheOriginalArguments(t *testing.T) {
	f := newFixture(t, false)
	f.runner.CarryContext = false
	args := []string{"-p", "--output-format", "stream-json", "hello"}
	if code, err := f.runner.Run(context.Background(), f.runner.Profiles[0], args); err != nil || code != 0 {
		t.Fatalf("Run = %d, %v", code, err)
	}
	if got, _ := f.lastReport(t); !slices.Equal(got, args) {
		t.Errorf("second attempt args = %v, want %v", got, args)
	}
}

func TestAllProfilesLimited(t *testing.T) {
	f := newFixture(t, false)
	f.runner.Profiles[1].Dir += "-limited"
	_, err := f.runner.Run(context.Background(), f.runner.Profiles[0], []string{"-p", "hello"})
	var locked *selector.AllLockedError
	if !errors.As(err, &locked) {
		t.Fatalf("err = %v, want AllLockedError", err)
	}
	if len(f.attempts) != 2 {
		t.Errorf("%d attempts, want 2", len(f.attempts))
	}
}

func TestOtherFailureDoesNotSwitch(t *testing.T) {
	f := newFixture(t, false)
	f.runner.Launcher.Environ = append(f.runner.Launcher.Environ, "FAKE_EXIT=3", "FAKE_LIMIT_DIR=nomatch")
	code, err := f.runner.Run(context.Background(), f.runner.Profiles[0], []string{"-p", "hello"})
	if err != nil || code != 3 {
		t.Fatalf("Run = %d, %v; want exit code 3", code, err)
	}
	if len(f.attempts) != 1 {
		t.Errorf("%d attempts, want 1", len(f.attempts))
	}
}

func TestSuccessDoesNotSwitch(t *testing.T) {
	f := newFixture(t, false)
	code, err := f.runner.Run(context.Background(), f.runner.Profiles[1], []string{"-p", "hello"})
	if err != nil || code != 0 {
		t.Fatalf("Run = %d, %v", code, err)
	}
	if len(f.attempts) != 1 || f.notice.Len() != 0 {
		t.Errorf("attempts = %v, notice = %q", f.attempts, f.notice.String())
	}
}

func TestReplayableStdin(t *testing.T) {
	s := newReplayableStdin(strings.NewReader("prompt from stdin"))
	first, err := io.ReadAll(s.reader())
	if err != nil {
		t.Fatal(err)
	}
	second, err := io.ReadAll(s.reader())
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != "prompt from stdin" || string(second) != "prompt from stdin" {
		t.Errorf("first = %q, second = %q", first, second)
	}
	if newReplayableStdin(nil).reader() != nil {
		t.Error("nil stdin must stay nil")
	}
}
