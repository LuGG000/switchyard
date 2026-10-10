package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/LuGG000/switchyard/internal/state"
)

func runDecision(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs(append([]string{"decision"}, args...))
	err := cmd.Execute()
	return out.String(), err
}

func ask(t *testing.T, store *state.Store, expires time.Duration) {
	t.Helper()
	err := store.Update(func(st *state.State) error {
		st.Decision = &state.Decision{Profile: "a", Reason: state.ReasonRateLimit, Options: []string{"b"}, Carry: true,
			AskedAt: time.Now().UTC(), ExpiresAt: time.Now().Add(expires).UTC()}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDecisionReportsTheQuestionAndRecordsAHeartbeat(t *testing.T) {
	store := handoffEnv(t, "")
	out, err := runDecision(t, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var none struct {
		Schema  int             `json:"schema"`
		Pending json.RawMessage `json:"pending"`
	}
	if json.Unmarshal([]byte(out), &none) != nil || none.Schema != decisionSchema || string(none.Pending) != "null" {
		t.Fatalf("nothing pending, got %s", out)
	}
	if time.Since(store.HeartbeatAt()) > 5*time.Second {
		t.Error("a poll must record the heartbeat")
	}

	ask(t, store, time.Minute)
	out, err = runDecision(t, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got decisionReport
	if err := json.Unmarshal([]byte(out), &got); err != nil || got.Pending == nil || got.Pending.Profile != "a" || got.Pending.Options[0] != "b" || !got.Pending.Carry {
		t.Fatalf("pending = %+v (%v) from %s", got.Pending, err, out)
	}
}

func TestExpiredOrAnsweredQuestionsAreNotPending(t *testing.T) {
	store := handoffEnv(t, "")
	ask(t, store, -time.Second)
	out, _ := runDecision(t, "--json")
	if !strings.Contains(out, `"pending": null`) {
		t.Errorf("an expired question must not be shown: %s", out)
	}
}

func TestAnswerSwitchRecordsTheChoice(t *testing.T) {
	store := handoffEnv(t, "")
	ask(t, store, time.Minute)
	if _, err := runDecision(t, "answer", "switch", "b", "--fresh"); err != nil {
		t.Fatal(err)
	}
	st, _ := store.Read()
	a := st.Decision.Answer
	if a == nil || a.Action != state.ActionSwitch || a.Target != "b" || a.Carry == nil || *a.Carry {
		t.Fatalf("answer = %+v", a)
	}
	if _, err := runDecision(t, "answer", "stay"); err == nil {
		t.Error("a question can be answered once")
	}
}

func TestAnswerIsChecked(t *testing.T) {
	store := handoffEnv(t, "")
	if _, err := runDecision(t, "answer", "stay"); err == nil {
		t.Error("no question is waiting")
	}
	ask(t, store, time.Minute)
	for _, args := range [][]string{{"answer", "switch"}, {"answer", "switch", "zzz"}, {"answer", "stay", "b"}, {"answer", "go"}} {
		if _, err := runDecision(t, args...); err == nil {
			t.Errorf("%v should be refused", args)
		}
	}
	if st, _ := store.Read(); st.Decision.Answer != nil {
		t.Errorf("a refused answer must not be stored: %+v", st.Decision.Answer)
	}
	if _, err := runDecision(t, "answer", "stay"); err != nil {
		t.Errorf("stay: %v", err)
	}
}
