package main

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/LuGG000/switchyard/internal/breaker"
	"github.com/LuGG000/switchyard/internal/selector"
)

func TestVersionFlag(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "switchyard "+version+"\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExitCodes(t *testing.T) {
	locked := fmt.Errorf("switch: %w", &selector.AllLockedError{EarliestReset: time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)})
	tests := []struct {
		name    string
		err     error
		code    int
		message bool
	}{
		{"success", nil, 0, false},
		{"claude's own code passes through", exitError{code: 3}, 3, false},
		{"every profile at its limit", locked, exitAllLimited, true},
		{"the switch limits stop a run", fmt.Errorf("profile main reached its limit: %w", &breaker.TrippedError{Reason: "x", Until: time.Now()}), exitSwitchPaused, true},
		{"anything else", errors.New("boom"), 1, true},
	}
	for _, tt := range tests {
		code, message := exitCodeFor(tt.err)
		if code != tt.code || (message != "") != tt.message {
			t.Errorf("%s: got %d %q, want %d (message: %v)", tt.name, code, message, tt.code, tt.message)
		}
	}
}

func TestShellInitPrintsAFunctionPerShell(t *testing.T) {
	for _, shell := range shellNames() {
		var out bytes.Buffer
		cmd := newRootCmd()
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"shell-init", shell})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%s: %v", shell, err)
		}
		if !strings.Contains(out.String(), "switchyard run --") || !strings.Contains(out.String(), "claude") {
			t.Errorf("%s: %q", shell, out.String())
		}
	}

	cmd := newRootCmd()
	cmd.SetArgs([]string{"shell-init", "nushell"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "bash|fish|powershell|zsh") {
		t.Errorf("an unknown shell should list the known ones, got %v", err)
	}
}
