package linker

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func outcomeOf(results []Result, name string) Outcome {
	for _, r := range results {
		if r.Name == name {
			return r.Outcome
		}
	}
	return ""
}

func setup(t *testing.T) (source, profile string) {
	t.Helper()
	source, profile = t.TempDir(), t.TempDir()
	write(t, filepath.Join(source, "projects", "p1", "session.jsonl"), "one")
	write(t, filepath.Join(source, "CLAUDE.md"), "rules")
	return source, profile
}

func TestLinkSharesDirectoriesAndFiles(t *testing.T) {
	source, profile := setup(t)
	results, err := Link(source, profile, []string{"projects", "CLAUDE.md"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"projects", "CLAUDE.md"} {
		if got := outcomeOf(results, name); got != Linked {
			t.Errorf("%s: outcome %q, want %q", name, got, Linked)
		}
	}
	if got := read(t, filepath.Join(profile, "projects", "p1", "session.jsonl")); got != "one" {
		t.Errorf("shared directory not readable: %q", got)
	}
	// A write through the profile must show up in the source.
	write(t, filepath.Join(profile, "projects", "p1", "session.jsonl"), "two")
	if got := read(t, filepath.Join(source, "projects", "p1", "session.jsonl")); got != "two" {
		t.Errorf("write did not reach the source: %q", got)
	}
}

func TestLinkIsIdempotent(t *testing.T) {
	source, profile := setup(t)
	if _, err := Link(source, profile, []string{"projects", "CLAUDE.md"}); err != nil {
		t.Fatal(err)
	}
	results, err := Link(source, profile, []string{"projects", "CLAUDE.md"})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.Outcome != Unchanged {
			t.Errorf("%s: outcome %q, want %q", r.Name, r.Outcome, Unchanged)
		}
	}
}

func TestLinkSkipsMissingSource(t *testing.T) {
	source, profile := setup(t)
	results, err := Link(source, profile, []string{"skills"})
	if err != nil {
		t.Fatal(err)
	}
	if got := outcomeOf(results, "skills"); got != SourceMissing {
		t.Errorf("outcome %q, want %q", got, SourceMissing)
	}
	if _, err := os.Lstat(filepath.Join(profile, "skills")); err == nil {
		t.Error("an entry was created for a missing source")
	}
}

func TestLinkReplacesEmptyDirectory(t *testing.T) {
	source, profile := setup(t)
	if err := os.Mkdir(filepath.Join(profile, "projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	results, err := Link(source, profile, []string{"projects"})
	if err != nil {
		t.Fatal(err)
	}
	if got := outcomeOf(results, "projects"); got != Linked {
		t.Errorf("outcome %q, want %q", got, Linked)
	}
}

func TestLinkNeverOverwritesRealContent(t *testing.T) {
	source, profile := setup(t)
	write(t, filepath.Join(profile, "projects", "own", "session.jsonl"), "mine")
	write(t, filepath.Join(profile, "CLAUDE.md"), "my rules")

	results, err := Link(source, profile, []string{"projects", "CLAUDE.md"})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("got %v, want ErrConflict", err)
	}
	for _, name := range []string{"projects", "CLAUDE.md"} {
		if got := outcomeOf(results, name); got != Conflict {
			t.Errorf("%s: outcome %q, want %q", name, got, Conflict)
		}
	}
	if got := read(t, filepath.Join(profile, "projects", "own", "session.jsonl")); got != "mine" {
		t.Errorf("profile content changed: %q", got)
	}
	if got := read(t, filepath.Join(profile, "CLAUDE.md")); got != "my rules" {
		t.Errorf("profile file changed: %q", got)
	}
}

func TestLinkProcessesAllEntriesDespiteConflict(t *testing.T) {
	source, profile := setup(t)
	write(t, filepath.Join(profile, "CLAUDE.md"), "my rules")
	results, err := Link(source, profile, []string{"CLAUDE.md", "projects"})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("got %v, want ErrConflict", err)
	}
	if got := outcomeOf(results, "projects"); got != Linked {
		t.Errorf("projects: outcome %q, want %q", got, Linked)
	}
}

func TestLinkRejectsUnsafeNames(t *testing.T) {
	source, profile := setup(t)
	for _, name := range []string{"..", "a/b", `a\b`, ""} {
		if _, err := Link(source, profile, []string{name}); err == nil {
			t.Errorf("name %q was accepted", name)
		}
	}
}
