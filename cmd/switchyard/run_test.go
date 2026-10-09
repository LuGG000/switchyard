package main

import (
	"path/filepath"
	"testing"

	"github.com/LuGG000/switchyard/internal/profiles"
	"github.com/LuGG000/switchyard/internal/state"
)

func TestResolveProfile(t *testing.T) {
	m := &profiles.Manager{Root: t.TempDir()}
	for _, name := range []string{"a", "b"} {
		if _, err := m.Create(name); err != nil {
			t.Fatal(err)
		}
	}
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))

	if _, err := resolveProfile(&profiles.Manager{Root: t.TempDir()}, store, ""); err == nil {
		t.Error("expected an error without profiles")
	}
	if p, _ := resolveProfile(m, store, ""); p.Name != "a" {
		t.Errorf("fallback: got %q, want a", p.Name)
	}
	if err := store.Update(func(st *state.State) error { st.Active = "b"; return nil }); err != nil {
		t.Fatal(err)
	}
	if p, _ := resolveProfile(m, store, ""); p.Name != "b" {
		t.Errorf("active: got %q, want b", p.Name)
	}
	if p, _ := resolveProfile(m, store, "a"); p.Name != "a" {
		t.Errorf("explicit: got %q, want a", p.Name)
	}
	if _, err := resolveProfile(m, store, "zzz"); err == nil {
		t.Error("expected an error for an unknown profile")
	}
}
