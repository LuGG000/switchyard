package hooks

import (
	"time"

	"github.com/LuGG000/switchyard/internal/state"
)

// fallbackCooldown applies when no reset time is known for the exhausted window.
const fallbackCooldown = 30 * time.Minute

// exhausted is the usage at which a window counts as used up.
const exhausted = 99.5

// StopFailure marks the profile as rate limited. The cooldown lasts until the
// window that is used up resets, or, if usage is unknown, a fixed time.
func (h *Handler) StopFailure() error {
	now := h.Now().UTC()
	return h.Store.Update(func(st *state.State) error {
		p := st.Profiles[h.Profile]
		p.CooldownUntil = cooldownUntil(p, now)
		st.Profiles[h.Profile] = p
		return nil
	})
}

func cooldownUntil(p state.Profile, now time.Time) time.Time {
	var until time.Time
	for _, u := range []*state.Usage{p.FiveHour, p.SevenDay} {
		if u != nil && u.UsedPercent >= exhausted && u.ResetsAt.After(until) && u.ResetsAt.After(now) {
			until = u.ResetsAt
		}
	}
	if !until.IsZero() {
		return until
	}
	// Usage looks stale: fall back to the nearest known future reset, else a fixed time.
	for _, u := range []*state.Usage{p.FiveHour, p.SevenDay} {
		if u != nil && u.ResetsAt.After(now) && (until.IsZero() || u.ResetsAt.Before(until)) {
			until = u.ResetsAt
		}
	}
	if !until.IsZero() {
		return until
	}
	return now.Add(fallbackCooldown)
}
