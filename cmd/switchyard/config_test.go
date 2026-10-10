package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func runConfig(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs(append([]string{"config"}, args...))
	err := cmd.Execute()
	return out.String(), err
}

func TestConfigShowsAndSetsSettings(t *testing.T) {
	t.Setenv("SWITCHYARD_CONFIG_DIR", t.TempDir())

	out, err := runConfig(t, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var report configReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if report.Schema != configSchema || report.Mode != "ask" || !report.CarryContext {
		t.Errorf("defaults = %+v", report)
	}

	if _, err := runConfig(t, "set", "mode", "auto"); err != nil {
		t.Fatal(err)
	}
	if _, err := runConfig(t, "set", "carry_context", "false"); err != nil {
		t.Fatal(err)
	}
	out, err = runConfig(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"mode", "auto", "carry_context", "false"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q misses %q", out, want)
		}
	}
}

func TestConfigSetRejectsBadValues(t *testing.T) {
	t.Setenv("SWITCHYARD_CONFIG_DIR", t.TempDir())
	for _, args := range [][]string{{"set", "mode", "sometimes"}, {"set", "nope", "1"}, {"set", "mode"}} {
		if _, err := runConfig(t, args...); err == nil {
			t.Errorf("config %v did not fail", args)
		}
	}
}

func TestConfigColors(t *testing.T) {
	t.Setenv("SWITCHYARD_CONFIG_DIR", t.TempDir())

	if _, err := runConfig(t, "colors", "dark"); err != nil {
		t.Fatal(err)
	}
	out, err := runConfig(t, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var report configReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if report.Colors.Background == "" || report.Colors.High == "" {
		t.Errorf("colors = %+v, want the dark palette", report.Colors)
	}

	if _, err := runConfig(t, "set", "color_background", "#101010"); err != nil {
		t.Fatal(err)
	}
	if _, err := runConfig(t, "colors", "default"); err != nil {
		t.Fatal(err)
	}
	text, _ := runConfig(t)
	if !strings.Contains(text, "color_background             (theme)") {
		t.Errorf("output %q does not show the theme default", text)
	}
	if _, err := runConfig(t, "colors", "neon"); err == nil {
		t.Error("an unknown palette was accepted")
	}
}
