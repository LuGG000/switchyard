package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func entry(i int) Entry {
	return Entry{Time: time.Date(2026, 10, 10, 12, i, 0, 0, time.UTC), From: "a", To: "b", Reason: "rate_limit", How: Auto, Carry: true}
}

func TestAppendAndLast(t *testing.T) {
	log := New(filepath.Join(t.TempDir(), "history.jsonl"))
	if got, err := log.Last(5); err != nil || len(got) != 0 {
		t.Fatalf("a missing file = %v, %v, want an empty history", got, err)
	}
	for i := range 5 {
		if err := log.Append(entry(i)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := log.Last(3)
	if err != nil || len(got) != 3 || got[0].Time.Minute() != 2 || got[2].Time.Minute() != 4 {
		t.Fatalf("Last(3) = %v, %v, want the newest three, oldest first", got, err)
	}
	if got, _ := log.Last(0); len(got) != 5 {
		t.Errorf("Last(0) = %d entries, want all 5", len(got))
	}
}

func TestInvalidLinesAreSkipped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	if err := os.WriteFile(path, []byte("not json\n{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	log := New(path)
	if err := log.Append(entry(1)); err != nil {
		t.Fatal(err)
	}
	if got, err := log.Last(10); err != nil || len(got) != 1 {
		t.Errorf("Last = %v, %v, want only the valid entry", got, err)
	}
}

func TestTheFileIsCutBackWhenItGrows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	log := New(path)
	e := entry(1)
	for i := 0; i < 3000; i++ {
		e.Time = e.Time.Add(time.Minute)
		if err := log.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > maxBytes {
		t.Errorf("the file is %d bytes, want at most %d", info.Size(), maxBytes)
	}
	got, _ := log.Last(0)
	if len(got) < keepEntries || !got[len(got)-1].Time.Equal(e.Time) {
		t.Errorf("%d entries, newest %v: the newest entries must survive", len(got), got[len(got)-1].Time)
	}
}
