package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/LuGG000/switchyard/internal/state"
)

func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

// dryRunWorld makes profiles a and b below a temporary data dir and returns the state file path.
func dryRunWorld(t *testing.T) string {
	t.Helper()
	dataDir := t.TempDir()
	t.Setenv("SWITCHYARD_DATA_DIR", dataDir)
	t.Setenv("SWITCHYARD_CONFIG_DIR", t.TempDir())
	t.Setenv("ANTHROPIC_API_KEY", "must-never-be-printed")
	for _, name := range []string{"a", "b"} {
		if err := os.MkdirAll(filepath.Join(dataDir, profilesSubdir, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dataDir, stateFile)
}

func TestRunDryRunShowsThePlanAndChangesNothing(t *testing.T) {
	statePath := dryRunWorld(t)

	out, err := runCLI(t, "run", "--dry-run", "-p", "a", "--", "-p", "hello")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Profile:     a (", "headless", "If a hits its limit: b", "ANTHROPIC_API_KEY", "--settings", "hook stop-failure --profile a", "hello", "nothing was started"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "must-never-be-printed") {
		t.Error("the value of a credential variable was printed")
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Errorf("a dry run wrote the state file: %v", err)
	}
}

func TestDryRunNamesWhereAllProfilesAreLimited(t *testing.T) {
	statePath := dryRunWorld(t)
	err := state.NewStore(statePath).Update(func(st *state.State) error {
		st.Profiles["b"] = state.Profile{CooldownUntil: time.Now().Add(time.Hour)}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := runCLI(t, "run", "--dry-run", "-p", "a")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no other profile is available; the first one is back at") {
		t.Errorf("output = %s", out)
	}
}

func TestSwitchDryRun(t *testing.T) {
	statePath := dryRunWorld(t)

	out, err := runCLI(t, "switch", "--dry-run", "--resume", "b")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Profile:     b (") || !strings.Contains(out, "  --continue") || !strings.Contains(out, "interactive") {
		t.Errorf("output = %s", out)
	}
	out, err = runCLI(t, "switch", "--dry-run", "--fresh", "b")
	if err != nil || !strings.Contains(out, "A new conversation is started.") || strings.Contains(out, "--continue\n") {
		t.Errorf("--fresh: %v\n%s", err, out)
	}
	out, err = runCLI(t, "switch", "--dry-run", "--no-launch", "b")
	if err != nil || !strings.Contains(out, "Would make b the active profile") {
		t.Errorf("--no-launch: %v\n%s", err, out)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Errorf("a dry run wrote the state file: %v", err)
	}
	if _, err := runCLI(t, "switch", "--dry-run", "nope"); err == nil {
		t.Error("an unknown profile was accepted")
	}
}
