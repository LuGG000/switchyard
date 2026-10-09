//go:build !windows

package linker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinkRepairsDanglingLink(t *testing.T) {
	source, profile := setup(t)
	if err := os.Symlink(filepath.Join(t.TempDir(), "gone"), filepath.Join(profile, "projects")); err != nil {
		t.Fatal(err)
	}
	results, err := Link(source, profile, []string{"projects"})
	if err != nil {
		t.Fatal(err)
	}
	if got := outcomeOf(results, "projects"); got != Relinked {
		t.Errorf("outcome %q, want %q", got, Relinked)
	}
	if got := read(t, filepath.Join(profile, "projects", "p1", "session.jsonl")); got != "one" {
		t.Errorf("relinked directory not readable: %q", got)
	}
}

func TestLinkKeepsForeignValidLinkAsConflict(t *testing.T) {
	source, profile := setup(t)
	other := t.TempDir()
	if err := os.Symlink(other, filepath.Join(profile, "projects")); err != nil {
		t.Fatal(err)
	}
	results, err := Link(source, profile, []string{"projects"})
	if err == nil {
		t.Fatal("expected a conflict")
	}
	if got := outcomeOf(results, "projects"); got != Conflict {
		t.Errorf("outcome %q, want %q", got, Conflict)
	}
	if target, _ := os.Readlink(filepath.Join(profile, "projects")); target != other {
		t.Errorf("foreign link was modified: %q", target)
	}
}
