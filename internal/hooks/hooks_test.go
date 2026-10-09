package hooks

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/LuGG000/switchyard/internal/state"
)

var now = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

func newHandler(t *testing.T) *Handler {
	t.Helper()
	return &Handler{
		Store:      state.NewStore(filepath.Join(t.TempDir(), "state.json")),
		Profile:    "acc1",
		ProfileDir: t.TempDir(),
		Now:        func() time.Time { return now },
	}
}

const rateLimitsJSON = `{"session_id":"s1","rate_limits":{
  "five_hour":{"used_percentage":63,"resets_at":1791540600},
  "seven_day":{"used_percentage":21,"resets_at":1791892800}}}`

func TestSettingsWiresBothHooks(t *testing.T) {
	raw, err := Settings("/opt/switch yard/switchyard", "acc1")
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Hooks struct {
			StopFailure []struct {
				Matcher string `json:"matcher"`
				Hooks   []struct {
					Type    string `json:"type"`
					Command string `json:"command"`
				} `json:"hooks"`
			} `json:"StopFailure"`
		} `json:"hooks"`
		StatusLine struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		} `json:"statusLine"`
	}
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatal(err)
	}
	if len(s.Hooks.StopFailure) != 1 || s.Hooks.StopFailure[0].Matcher != "rate_limit" {
		t.Fatalf("unexpected StopFailure hooks: %+v", s.Hooks.StopFailure)
	}
	stop := s.Hooks.StopFailure[0].Hooks[0]
	if stop.Type != "command" || !strings.HasSuffix(stop.Command, "hook stop-failure --profile acc1") {
		t.Errorf("stop-failure command %q", stop.Command)
	}
	if s.StatusLine.Type != "command" || !strings.HasSuffix(s.StatusLine.Command, "hook statusline --profile acc1") {
		t.Errorf("statusline %+v", s.StatusLine)
	}
	if !strings.Contains(stop.Command, "switch yard") {
		t.Errorf("path lost: %q", stop.Command)
	}
	if runtime.GOOS != "windows" && !strings.HasPrefix(stop.Command, "'/opt/switch yard/switchyard'") {
		t.Errorf("path not quoted: %q", stop.Command)
	}
}

func TestQuoteEscapesSingleQuotes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX quoting")
	}
	if got, want := quote("/a'b"), `'/a'\''b'`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStatuslineRecordsUsageAndPrintsSummary(t *testing.T) {
	h := newHandler(t)
	var out bytes.Buffer
	if err := h.Statusline(strings.NewReader(rateLimitsJSON), &out); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "acc1 · 5h 63% · 7d 21%\n"; got != want {
		t.Errorf("output %q, want %q", got, want)
	}
	st, err := h.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	p := st.Profiles["acc1"]
	if p.FiveHour == nil || p.FiveHour.UsedPercent != 63 || p.FiveHour.ResetsAt.Unix() != 1791540600 || !p.FiveHour.UpdatedAt.Equal(now) {
		t.Errorf("five hour usage %+v", p.FiveHour)
	}
	if p.SevenDay == nil || p.SevenDay.UsedPercent != 21 {
		t.Errorf("seven day usage %+v", p.SevenDay)
	}
}

func TestStatuslineWithoutRateLimitsPrintsProfileOnly(t *testing.T) {
	h := newHandler(t)
	var out bytes.Buffer
	if err := h.Statusline(strings.NewReader(`{"session_id":"s1","rate_limits":null}`), &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "acc1\n" {
		t.Errorf("output %q", out.String())
	}
	if st, _ := h.Store.Read(); st.Profiles["acc1"].FiveHour != nil {
		t.Error("usage recorded without rate limits")
	}
}

func TestStatuslineSurvivesGarbageInput(t *testing.T) {
	h := newHandler(t)
	var out bytes.Buffer
	if err := h.Statusline(strings.NewReader("not json"), &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "acc1\n" {
		t.Errorf("output %q", out.String())
	}
}

func TestStatuslineChainsUserCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell command")
	}
	h := newHandler(t)
	settings := `{"statusLine":{"type":"command","command":"jq -r .session_id | tr a-z A-Z"}}`
	if err := os.WriteFile(filepath.Join(h.ProfileDir, "settings.json"), []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := h.Statusline(strings.NewReader(rateLimitsJSON), &out); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != "S1" {
		t.Errorf("user output %q, want S1", got)
	}
	if st, _ := h.Store.Read(); st.Profiles["acc1"].FiveHour == nil {
		t.Error("usage not recorded while chaining")
	}
}

func TestStatuslineFallsBackWhenUserCommandFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell command")
	}
	h := newHandler(t)
	settings := `{"statusLine":{"type":"command","command":"exit 3"}}`
	if err := os.WriteFile(filepath.Join(h.ProfileDir, "settings.json"), []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := h.Statusline(strings.NewReader(rateLimitsJSON), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "acc1 · 5h") {
		t.Errorf("output %q", out.String())
	}
}

func TestStopFailureCooldown(t *testing.T) {
	future := func(d time.Duration, pct float64) *state.Usage {
		return &state.Usage{UsedPercent: pct, ResetsAt: now.Add(d)}
	}
	tests := []struct {
		name string
		p    state.Profile
		want time.Time
	}{
		{"five hour window used up", state.Profile{FiveHour: future(2*time.Hour, 100), SevenDay: future(48*time.Hour, 40)}, now.Add(2 * time.Hour)},
		{"weekly window used up wins", state.Profile{FiveHour: future(2*time.Hour, 100), SevenDay: future(48*time.Hour, 100)}, now.Add(48 * time.Hour)},
		{"stale usage uses the nearest reset", state.Profile{FiveHour: future(3*time.Hour, 80), SevenDay: future(48*time.Hour, 40)}, now.Add(3 * time.Hour)},
		{"past resets are ignored", state.Profile{FiveHour: future(-time.Hour, 100)}, now.Add(fallbackCooldown)},
		{"unknown usage uses the fixed time", state.Profile{}, now.Add(fallbackCooldown)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHandler(t)
			if err := h.Store.Update(func(st *state.State) error {
				st.Profiles["acc1"] = tc.p
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if err := h.StopFailure(); err != nil {
				t.Fatal(err)
			}
			st, _ := h.Store.Read()
			if got := st.Profiles["acc1"].CooldownUntil; !got.Equal(tc.want) {
				t.Errorf("cooldown until %v, want %v", got, tc.want)
			}
		})
	}
}
