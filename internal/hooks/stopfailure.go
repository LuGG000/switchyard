package hooks

import (
	"encoding/json"
	"io"
	"time"

	"github.com/LuGG000/switchyard/internal/state"
)

// fallbackCooldown applies when no reset time is known for the exhausted window.
const fallbackCooldown = 30 * time.Minute

// exhausted is the usage at which a window counts as used up.
const exhausted = 99.5

// sessionInput is the part of the hook input that switchyard reads.
type sessionInput struct {
	SessionID string `json:"session_id"`
}

// StopFailure marks the profile as rate limited and asks the launcher to switch.
// The cooldown lasts until the window that is used up resets, or, if usage is
// unknown, a fixed time. The hook input on in names the session to resume; an
// unreadable input only costs that.
func (h *Handler) StopFailure(in io.Reader) error {
	raw, _ := io.ReadAll(io.LimitReader(in, maxInput))
	var input sessionInput
	_ = json.Unmarshal(raw, &input)
	now := h.Now().UTC()
	return h.Store.Update(func(st *state.State) error {
		h.markLimited(st, now)
		st.SwitchRequest = &state.SwitchRequest{
			Profile:     h.Profile,
			Reason:      state.ReasonRateLimit,
			SessionID:   input.SessionID,
			RequestedAt: now,
		}
		return nil
	})
}

// MarkLimited puts the profile into cooldown without asking for a switch.
func (h *Handler) MarkLimited() error {
	now := h.Now().UTC()
	return h.Store.Update(func(st *state.State) error {
		h.markLimited(st, now)
		return nil
	})
}

func (h *Handler) markLimited(st *state.State, now time.Time) {
	p := st.Profiles[h.Profile]
	p.CooldownUntil = cooldownUntil(p, now)
	st.Profiles[h.Profile] = p
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
