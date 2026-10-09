// Package config loads and validates switchyard's config.toml.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

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
	// UpdateCheck lets status, list and doctor mention a newer release once a day.
	UpdateCheck bool `toml:"update_check"`
	// The color settings style the mod's accounts pane; empty means the Claude theme's color.
	ColorBackground   string `toml:"color_background"`
	ColorText         string `toml:"color_text"`
	ColorLow          string `toml:"color_low"`
	ColorMedium       string `toml:"color_medium"`
	ColorHigh         string `toml:"color_high"`
	ColorBorderActive string `toml:"color_border_active"`
	ColorBorder       string `toml:"color_border"`
	// SourceDir is the claude config dir whose entries are shared with profiles.
	// Empty means ~/.claude.
	SourceDir string `toml:"source_dir"`
	// Link lists the entries of a profile directory shared with the default one.
	Link []string `toml:"link"`
}

// Default returns the configuration used when no file exists.
func Default() Config {
	return Config{
		Mode:         ModeAsk,
		CarryContext: true,
		UpdateCheck:  true,
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
	if err := c.validateColors(); err != nil {
		return err
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

// ResolveSourceDir returns SourceDir, falling back to ~/.claude.
func (c Config) ResolveSourceDir() (string, error) {
	if c.SourceDir != "" {
		return c.SourceDir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home dir: %w", err)
	}
	return filepath.Join(home, ".claude"), nil
}

// WriteDefault writes the default configuration to path unless a file already
// exists there. It reports whether a file was written.
func WriteDefault(path string) (bool, error) {
	data, err := toml.Marshal(Default())
	if err != nil {
		return false, fmt.Errorf("encode config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, fmt.Errorf("create config dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("create config: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return false, fmt.Errorf("write config: %w", err)
	}
	if err := f.Close(); err != nil {
		return false, fmt.Errorf("write config: %w", err)
	}
	return true, nil
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

// Setting names Set can change.
const (
	KeyMode               = "mode"
	KeyCarryContext       = "carry_context"
	KeyStrategy           = "strategy"
	KeyProactiveThreshold = "proactive_threshold"
	KeyUpdateCheck        = "update_check"
)

// SettableKeys lists the settings Set can change, in display order.
var SettableKeys = append([]string{KeyMode, KeyCarryContext, KeyStrategy, KeyProactiveThreshold, KeyUpdateCheck}, ColorKeys...)

// Set changes one setting in the config file at path and leaves the rest of the
// file, comments included, as it is. The value is checked before anything is
// written. A missing file is created from the defaults first.
func Set(path, key, value string) error {
	return SetAll(path, [][2]string{{key, value}})
}

// SetAll changes several settings, each given as {key, value}, in one write.
// Nothing is written unless every change is valid.
func SetAll(path string, changes [][2]string) error {
	cfg, err := Load(path)
	if err != nil {
		return err
	}
	literals := make([]string, len(changes))
	for i, change := range changes {
		if literals[i], err = apply(&cfg, change[0], change[1]); err != nil {
			return err
		}
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	if _, err := WriteDefault(path); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	for i, change := range changes {
		data = replaceKey(data, change[0], literals[i])
	}
	return writeAtomic(path, data)
}

// apply sets the field for key and returns the value as a TOML literal.
func apply(cfg *Config, key, value string) (string, error) {
	switch key {
	case KeyMode:
		cfg.Mode = value
		return strconv.Quote(value), nil
	case KeyStrategy:
		cfg.Strategy = value
		return strconv.Quote(value), nil
	case KeyCarryContext, KeyUpdateCheck:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return "", fmt.Errorf("%s: %q is not true or false", key, value)
		}
		if key == KeyUpdateCheck {
			cfg.UpdateCheck = b
		} else {
			cfg.CarryContext = b
		}
		return strconv.FormatBool(b), nil
	case KeyProactiveThreshold:
		n, err := strconv.Atoi(value)
		if err != nil {
			return "", fmt.Errorf("%s: %q is not a whole number", key, value)
		}
		cfg.ProactiveThreshold = n
		return strconv.Itoa(n), nil
	}
	if cfg.setColor(key, value) {
		return strconv.Quote(value), nil
	}
	return "", fmt.Errorf("unknown setting %q (want one of %s)", key, strings.Join(SettableKeys, ", "))
}

// replaceKey sets `key = literal` on the first line that assigns key, or appends
// it. Line endings are kept.
func replaceKey(data []byte, key, literal string) []byte {
	pattern := regexp.MustCompile(`(?m)^([ \t]*)` + regexp.QuoteMeta(key) + `[ \t]*=.*?(\r?)$`)
	line := key + " = " + literal
	if pattern.Match(data) {
		replaced := false
		return pattern.ReplaceAllFunc(data, func(m []byte) []byte {
			if replaced {
				return m
			}
			replaced = true
			parts := pattern.FindSubmatch(m)
			return []byte(string(parts[1]) + line + string(parts[2]))
		})
	}
	if len(data) > 0 && !bytes.HasSuffix(data, []byte("\n")) {
		data = append(data, '\n')
	}
	return append(data, []byte(line+"\n")...)
}

// writeAtomic replaces the file at path through a temp file in the same directory.
func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}
