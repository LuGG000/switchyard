package claudeenv

import (
	"slices"
	"testing"
)

func TestForProfileRemovesCredentialsAndSetsConfigDir(t *testing.T) {
	base := []string{
		"PATH=/usr/bin",
		"ANTHROPIC_API_KEY=secret",
		"anthropic_auth_token=secret",
		"CLAUDE_CODE_OAUTH_TOKEN=secret",
		"CLAUDE_CODE_USE_BEDROCK=1",
		"CLAUDE_CONFIG_DIR=/other",
		"HOME=/home/u",
	}
	got := ForProfile(base, "/profiles/a")
	want := []string{"PATH=/usr/bin", "HOME=/home/u", "CLAUDE_CONFIG_DIR=/profiles/a"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestForProfileDoesNotModifyInput(t *testing.T) {
	base := []string{"ANTHROPIC_API_KEY=secret", "PATH=/bin"}
	_ = ForProfile(base, "/p")
	if base[0] != "ANTHROPIC_API_KEY=secret" || len(base) != 2 {
		t.Errorf("input modified: %v", base)
	}
}

func TestForProfileKeepsEntriesWithoutValue(t *testing.T) {
	got := ForProfile([]string{"=C:=C:\\dir"}, "/p")
	if !slices.Contains(got, "=C:=C:\\dir") {
		t.Errorf("Windows drive entry dropped: %v", got)
	}
}
