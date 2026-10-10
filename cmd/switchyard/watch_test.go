package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/LuGG000/switchyard/internal/config"
	"github.com/LuGG000/switchyard/internal/history"
	"github.com/LuGG000/switchyard/internal/state"
)

func TestWriteWatchShowsStatusSettingsAndSwitches(t *testing.T) {
	now := time.Date(2026, 10, 10, 14, 2, 11, 0, time.Local)
	report := statusReport{Version: "0.2.0", Active: "main", Profiles: []profileStatus{
		{Name: "main", Active: true, FiveHour: &usageInfo{UsedPercent: 42}},
		{Name: "zweit"},
	}}
	st := state.State{AutoSwitches: []time.Time{now.Add(-2 * time.Minute), now.Add(-30 * time.Hour)}}
	cfg := config.Default()
	cfg.ProactiveThreshold = 90
	recent := []history.Entry{{Time: now.Add(-2 * time.Minute), From: "zweit", To: "main", Reason: "rate_limit", How: history.Auto, Carry: true}}

	var out bytes.Buffer
	if err := writeWatch(&out, true, report, st, cfg, recent, now); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{
		"switchyard 0.2.0  14:02:11  (Ctrl+C quits)", "main", "42%", "zweit",
		"mode ask, strategy sequential, thresholds 90% / off",
		"Automatic switches in the last 24 hours: 1 of 6",
		"Automatic switching is paused: the last automatic switch was 2 minutes ago",
		"zweit -> main  rate_limit, auto, conversation continued",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the frame lacks %q:\n%s", want, text)
		}
	}
}

func TestWriteWatchWithoutSwitches(t *testing.T) {
	var out bytes.Buffer
	if err := writeWatch(&out, false, statusReport{Version: "x"}, state.State{}, config.Default(), nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "none yet") || !strings.Contains(out.String(), "No profiles yet") || strings.Contains(out.String(), "paused") {
		t.Errorf("frame:\n%s", out.String())
	}
}

func TestWatchWithoutATerminalPrintsOneSnapshot(t *testing.T) {
	dryRunWorld(t)
	out, err := runCLI(t, "watch")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Recent switches:") || !strings.Contains(out, "NAME") || strings.Contains(out, "\x1b") {
		t.Errorf("snapshot:\n%s", out)
	}
	if _, err := runCLI(t, "watch", "--interval", "10ms"); err == nil {
		t.Error("an interval below a second was accepted")
	}
}
