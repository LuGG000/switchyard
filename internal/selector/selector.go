// Package selector chooses the next profile to use.
package selector

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/LuGG000/switchyard/internal/config"
	"github.com/LuGG000/switchyard/internal/profiles"
	"github.com/LuGG000/switchyard/internal/state"
)

// AllLockedError is returned when every candidate is cooling down.
type AllLockedError struct {
	// EarliestReset is when the first profile becomes usable again.
	EarliestReset time.Time
}

func (e *AllLockedError) Error() string {
	return fmt.Sprintf("all profiles are at their limit; the first one is available again at %s",
		e.EarliestReset.Local().Format("2006-01-02 15:04"))
}

// ErrNoCandidates is returned when there is no profile to choose from.
var ErrNoCandidates = errors.New("no profiles to choose from")

// Candidate is a profile with the state the strategies look at.
type Candidate struct {
	Name  string
	State state.Profile
}

// Next picks the profile to use after current according to strategy.
// Candidates are considered in name order. Profiles in cooldown are skipped,
// and current is a candidate again once its own cooldown is over.
func Next(strategy, current string, candidates []Candidate, now time.Time) (string, error) {
	if len(candidates) == 0 {
		return "", ErrNoCandidates
	}
	sorted := slices.Clone(candidates)
	slices.SortFunc(sorted, func(a, b Candidate) int { return strings.Compare(a.Name, b.Name) })

	available := make([]Candidate, 0, len(sorted))
	for _, c := range sorted {
		if !c.State.CooldownUntil.After(now) {
			available = append(available, c)
		}
	}
	if len(available) == 0 {
		return "", &AllLockedError{EarliestReset: earliestReset(sorted)}
	}

	switch strategy {
	case config.StrategySequential:
		return sequential(current, available), nil
	case config.StrategyMostHeadroom:
		return mostHeadroom(available, now), nil
	case config.StrategyRoundRobin:
		return roundRobin(current, sorted, available), nil
	}
	return "", fmt.Errorf("unknown strategy %q", strategy)
}

// sequential keeps using current until it is unavailable, then takes the first available.
func sequential(current string, available []Candidate) string {
	for _, c := range available {
		if c.Name == current {
			return c.Name
		}
	}
	return available[0].Name
}

// mostHeadroom takes the profile with the lowest weekly usage; ties go to the first by name.
func mostHeadroom(available []Candidate, now time.Time) string {
	best := available[0]
	for _, c := range available[1:] {
		if weeklyUsed(c.State, now) < weeklyUsed(best.State, now) {
			best = c
		}
	}
	return best.Name
}

// roundRobin takes the first available profile after current in name order, wrapping around.
func roundRobin(current string, all, available []Candidate) string {
	start := slices.IndexFunc(all, func(c Candidate) bool { return c.Name == current })
	for offset := 1; offset <= len(all); offset++ {
		name := all[(start+offset+len(all))%len(all)].Name
		if slices.ContainsFunc(available, func(c Candidate) bool { return c.Name == name }) {
			return name
		}
	}
	return available[0].Name
}

// weeklyUsed returns the weekly usage, treating an unknown or already reset window as unused.
func weeklyUsed(p state.Profile, now time.Time) float64 {
	u := p.SevenDay
	if u == nil || (!u.ResetsAt.IsZero() && !u.ResetsAt.After(now)) {
		return 0
	}
	return u.UsedPercent
}

func earliestReset(candidates []Candidate) time.Time {
	earliest := candidates[0].State.CooldownUntil
	for _, c := range candidates[1:] {
		if c.State.CooldownUntil.Before(earliest) {
			earliest = c.State.CooldownUntil
		}
	}
	return earliest
}

// NextProfile picks the profile to continue with after current among list,
// using the cooldowns and usage in the stored state.
func NextProfile(store *state.Store, list []profiles.Profile, strategy, current string, now time.Time) (profiles.Profile, error) {
	st, err := store.Read()
	if err != nil {
		return profiles.Profile{}, err
	}
	candidates := make([]Candidate, len(list))
	for i, p := range list {
		candidates[i] = Candidate{Name: p.Name, State: st.Profiles[p.Name]}
	}
	name, err := Next(strategy, current, candidates, now)
	if err != nil {
		return profiles.Profile{}, err
	}
	i := slices.IndexFunc(list, func(p profiles.Profile) bool { return p.Name == name })
	if i < 0 {
		return profiles.Profile{}, fmt.Errorf("selected profile %q does not exist", name)
	}
	return list[i], nil
}

// WaitUntil blocks until until, judged by now, or until ctx ends, in which case it
// returns the error of ctx.
func WaitUntil(ctx context.Context, until time.Time, now func() time.Time) error {
	timer := time.NewTimer(until.Sub(now()))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
