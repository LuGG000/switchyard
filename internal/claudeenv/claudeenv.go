// Package claudeenv builds the environment for child claude processes.
//
// Credentials in the environment take precedence over a config dir login, so
// they are removed to make sure the subscription login of the profile is used.
package claudeenv

import "strings"

// ConfigDirVar names the variable that selects a claude config directory.
const ConfigDirVar = "CLAUDE_CONFIG_DIR"

// scrubbed lists variables that override or redirect the subscription login.
var scrubbed = []string{
	"ANTHROPIC_API_KEY",
	"ANTHROPIC_AUTH_TOKEN",
	"CLAUDE_CODE_OAUTH_TOKEN",
	"CLAUDE_CODE_USE_BEDROCK",
	"CLAUDE_CODE_USE_VERTEX",
	"CLAUDE_CODE_USE_FOUNDRY",
	"AWS_BEARER_TOKEN_BEDROCK",
	"ANTHROPIC_BEDROCK_BASE_URL",
	"ANTHROPIC_VERTEX_PROJECT_ID",
	"ANTHROPIC_VERTEX_BASE_URL",
	"ANTHROPIC_FOUNDRY_API_KEY",
	"ANTHROPIC_FOUNDRY_RESOURCE",
	"ANTHROPIC_FOUNDRY_BASE_URL",
}

// ForProfile returns base without credential variables and with
// CLAUDE_CONFIG_DIR set to configDir. Names are matched case-insensitively
// because Windows treats them that way.
func ForProfile(base []string, configDir string) []string {
	env := make([]string, 0, len(base)+1)
	for _, entry := range base {
		name, _, _ := strings.Cut(entry, "=")
		if isDropped(name) {
			continue
		}
		env = append(env, entry)
	}
	return append(env, ConfigDirVar+"="+configDir)
}

func isDropped(name string) bool {
	if strings.EqualFold(name, ConfigDirVar) {
		return true
	}
	for _, v := range scrubbed {
		if strings.EqualFold(name, v) {
			return true
		}
	}
	return false
}
