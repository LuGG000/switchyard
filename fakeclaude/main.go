// Command fakeclaude stands in for the claude executable in tests.
//
// It never talks to a network or an account. Behavior is driven by environment
// variables so tests can script outcomes:
//
//	FAKE_EXIT       exit code for a normal run (default 0)
//	FAKE_LOGGED_IN  "true" or "false" for `auth status` (default true)
//	FAKE_AUTH       authMethod for `auth status` (default claude.ai)
//	FAKE_LIMIT_DIR  if set and the config dir contains this text, the run hits a
//	                simulated rate limit: it prints the limit signal and exits 1
//
// A normal run prints a JSON report of what the process received, which tests
// decode to check arguments, config dir and that no credentials leaked.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Report is what a normal run prints on stdout.
type Report struct {
	Args      []string `json:"args"`
	ConfigDir string   `json:"configDir"`
	// EnvNames lists the names of credential variables that are set. Values are never printed.
	EnvNames []string `json:"envNames"`
}

var credentialVars = []string{
	"ANTHROPIC_API_KEY",
	"ANTHROPIC_AUTH_TOKEN",
	"CLAUDE_CODE_OAUTH_TOKEN",
	"CLAUDE_CODE_USE_BEDROCK",
	"CLAUDE_CODE_USE_VERTEX",
}

func main() {
	args := os.Args[1:]
	switch strings.Join(firstN(args, 2), " ") {
	case "auth status":
		os.Exit(authStatus())
	case "auth login":
		fmt.Println("fakeclaude: login in", os.Getenv("CLAUDE_CONFIG_DIR"))
		return
	}
	if marker := os.Getenv("FAKE_LIMIT_DIR"); marker != "" && strings.Contains(os.Getenv("CLAUDE_CONFIG_DIR"), marker) {
		os.Exit(hitLimit(args))
	}
	report := Report{Args: args, ConfigDir: os.Getenv("CLAUDE_CONFIG_DIR")}
	for _, name := range credentialVars {
		if os.Getenv(name) != "" {
			report.EnvNames = append(report.EnvNames, name)
		}
	}
	if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
		os.Exit(70)
	}
	code, err := strconv.Atoi(getenv("FAKE_EXIT", "0"))
	if err != nil {
		os.Exit(70)
	}
	os.Exit(code)
}

func authStatus() int {
	loggedIn := getenv("FAKE_LOGGED_IN", "true")
	fmt.Printf(`{"loggedIn":%s,"authMethod":%q,"subscriptionType":"pro","configDirectory":%q}`+"\n",
		loggedIn, getenv("FAKE_AUTH", "claude.ai"), os.Getenv("CLAUDE_CONFIG_DIR"))
	if loggedIn != "true" {
		return 1
	}
	return 0
}

// hitLimit prints what a headless run might print at a subscription limit and
// returns the exit code. The real output is not observed yet (spike #6).
func hitLimit(args []string) int {
	const message = "You've hit your session limit · resets 3:45pm"
	if !slices.Contains(args, "stream-json") {
		fmt.Println(message)
		return 1
	}
	resets := time.Now().Add(time.Hour).Unix()
	fmt.Println(`{"type":"system","subtype":"init","session_id":"fake-session"}`)
	fmt.Printf(`{"type":"rate_limit_event","rate_limit_info":{"status":"rejected","unifiedWindows":{"five_hour":{"utilization":1,"resetsAt":%d}}}}`+"\n", resets)
	fmt.Printf(`{"type":"result","is_error":true,"session_id":"fake-session","result":%q}`+"\n", message)
	return 1
}

func firstN(s []string, n int) []string {
	if len(s) < n {
		return s
	}
	return s[:n]
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
