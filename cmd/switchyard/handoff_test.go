package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LuGG000/switchyard/internal/profiles"
	"github.com/LuGG000/switchyard/internal/state"
)

// handoffEnv creates profiles a and b below a temp data dir and makes a active.
func handoffEnv(t *testing.T, configToml string) *state.Store {
	t.Helper()
	dataDir, configDir := t.TempDir(), t.TempDir()
	t.Setenv("SWITCHYARD_DATA_DIR", dataDir)
	t.Setenv("SWITCHYARD_CONFIG_DIR", configDir)
	if configToml != "" {
		if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(configToml), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	m := &profiles.Manager{Root: filepath.Join(dataDir, profilesSubdir)}
	for _, name := range []string{"a", "b"} {
		if _, err := m.Create(name); err != nil {
			t.Fatal(err)
		}
	}
	store := state.NewStore(filepath.Join(dataDir, stateFile))
	if err := store.Update(func(st *state.State) error { st.Active = "a"; return nil }); err != nil {
		t.Fatal(err)
	}
	return store
}

func runHandoff(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs(append([]string{"handoff"}, args...))
	err := cmd.Execute()
	return out.String(), err
}

func TestHandoffWritesRequestForActiveProfile(t *testing.T) {
	tests := []struct {
		name      string
		config    string
		args      []string
		wantCarry bool
	}{
		{"carry_context default on", "", []string{"b"}, true},
		{"carry_context off", "carry_context = false\n", []string{"b"}, false},
		{"--fresh wins over config", "", []string{"b", "--fresh"}, false},
		{"--resume wins over config", "carry_context = false\n", []string{"b", "--resume"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := handoffEnv(t, tt.config)
			out, err := runHandoff(t, append(tt.args, "--session", "s1")...)
			if err != nil || !strings.Contains(out, "Handoff to b requested") {
				t.Fatalf("handoff = %q, %v", out, err)
			}
			st, _ := store.Read()
			req := st.SwitchRequest
			if req == nil || req.Profile != "a" || req.Target != "b" || req.Reason != state.ReasonManual || req.SessionID != "s1" {
				t.Fatalf("request = %+v", req)
			}
			if req.Carry == nil || *req.Carry != tt.wantCarry {
				t.Errorf("carry = %v, want %v", req.Carry, tt.wantCarry)
			}
		})
	}
}

func TestHandoffRejectsBadTargets(t *testing.T) {
	store := handoffEnv(t, "")
	for _, name := range []string{"a", "zzz"} {
		if _, err := runHandoff(t, name); err == nil {
			t.Errorf("handoff to %q did not fail", name)
		}
	}
	st, _ := store.Read()
	if st.SwitchRequest != nil {
		t.Errorf("a rejected handoff left request %+v", st.SwitchRequest)
	}
}

func TestHandoffWithoutActiveProfile(t *testing.T) {
	store := handoffEnv(t, "")
	if err := store.Update(func(st *state.State) error { st.Active = ""; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := runHandoff(t, "b"); err == nil {
		t.Error("handoff without an active profile did not fail")
	}
}
