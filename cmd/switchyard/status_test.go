package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/LuGG000/switchyard/internal/config"
	"github.com/LuGG000/switchyard/internal/profiles"
	"github.com/LuGG000/switchyard/internal/state"
)

func TestBuildStatus(t *testing.T) {
	m := &profiles.Manager{Root: t.TempDir()}
	for _, name := range []string{"a", "b"} {
		if _, err := m.Create(name); err != nil {
			t.Fatal(err)
		}
	}
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	used := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	err := store.Update(func(st *state.State) error {
		st.Active = "b"
		st.Profiles["b"] = state.Profile{LastUsed: used, FiveHour: &state.Usage{UsedPercent: 63, ResetsAt: used}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	report, err := buildStatus(m, store)
	if err != nil {
		t.Fatal(err)
	}
	if report.Schema != statusSchema || report.Active != "b" || len(report.Profiles) != 2 {
		t.Fatalf("unexpected report: %+v", report)
	}
	a, b := report.Profiles[0], report.Profiles[1]
	if a.Active || a.LastUsed != nil || !b.Active || b.LastUsed == nil || !b.LastUsed.Equal(used) {
		t.Errorf("unexpected profile entries: %+v %+v", a, b)
	}

	if b.FiveHour == nil || b.FiveHour.UsedPercent != 63 || a.FiveHour != nil {
		t.Errorf("usage not reported: %+v %+v", a.FiveHour, b.FiveHour)
	}

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"schema", "version", "active", "profiles"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("JSON key %q missing", key)
		}
	}
}

func TestBuildStatusWithoutProfilesIsEmptyList(t *testing.T) {
	report, err := buildStatus(&profiles.Manager{Root: t.TempDir()}, state.NewStore(filepath.Join(t.TempDir(), "s.json")))
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(report)
	var raw struct {
		Profiles []any `json:"profiles"`
	}
	if err := json.Unmarshal(data, &raw); err != nil || raw.Profiles == nil {
		t.Errorf("profiles must encode as [], got %s", data)
	}
}

func TestPendingUpdateComesFromTheCache(t *testing.T) {
	t.Setenv("SWITCHYARD_DATA_DIR", t.TempDir())
	t.Setenv("SWITCHYARD_CONFIG_DIR", t.TempDir())
	old := version
	version = "0.1.0"
	t.Cleanup(func() { version = old })

	if got := pendingUpdate(); got != nil {
		t.Fatalf("nothing cached, got %+v", got)
	}
	dir, _ := config.DataDir()
	cache := `{"checked_at":"2026-10-10T10:00:00Z","version":"0.2.0","url":"https://example.test/v0.2.0"}`
	if err := os.WriteFile(filepath.Join(dir, "update-check.json"), []byte(cache), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := pendingUpdate(); got == nil || got.Version != "0.2.0" || got.URL != "https://example.test/v0.2.0" {
		t.Fatalf("pendingUpdate = %+v", got)
	}

	cache = `{"checked_at":"2026-10-10T10:00:00Z","version":"0.1.0","url":"u"}`
	if err := os.WriteFile(filepath.Join(dir, "update-check.json"), []byte(cache), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := pendingUpdate(); got != nil {
		t.Fatalf("same version is not an update, got %+v", got)
	}

	t.Setenv(noUpdateCheckEnv, "1")
	if got := pendingUpdate(); got != nil {
		t.Fatalf("opt-out must hide the update, got %+v", got)
	}
}

func TestShortStatus(t *testing.T) {
	now := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
	later := now.Add(time.Hour)
	earlier := now.Add(-time.Hour)
	usage := func(p float64) *usageInfo { return &usageInfo{UsedPercent: p} }
	tests := []struct {
		name     string
		profiles []profileStatus
		want     string
	}{
		{"no profile", nil, ""},
		{"none active", []profileStatus{{Name: "a"}}, ""},
		{"no data yet", []profileStatus{{Name: "a", Active: true}}, "a"},
		{"usage", []profileStatus{{Name: "a", Active: true, FiveHour: usage(33), SevenDay: usage(19.4)}}, "a 5h 33% 7d 19%"},
		{"five-hour only", []profileStatus{{Name: "a", Active: true, FiveHour: usage(5)}}, "a 5h 5%"},
		{"cooling down", []profileStatus{{Name: "a", Active: true, FiveHour: usage(100), CooldownUntil: &later}}, "a limit until " + later.Local().Format("15:04")},
		{"cooldown over", []profileStatus{{Name: "a", Active: true, FiveHour: usage(1), CooldownUntil: &earlier}}, "a 5h 1%"},
		{"another profile cools down", []profileStatus{{Name: "a", Active: true}, {Name: "b", CooldownUntil: &later}}, "a"},
	}
	for _, tt := range tests {
		if got := shortStatus(statusReport{Profiles: tt.profiles}, now); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}
