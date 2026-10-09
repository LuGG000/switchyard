package selector

import (
	"errors"
	"testing"
	"time"

	"github.com/LuGG000/switchyard/internal/config"
	"github.com/LuGG000/switchyard/internal/state"
)

var now = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

func cooling(name string, until time.Duration) Candidate {
	return Candidate{Name: name, State: state.Profile{CooldownUntil: now.Add(until)}}
}

func weekly(name string, percent float64, resetsIn time.Duration) Candidate {
	return Candidate{Name: name, State: state.Profile{SevenDay: &state.Usage{UsedPercent: percent, ResetsAt: now.Add(resetsIn)}}}
}

func free(name string) Candidate { return Candidate{Name: name} }

func TestNext(t *testing.T) {
	tests := []struct {
		name       string
		strategy   string
		current    string
		candidates []Candidate
		want       string
	}{
		{"sequential stays on current", config.StrategySequential, "b",
			[]Candidate{free("a"), free("b"), free("c")}, "b"},
		{"sequential leaves a cooling current", config.StrategySequential, "a",
			[]Candidate{cooling("a", time.Hour), free("b"), free("c")}, "b"},
		{"sequential without current takes the first", config.StrategySequential, "",
			[]Candidate{free("b"), free("a")}, "a"},
		{"round robin takes the next", config.StrategyRoundRobin, "a",
			[]Candidate{free("a"), free("b"), free("c")}, "b"},
		{"round robin wraps around", config.StrategyRoundRobin, "c",
			[]Candidate{free("a"), free("b"), free("c")}, "a"},
		{"round robin skips cooling profiles", config.StrategyRoundRobin, "a",
			[]Candidate{free("a"), cooling("b", time.Hour), free("c")}, "c"},
		{"round robin returns to the only free one", config.StrategyRoundRobin, "a",
			[]Candidate{free("a"), cooling("b", time.Hour)}, "a"},
		{"most headroom takes the lowest weekly usage", config.StrategyMostHeadroom, "a",
			[]Candidate{weekly("a", 80, time.Hour), weekly("b", 20, time.Hour), weekly("c", 50, time.Hour)}, "b"},
		{"most headroom treats an unknown window as unused", config.StrategyMostHeadroom, "a",
			[]Candidate{weekly("a", 10, time.Hour), free("b")}, "b"},
		{"most headroom treats a reset window as unused", config.StrategyMostHeadroom, "a",
			[]Candidate{weekly("a", 10, time.Hour), weekly("b", 90, -time.Minute)}, "b"},
		{"most headroom ignores cooling profiles", config.StrategyMostHeadroom, "a",
			[]Candidate{weekly("a", 50, time.Hour), Candidate{Name: "b", State: state.Profile{
				CooldownUntil: now.Add(time.Hour), SevenDay: &state.Usage{UsedPercent: 1},
			}}}, "a"},
		{"expired cooldown counts as available", config.StrategySequential, "",
			[]Candidate{cooling("a", -time.Minute)}, "a"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Next(tc.strategy, tc.current, tc.candidates, now)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNextAllLockedReportsEarliestReset(t *testing.T) {
	_, err := Next(config.StrategySequential, "a",
		[]Candidate{cooling("a", 3*time.Hour), cooling("b", time.Hour)}, now)
	var locked *AllLockedError
	if !errors.As(err, &locked) {
		t.Fatalf("got %v, want AllLockedError", err)
	}
	if want := now.Add(time.Hour); !locked.EarliestReset.Equal(want) {
		t.Errorf("earliest reset %v, want %v", locked.EarliestReset, want)
	}
}

func TestNextErrors(t *testing.T) {
	if _, err := Next(config.StrategySequential, "", nil, now); !errors.Is(err, ErrNoCandidates) {
		t.Errorf("got %v, want ErrNoCandidates", err)
	}
	if _, err := Next("random", "", []Candidate{free("a")}, now); err == nil {
		t.Error("expected an error for an unknown strategy")
	}
}

func TestNextDoesNotReorderInput(t *testing.T) {
	in := []Candidate{free("b"), free("a")}
	if _, err := Next(config.StrategySequential, "", in, now); err != nil {
		t.Fatal(err)
	}
	if in[0].Name != "b" {
		t.Error("input slice was reordered")
	}
}
