package state

import (
	"os"
	"path/filepath"
	"time"
)

// Actions of an Answer.
const (
	// ActionSwitch continues in Answer.Target.
	ActionSwitch = "switch"
	// ActionStay keeps the session in the limited profile and does nothing.
	ActionStay = "stay"
)

// Decision is a question the launcher puts to the mod while claude keeps
// running: what to do now that Profile hit its limit. The mod shows it as
// buttons and answers through `switchyard decision answer`.
type Decision struct {
	// Profile is the profile that hit its limit.
	Profile string `json:"profile"`
	Reason  string `json:"reason"`
	// Options are the profiles the session can continue in.
	Options []string `json:"options"`
	// Carry is the default for resuming the conversation (carry_context).
	Carry     bool      `json:"carry"`
	AskedAt   time.Time `json:"asked_at"`
	ExpiresAt time.Time `json:"expires_at"`
	// Answer is set by the mod; the launcher clears the decision after reading it.
	Answer *Answer `json:"answer,omitempty"`
}

// Answer is the choice made in the mod.
type Answer struct {
	Action string `json:"action"`
	Target string `json:"target,omitempty"`
	// Carry says whether to resume the conversation; nil leaves it to carry_context.
	Carry *bool `json:"carry,omitempty"`
}

const heartbeatFile = "mod-heartbeat"

// TouchHeartbeat records that a mod is polling. It is a file of its own, next
// to the state file, so the frequent polling does not rewrite state.json.
func (s *Store) TouchHeartbeat(now time.Time) error {
	path := filepath.Join(filepath.Dir(s.path), heartbeatFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		return err
	}
	return os.Chtimes(path, now, now)
}

// HeartbeatAt is when a mod last polled; the zero time if none did.
func (s *Store) HeartbeatAt() time.Time {
	info, err := os.Stat(filepath.Join(filepath.Dir(s.path), heartbeatFile))
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}
