package main

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

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
		st.Profiles["b"] = state.Profile{LastUsed: used}
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
