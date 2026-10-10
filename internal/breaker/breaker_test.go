package breaker

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/LuGG000/switchyard/internal/state"
)

var now = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

// switchesAgo lists switches the way Record stores them: oldest first.
func switchesAgo(minutes ...int) state.State {
	var st state.State
	for _, m := range minutes {
		st.AutoSwitches = append(st.AutoSwitches, now.Add(-time.Duration(m)*time.Minute))
	}
	return st
}

func TestCheck(t *testing.T) {
	limits := Limits{MinInterval: 10 * time.Minute, MaxPerDay: 3}
	tests := []struct {
		name   string
		limits Limits
		st     state.State
		until  time.Duration // zero: allowed; otherwise when it is allowed again, from now
	}{
		{"no switch yet", limits, switchesAgo(), 0},
		{"the last one is long enough ago", limits, switchesAgo(30), 0},
		{"the last one is too recent", limits, switchesAgo(3), 7 * time.Minute},
		{"the daily maximum is reached", limits, switchesAgo(600, 300, 60), 14 * time.Hour},
		{"old switches do not count", limits, switchesAgo(1500, 1450, 1441, 60), 0},
		{"the later of two limits applies", limits, switchesAgo(1300, 200, 2), 2*time.Hour + 20*time.Minute},
		{"both limits off", Limits{}, switchesAgo(5, 4, 3, 2, 1), 0},
		{"only the interval", Limits{MinInterval: 10 * time.Minute}, switchesAgo(5, 4, 3, 2, 1), 9 * time.Minute},
		{"only the daily maximum", Limits{MaxPerDay: 2}, switchesAgo(2, 1), 24*time.Hour - 2*time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.limits.Check(tt.st, now)
			if tt.until == 0 {
				if err != nil {
					t.Fatalf("Check = %v, want allowed", err)
				}
				return
			}
			var tripped *TrippedError
			if !errors.As(err, &tripped) {
				t.Fatalf("Check = %v, want a TrippedError", err)
			}
			if got := tripped.Until.Sub(now); got != tt.until {
				t.Errorf("allowed again in %v, want %v (%v)", got, tt.until, tripped)
			}
		})
	}
}

func TestRecordKeepsOnlyTheLastDay(t *testing.T) {
	store := state.NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Update(func(st *state.State) error {
		st.AutoSwitches = []time.Time{now.Add(-30 * time.Hour), now.Add(-2 * time.Hour)}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := Record(store, now); err != nil {
		t.Fatal(err)
	}
	st, err := store.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.AutoSwitches) != 2 || !st.AutoSwitches[0].Equal(now.Add(-2*time.Hour)) || !st.AutoSwitches[1].Equal(now) {
		t.Errorf("AutoSwitches = %v", st.AutoSwitches)
	}
}
