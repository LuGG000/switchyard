// Package config loads and validates switchyard's config.toml.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// Failover modes.
const (
	ModeAsk  = "ask"
	ModeAuto = "auto"
)

// Profile selection strategies.
const (
	StrategySequential   = "sequential"
	StrategyMostHeadroom = "most-headroom"
	StrategyRoundRobin   = "round-robin"
)

const (
	configDirEnv = "SWITCHYARD_CONFIG_DIR"
	dataDirEnv   = "SWITCHYARD_DATA_DIR"
	appName      = "switchyard"
	fileName     = "config.toml"
)

// Config holds the user-editable settings.
type Config struct {
	// Mode decides whether a limit hit switches profiles automatically or asks first.
	Mode string `toml:"mode"`
	// CarryContext resumes the current session in the new profile on a switch.
	CarryContext bool `toml:"carry_context"`
	// Strategy selects the next profile when switching.
	Strategy string `toml:"strategy"`
	// ContinuePrompt is sent after a resumed session starts. Empty sends nothing.
	ContinuePrompt string `toml:"continue_prompt"`
	// ProactiveThreshold switches at this five-hour usage percentage. Zero disables it.
	ProactiveThreshold int `toml:"proactive_threshold"`
	// Link lists the entries of a profile directory shared with the default one.
	Link []string `toml:"link"`
}

// Default returns the configuration used when no file exists.
func Default() Config {
	return Config{
		Mode:         ModeAsk,
		CarryContext: true,
		Strategy:     StrategySequential,
		Link:         []string{"projects", "settings.json", "CLAUDE.md", "skills", "agents", "commands", "plugins"},
	}
}

// Validate reports the first invalid setting.
func (c Config) Validate() error {
	switch c.Mode {
	case ModeAsk, ModeAuto:
	default:
		return fmt.Errorf("mode: unknown value %q (want %q or %q)", c.Mode, ModeAsk, ModeAuto)
	}
	switch c.Strategy {
	case StrategySequential, StrategyMostHeadroom, StrategyRoundRobin:
	default:
		return fmt.Errorf("strategy: unknown value %q", c.Strategy)
	}
	if c.ProactiveThreshold < 0 || c.ProactiveThreshold > 100 {
		return fmt.Errorf("proactive_threshold: %d out of range 0-100", c.ProactiveThreshold)
	}
	return nil
}

// Load reads the config file at path. A missing file yields the defaults.
func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// Path returns the location of config.toml.
func Path() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fileName), nil
}

// DataDir returns the directory holding profiles and state.json.
func DataDir() (string, error) {
	if dir := os.Getenv(dataDirEnv); dir != "" {
		return dir, nil
	}
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, appName), nil
	}
	if localAppData := os.Getenv("LocalAppData"); localAppData != "" {
		return filepath.Join(localAppData, appName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate data dir: %w", err)
	}
	return filepath.Join(home, ".local", "share", appName), nil
}

func configDir() (string, error) {
	if dir := os.Getenv(configDirEnv); dir != "" {
		return dir, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate config dir: %w", err)
	}
	return filepath.Join(base, appName), nil
}
