// Command fakeclaude stands in for the claude executable in tests.
//
// It never talks to a network or an account. Behavior is driven by environment
// variables so tests can script outcomes:
//
//	FAKE_EXIT       exit code for a normal run (default 0)
//	FAKE_LOGGED_IN  "true" or "false" for `auth status` (default true)
//	FAKE_AUTH       authMethod for `auth status` (default claude.ai)
//
// A normal run prints a JSON report of what the process received, which tests
// decode to check arguments, config dir and that no credentials leaked.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
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
