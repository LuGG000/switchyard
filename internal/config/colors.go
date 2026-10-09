package config

import (
	"fmt"
	"regexp"
	"strings"
)

// Color settings. Empty means the Claude theme's color.
const (
	KeyColorBackground   = "color_background"
	KeyColorText         = "color_text"
	KeyColorLow          = "color_low"
	KeyColorMedium       = "color_medium"
	KeyColorHigh         = "color_high"
	KeyColorBorderActive = "color_border_active"
	KeyColorBorder       = "color_border"
)

// ColorKeys lists the color settings in display order.
var ColorKeys = []string{
	KeyColorBackground, KeyColorText, KeyColorLow, KeyColorMedium,
	KeyColorHigh, KeyColorBorderActive, KeyColorBorder,
}

// Colors are the colors the mod draws its pane with.
type Colors struct {
	// Background fills the whole pane.
	Background string `json:"background"`
	Text       string `json:"text"`
	// Low, Medium and High color the usage bars below 70%, from 70% and from 90%.
	Low          string `json:"low"`
	Medium       string `json:"medium"`
	High         string `json:"high"`
	BorderActive string `json:"border_active"`
	Border       string `json:"border"`
}

// Palette returns the configured colors.
func (c Config) Palette() Colors {
	return Colors{
		Background:   c.ColorBackground,
		Text:         c.ColorText,
		Low:          c.ColorLow,
		Medium:       c.ColorMedium,
		High:         c.ColorHigh,
		BorderActive: c.ColorBorderActive,
		Border:       c.ColorBorder,
	}
}

// colorPattern accepts a theme key or color name (letters, digits, - and _) or hex.
var colorPattern = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_-]{0,31}|#[0-9A-Fa-f]{3,8})$`)

func validColor(key, value string) error {
	if value != "" && !colorPattern.MatchString(value) {
		return fmt.Errorf("%s: %q is not a color name, theme color or hex like #1e1e1e", key, value)
	}
	return nil
}

func (c Config) validateColors() error {
	for _, kv := range [][2]string{
		{KeyColorBackground, c.ColorBackground}, {KeyColorText, c.ColorText},
		{KeyColorLow, c.ColorLow}, {KeyColorMedium, c.ColorMedium}, {KeyColorHigh, c.ColorHigh},
		{KeyColorBorderActive, c.ColorBorderActive}, {KeyColorBorder, c.ColorBorder},
	} {
		if err := validColor(kv[0], kv[1]); err != nil {
			return err
		}
	}
	return nil
}

// setColor sets the field of a color key.
func (c *Config) setColor(key, value string) bool {
	fields := map[string]*string{
		KeyColorBackground: &c.ColorBackground, KeyColorText: &c.ColorText,
		KeyColorLow: &c.ColorLow, KeyColorMedium: &c.ColorMedium, KeyColorHigh: &c.ColorHigh,
		KeyColorBorderActive: &c.ColorBorderActive, KeyColorBorder: &c.ColorBorder,
	}
	field, ok := fields[key]
	if ok {
		*field = value
	}
	return ok
}

// Palette presets by name; "default" clears every color so the theme applies.
var presets = map[string]Colors{
	"default": {},
	"dark": {
		Background: "#1e1e1e", Text: "#d4d4d4", Low: "#4ec9b0", Medium: "#dcdcaa",
		High: "#f48771", BorderActive: "#4ec9b0", Border: "#6a6a6a",
	},
	"light": {
		Background: "#f5f5f5", Text: "#1e1e1e", Low: "#2e7d32", Medium: "#b26a00",
		High: "#c62828", BorderActive: "#2e7d32", Border: "#9e9e9e",
	},
}

// PresetNames lists the palette presets.
func PresetNames() []string { return []string{"default", "dark", "light"} }

// SetPreset replaces all color settings in the config file at path by a preset.
func SetPreset(path, name string) error {
	p, ok := presets[name]
	if !ok {
		return fmt.Errorf("unknown palette %q (want one of %s)", name, strings.Join(PresetNames(), ", "))
	}
	values := map[string]string{
		KeyColorBackground: p.Background, KeyColorText: p.Text, KeyColorLow: p.Low,
		KeyColorMedium: p.Medium, KeyColorHigh: p.High, KeyColorBorderActive: p.BorderActive, KeyColorBorder: p.Border,
	}
	changes := make([][2]string, 0, len(ColorKeys))
	for _, key := range ColorKeys {
		changes = append(changes, [2]string{key, values[key]})
	}
	return SetAll(path, changes)
}
