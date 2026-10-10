package state

import (
	"path/filepath"
	"testing"
	"time"
)

func TestHeartbeatIsRemembered(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "state.json"))
	if !s.HeartbeatAt().IsZero() {
		t.Fatal("no heartbeat yet")
	}
	at := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	if err := s.TouchHeartbeat(at); err != nil {
		t.Fatal(err)
	}
	if got := s.HeartbeatAt(); !got.Equal(at) {
		t.Errorf("HeartbeatAt = %v, want %v", got, at)
	}
}

func TestDecisionRoundTrips(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "state.json"))
	carry := false
	want := Decision{Profile: "a", Reason: ReasonRateLimit, Options: []string{"b"}, Carry: true,
		AskedAt: time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC), ExpiresAt: time.Date(2026, 10, 10, 12, 2, 0, 0, time.UTC),
		Answer: &Answer{Action: ActionSwitch, Target: "b", Carry: &carry}}
	if err := s.Update(func(st *State) error { st.Decision = &want; return nil }); err != nil {
		t.Fatal(err)
	}
	st, err := s.Read()
	if err != nil || st.Decision == nil || st.Decision.Answer == nil || st.Decision.Answer.Target != "b" || *st.Decision.Answer.Carry {
		t.Fatalf("Read = %+v, %v", st.Decision, err)
	}
}
