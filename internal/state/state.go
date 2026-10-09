// Package state persists switchyard's runtime state in state.json.
//
// All access goes through Store.Update, which holds an inter-process file lock
// for the whole read-modify-write cycle and replaces the file atomically.
package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

const (
	// SchemaVersion is bumped on incompatible changes to the file layout.
	SchemaVersion = 1

	lockSuffix = ".lock"
	lockRetry  = 20 * time.Millisecond
	lockWait   = 10 * time.Second
)

// Usage is the last known consumption of one rate limit window.
type Usage struct {
	// UsedPercent is the consumed share of the window, 0 to 100.
	UsedPercent float64 `json:"used_percent"`
	// ResetsAt is when the window resets.
	ResetsAt time.Time `json:"resets_at,omitzero"`
	// UpdatedAt is when the value was recorded.
	UpdatedAt time.Time `json:"updated_at,omitzero"`
}

// Profile is the per-profile runtime data.
type Profile struct {
	LastUsed      time.Time `json:"last_used,omitzero"`
	CooldownUntil time.Time `json:"cooldown_until,omitzero"`
	FiveHour      *Usage    `json:"five_hour,omitempty"`
	SevenDay      *Usage    `json:"seven_day,omitempty"`
}

// Reasons for a SwitchRequest.
const (
	// ReasonRateLimit means the profile hit its limit.
	ReasonRateLimit = "rate_limit"
	// ReasonThreshold means the profile reached the configured usage threshold.
	ReasonThreshold = "threshold"
)

// SwitchRequest asks the launcher that runs a profile to move to another one.
// A hook process writes it; the launcher polls for it and clears it.
type SwitchRequest struct {
	Profile     string    `json:"profile"`
	Reason      string    `json:"reason"`
	SessionID   string    `json:"session_id,omitempty"`
	RequestedAt time.Time `json:"requested_at"`
}

// State is the content of state.json.
type State struct {
	Version       int                `json:"version"`
	Active        string             `json:"active,omitempty"`
	Profiles      map[string]Profile `json:"profiles"`
	SwitchRequest *SwitchRequest     `json:"switch_request,omitempty"`
}

// Store reads and writes one state file.
type Store struct {
	path string
}

// NewStore returns a store backed by the file at path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// Read returns the current state without taking the write lock. A missing file
// yields an empty state.
func (s *Store) Read() (State, error) {
	return readFile(s.path)
}

// Update runs fn on the current state under the file lock and saves the result
// unless fn returns an error.
func (s *Store) Update(fn func(*State) error) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	lock := flock.New(s.path + lockSuffix)
	ctx, cancel := context.WithTimeout(context.Background(), lockWait)
	defer cancel()
	locked, err := lock.TryLockContext(ctx, lockRetry)
	if err != nil {
		return fmt.Errorf("lock state: %w", err)
	}
	if !locked {
		return errors.New("lock state: timed out")
	}
	defer func() { _ = lock.Unlock() }()

	st, err := readFile(s.path)
	if err != nil {
		return err
	}
	if err := fn(&st); err != nil {
		return err
	}
	return writeFile(s.path, st)
}

func readFile(path string) (State, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return empty(), nil
	}
	if err != nil {
		return State{}, fmt.Errorf("read state: %w", err)
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if st.Version > SchemaVersion {
		return State{}, fmt.Errorf("%s has schema version %d, this build supports up to %d", path, st.Version, SchemaVersion)
	}
	if st.Profiles == nil {
		st.Profiles = map[string]Profile{}
	}
	st.Version = SchemaVersion
	return st, nil
}

func writeFile(path string, st State) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp state file: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("replace state: %w", err)
	}
	return nil
}

func empty() State {
	return State{Version: SchemaVersion, Profiles: map[string]Profile{}}
}
