package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/LuGG000/switchyard/internal/history"
)

func TestHistoryCommand(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("SWITCHYARD_DATA_DIR", dataDir)

	if out, err := runCLI(t, "history"); err != nil || !strings.Contains(out, "No switches yet.") {
		t.Fatalf("empty history: %v\n%s", err, out)
	}
	log := history.New(filepath.Join(dataDir, historyFile))
	for i, how := range []string{history.Auto, history.Asked, history.Handoff} {
		e := history.Entry{Time: time.Date(2026, 10, 10, 12, i, 0, 0, time.UTC), From: "main", To: "zweit", Reason: "rate_limit", How: how, Carry: i != 1}
		if err := log.Append(e); err != nil {
			t.Fatal(err)
		}
	}

	out, err := runCLI(t, "history", "-n", "2")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "auto") || !strings.Contains(out, "asked") || !strings.Contains(out, "handoff") || !strings.Contains(out, "new") || !strings.Contains(out, "continued") {
		t.Errorf("history -n 2:\n%s", out)
	}
	out, err = runCLI(t, "history", "--json")
	if err != nil || !strings.Contains(out, `"how": "auto"`) || !strings.Contains(out, `"to": "zweit"`) {
		t.Errorf("history --json: %v\n%s", err, out)
	}
}
