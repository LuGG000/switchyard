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

func TestWriteDefaultCreatesLoadableFileOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.toml")
	written, err := WriteDefault(path)
	if err != nil || !written {
		t.Fatalf("first write: %t, %v", written, err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != Default().Mode || len(cfg.Link) != len(Default().Link) {
		t.Errorf("round trip differs: %+v", cfg)
	}

	if err := os.WriteFile(path, []byte("mode = \"auto\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	written, err = WriteDefault(path)
	if err != nil || written {
		t.Fatalf("second write: %t, %v", written, err)
	}
	if cfg, _ := Load(path); cfg.Mode != ModeAuto {
		t.Error("existing config was overwritten")
	}
}

func TestSetChangesOneLineAndKeepsTheRest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	original := "# my notes\nmode = 'ask'\ncarry_context = true\nstrategy = 'sequential'\nproactive_threshold = 0\nsource_dir = '/x'\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, change := range [][2]string{{KeyMode, "auto"}, {KeyCarryContext, "false"}, {KeyProactiveThreshold, "90"}, {KeyStrategy, "round-robin"}} {
		if err := Set(path, change[0], change[1]); err != nil {
			t.Fatalf("Set %v: %v", change, err)
		}
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != ModeAuto || got.CarryContext || got.ProactiveThreshold != 90 || got.Strategy != StrategyRoundRobin || got.SourceDir != "/x" {
		t.Errorf("config = %+v", got)
	}
	data, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(data), "# my notes\nmode = \"auto\"\n") {
		t.Errorf("file = %q", data)
	}
}

func TestSetKeepsWindowsLineEndings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("mode = 'ask'\r\nstrategy = 'sequential'\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Set(path, KeyMode, "auto"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "mode = \"auto\"\r\nstrategy = 'sequential'\r\n" {
		t.Errorf("file = %q", data)
	}
}

func TestSetAddsAMissingKeyAndCreatesAMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("mode = 'ask'"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Set(path, KeyProactiveThreshold, "80"); err != nil {
		t.Fatal(err)
	}
	if got, _ := Load(path); got.ProactiveThreshold != 80 {
		t.Errorf("threshold = %d", got.ProactiveThreshold)
	}

	fresh := filepath.Join(t.TempDir(), "sub", "config.toml")
	if err := Set(fresh, KeyMode, "auto"); err != nil {
		t.Fatal(err)
	}
	if got, _ := Load(fresh); got.Mode != ModeAuto || !got.CarryContext {
		t.Errorf("config = %+v, want auto with the defaults otherwise", got)
	}
}

func TestSetRejectsBadInputWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("mode = 'ask'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, change := range [][2]string{{KeyMode, "sometimes"}, {KeyCarryContext, "maybe"}, {KeyProactiveThreshold, "150"}, {KeyProactiveThreshold, "x"}, {"colour", "red"}, {"link", "a"}} {
		if err := Set(path, change[0], change[1]); err == nil {
			t.Errorf("Set %v did not fail", change)
		}
	}
	if data, _ := os.ReadFile(path); string(data) != "mode = 'ask'\n" {
		t.Errorf("file changed: %q", data)
	}
}

func TestSetColorsAcceptsNamesThemeKeysAndHex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	for _, change := range [][2]string{
		{KeyColorBackground, "#1e1e1e"}, {KeyColorText, "white"}, {KeyColorLow, "success"}, {KeyColorBorder, "#abc"},
	} {
		if err := Set(path, change[0], change[1]); err != nil {
			t.Fatalf("Set %v: %v", change, err)
		}
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if p := got.Palette(); p.Background != "#1e1e1e" || p.Text != "white" || p.Low != "success" || p.Border != "#abc" || p.High != "" {
		t.Errorf("palette = %+v", p)
	}
	if err := Set(path, KeyColorText, ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := Load(path); got.Palette().Text != "" {
		t.Errorf("an empty value did not clear the color: %+v", got.Palette())
	}
}

func TestSetColorsRejectsOddValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	for _, value := range []string{"#12", "#12345678a", "rgb(1,2,3)", "red; rm", "two words", "a\nb", strings.Repeat("a", 33)} {
		if err := Set(path, KeyColorText, value); err == nil {
			t.Errorf("color %q was accepted", value)
		}
	}
}

func TestSetColorKeyDoesNotTouchSimilarKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("color_border = 'red'\ncolor_border_active = 'green'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Set(path, KeyColorBorder, "blue"); err != nil {
		t.Fatal(err)
	}
	got, _ := Load(path)
	if got.ColorBorder != "blue" || got.ColorBorderActive != "green" {
		t.Errorf("config = %+v", got)
	}
}

func TestSetPresetReplacesAllColors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := SetPreset(path, "dark"); err != nil {
		t.Fatal(err)
	}
	got, _ := Load(path)
	if p := got.Palette(); p.Background == "" || p.Text == "" || p.Low == "" || p.Medium == "" || p.High == "" || p.BorderActive == "" || p.Border == "" {
		t.Errorf("dark leaves colors empty: %+v", p)
	}
	if err := SetPreset(path, "default"); err != nil {
		t.Fatal(err)
	}
	if got, _ := Load(path); got.Palette() != (Colors{}) {
		t.Errorf("default left colors: %+v", got.Palette())
	}
	if err := SetPreset(path, "neon"); err == nil {
		t.Error("an unknown palette was accepted")
	}
}

func TestUpdateCheckIsOnByDefaultAndCanBeTurnedOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := Load(path)
	if err != nil || !cfg.UpdateCheck {
		t.Fatalf("default: %+v, %v", cfg, err)
	}
	if err := Set(path, KeyUpdateCheck, "false"); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(path)
	if err != nil || cfg.UpdateCheck || !cfg.CarryContext {
		t.Fatalf("after set: %+v, %v (carry_context must stay untouched)", cfg, err)
	}
}
