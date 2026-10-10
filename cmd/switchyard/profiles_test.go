package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LuGG000/switchyard/internal/state"
)

func runRemove(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs(append([]string{"remove"}, args...))
	err := cmd.Execute()
	return out.String(), err
}

func TestRemoveAsksUnlessToldNotTo(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("SWITCHYARD_DATA_DIR", dataDir)
	dir := filepath.Join(dataDir, profilesSubdir, "zweit")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	store := state.NewStore(filepath.Join(dataDir, stateFile))
	err := store.Update(func(st *state.State) error {
		st.Active = "zweit"
		st.Profiles["zweit"] = state.Profile{}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := runRemove(t, "zweit"); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("without a terminal and --yes, err = %v, want a hint about --yes", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("the profile was removed without consent: %v", err)
	}

	if out, err := runRemove(t, "--yes", "zweit"); err != nil || !strings.Contains(out, "Removed profile zweit") {
		t.Fatalf("remove --yes = %q, %v", out, err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the profile directory is still there: %v", err)
	}
	st, err := store.Read()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st.Profiles["zweit"]; ok || st.Active != "" {
		t.Errorf("state still refers to the removed profile: %+v", st)
	}
}

func TestRemoveUnknownProfile(t *testing.T) {
	t.Setenv("SWITCHYARD_DATA_DIR", t.TempDir())
	if _, err := runRemove(t, "--yes", "nope"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %v, want profile not found", err)
	}
}
