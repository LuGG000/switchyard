package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"time"
)

// interactiveWait is how long a simulated interactive session stays at its
// prompt unless the launcher ends it.
const interactiveWait = 20 * time.Second

// settings is the part of the --settings JSON that fakeclaude acts on.
type settings struct {
	Hooks struct {
		StopFailure []struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"StopFailure"`
	} `json:"hooks"`
	StatusLine struct {
		Command string `json:"command"`
	} `json:"statusLine"`
}

// interactiveLimit simulates an interactive session that hits its limit: the
// StopFailure hook fires, then claude waits at its prompt for the launcher to
// end it. It returns the exit code if nobody does.
func interactiveLimit(args []string) int {
	s := parseSettings(args)
	if len(s.Hooks.StopFailure) > 0 && len(s.Hooks.StopFailure[0].Hooks) > 0 {
		runHook(s.Hooks.StopFailure[0].Hooks[0].Command, `{"session_id":"fake-session","hook_event_name":"StopFailure"}`)
	}
	return wait()
}

// interactiveThreshold simulates a turn that ends with the five-hour window
// nearly used up: the statusLine command receives the usage, then claude waits.
func interactiveThreshold(args []string) int {
	s := parseSettings(args)
	resets := time.Now().Add(2 * time.Hour).Unix()
	input, _ := json.Marshal(map[string]any{
		"session_id":  "fake-session",
		"rate_limits": map[string]any{"five_hour": map[string]any{"used_percentage": 96, "resets_at": resets}},
	})
	runHook(s.StatusLine.Command, string(input))
	return wait()
}

func parseSettings(args []string) settings {
	var s settings
	for i, a := range args {
		if a == "--settings" && i+1 < len(args) {
			_ = json.Unmarshal([]byte(args[i+1]), &s)
		}
	}
	return s
}

func wait() int {
	time.Sleep(interactiveWait)
	return 5
}

// runHook runs a hook command with input on stdin. The command is the
// executable, quoted if its path needs it, followed by plain arguments.
func runHook(command, input string) {
	if command == "" {
		return
	}
	name, rest := command, ""
	if q := command[0]; q == '"' || q == '\'' {
		end := strings.IndexByte(command[1:], q)
		name, rest = command[1:end+1], command[end+2:]
	} else if i := strings.IndexByte(command, ' '); i >= 0 {
		name, rest = command[:i], command[i:]
	}
	cmd := exec.Command(name, strings.Fields(rest)...)
	cmd.Stdin = strings.NewReader(input)
	cmd.Env = os.Environ()
	_ = cmd.Run()
}
