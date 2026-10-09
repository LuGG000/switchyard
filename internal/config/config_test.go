package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "absent.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != ModeAsk || !cfg.CarryContext || cfg.Strategy != StrategySequential {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadOverridesOnlyGivenKeys(t *testing.T) {
	cfg, err := Load(writeConfig(t, "mode = \"auto\"\ncarry_context = false\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != ModeAuto || cfg.CarryContext {
		t.Errorf("overrides not applied: %+v", cfg)
	}
	if cfg.Strategy != StrategySequential || len(cfg.Link) == 0 {
		t.Errorf("defaults lost: %+v", cfg)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	tests := map[string]string{
		"mode":      "mode = \"maybe\"",
		"strategy":  "strategy = \"random\"",
		"threshold": "proactive_threshold = 101",
		"syntax":    "mode = ",
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(writeConfig(t, content)); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func TestDataDirHonorsOverrides(t *testing.T) {
	t.Setenv(dataDirEnv, "")
	t.Setenv("XDG_DATA_HOME", "/xdg")
	got, err := DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join("/xdg", appName); got != want {
		t.Errorf("XDG: got %q, want %q", got, want)
	}

	t.Setenv(dataDirEnv, "/explicit")
	if got, _ = DataDir(); got != "/explicit" {
		t.Errorf("explicit override: got %q", got)
	}
}

func TestPathUsesConfigDirOverride(t *testing.T) {
	t.Setenv(configDirEnv, "/cfg")
	got, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, fileName) || !strings.Contains(got, "cfg") {
		t.Errorf("unexpected path %q", got)
	}
}

func TestResolveSourceDir(t *testing.T) {
	cfg := Default()
	cfg.SourceDir = "/custom"
	if got, _ := cfg.ResolveSourceDir(); got != "/custom" {
		t.Errorf("explicit: got %q", got)
	}
	cfg.SourceDir = ""
	got, err := cfg.ResolveSourceDir()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != ".claude" {
		t.Errorf("default: got %q", got)
	}
}
