// Package hooks implements the commands claude calls for a profile (statusLine
// and StopFailure) and builds the settings that wire them in per run.
package hooks

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
)

// Settings returns the JSON passed to `claude --settings` so that claude calls
// back into exe for profile. User hooks stay active; the statusLine replaces
// the user's, which the statusline hook runs itself (see Statusline).
func Settings(exe, profile string) (string, error) {
	base := quote(exe)
	settings := map[string]any{
		"hooks": map[string]any{
			"StopFailure": []any{
				map[string]any{
					"matcher": "rate_limit",
					"hooks": []any{
						map[string]any{"type": "command", "command": fmt.Sprintf("%s hook stop-failure --profile %s", base, profile)},
					},
				},
			},
		},
		"statusLine": map[string]any{
			"type":    "command",
			"command": fmt.Sprintf("%s hook statusline --profile %s", base, profile),
		},
	}
	data, err := json.Marshal(settings)
	if err != nil {
		return "", fmt.Errorf("encode settings: %w", err)
	}
	return string(data), nil
}

// quote protects a path for the shell claude runs hook commands in.
func quote(path string) string {
	if runtime.GOOS == "windows" {
		return `"` + path + `"`
	}
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}
