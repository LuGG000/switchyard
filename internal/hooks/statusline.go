package hooks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/LuGG000/switchyard/internal/state"
)

// maxInput bounds how much hook input is read.
const maxInput = 1 << 20

// Handler runs the hook commands for one profile.
type Handler struct {
	Store   *state.Store
	Profile string
	// ProfileDir is the profile's claude config dir; the user's statusLine is read from it.
	ProfileDir string
	// Mode is the failover mode (auto or ask); the status line names it.
	Mode string
	// Threshold is the five-hour usage percentage that asks the launcher to
	// switch profiles; zero disables it.
	Threshold int
	// Update is the version of a newer release, shown in the summary; empty for none.
	Update string
	// Now returns the current time.
	Now func() time.Time
}

type window struct {
	UsedPercentage float64 `json:"used_percentage"`
	ResetsAt       int64   `json:"resets_at"`
}

type statuslineInput struct {
	SessionID  string `json:"session_id"`
	RateLimits *struct {
		FiveHour *window `json:"five_hour"`
		SevenDay *window `json:"seven_day"`
	} `json:"rate_limits"`
}

// Statusline records the usage from claude's statusLine JSON and writes the
// status line to out: the output of the user's own statusLine command if there
// is one, otherwise a compact summary. It never fails the status line over
// state problems.
func (h *Handler) Statusline(in io.Reader, out io.Writer) error {
	raw, err := io.ReadAll(io.LimitReader(in, maxInput))
	if err != nil {
		return fmt.Errorf("read statusline input: %w", err)
	}
	var input statuslineInput
	_ = json.Unmarshal(raw, &input)

	var five, seven *state.Usage
	if rl := input.RateLimits; rl == nil {
		five, seven = h.lastKnownUsage()
	} else {
		now := h.Now()
		five, seven = usage(rl.FiveHour, now), usage(rl.SevenDay, now)
		_ = h.Store.Update(func(st *state.State) error {
			p := st.Profiles[h.Profile]
			p.FiveHour, p.SevenDay = five, seven
			st.Profiles[h.Profile] = p
			if h.Threshold > 0 && five != nil && five.UsedPercent >= float64(h.Threshold) {
				st.SwitchRequest = &state.SwitchRequest{
					Profile:     h.Profile,
					Reason:      state.ReasonThreshold,
					SessionID:   input.SessionID,
					RequestedAt: now.UTC(),
				}
			}
			return nil
		})
	}

	if text, ok := h.userStatusline(raw); ok {
		_, err := io.WriteString(out, text)
		return err
	}
	_, err = fmt.Fprintln(out, summary(h.Profile, five, seven, h.Mode, h.Update))
	return err
}

// lastKnownUsage returns the stored usage of windows that have not reset yet.
// claude sends no rate limits before the first response of a session.
func (h *Handler) lastKnownUsage() (five, seven *state.Usage) {
	st, err := h.Store.Read()
	if err != nil {
		return nil, nil
	}
	now := h.Now()
	current := func(u *state.Usage) *state.Usage {
		if u == nil || !u.ResetsAt.After(now) {
			return nil
		}
		return u
	}
	p := st.Profiles[h.Profile]
	return current(p.FiveHour), current(p.SevenDay)
}

func usage(w *window, now time.Time) *state.Usage {
	if w == nil {
		return nil
	}
	return &state.Usage{UsedPercent: w.UsedPercentage, ResetsAt: time.Unix(w.ResetsAt, 0).UTC(), UpdatedAt: now.UTC()}
}

func summary(profile string, five, seven *state.Usage, mode, update string) string {
	parts := []string{profile}
	if five != nil {
		parts = append(parts, fmt.Sprintf("5h %.0f%%", five.UsedPercent))
	}
	if seven != nil {
		parts = append(parts, fmt.Sprintf("7d %.0f%%", seven.UsedPercent))
	}
	if mode != "" {
		parts = append(parts, "on limit: "+mode)
	}
	if update != "" {
		parts = append(parts, "update "+update+" available")
	}
	return strings.Join(parts, " · ")
}

// userStatusline runs the statusLine command from the profile's settings.json
// with the same input. ok is false if there is none or it fails.
func (h *Handler) userStatusline(input []byte) (string, bool) {
	command := userStatuslineCommand(filepath.Join(h.ProfileDir, "settings.json"))
	if command == "" {
		return "", false
	}
	cmd := shellCommand(command)
	cmd.Stdin = bytes.NewReader(input)
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return string(out), true
}

func userStatuslineCommand(settingsPath string) string {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return ""
	}
	var settings struct {
		StatusLine struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		} `json:"statusLine"`
	}
	if json.Unmarshal(data, &settings) != nil || settings.StatusLine.Type != "command" {
		return ""
	}
	return settings.StatusLine.Command
}

func shellCommand(command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/C", command)
	}
	return exec.Command("sh", "-c", command)
}
