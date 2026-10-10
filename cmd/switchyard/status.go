package main

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/LuGG000/switchyard/internal/profiles"
	"github.com/LuGG000/switchyard/internal/state"
)

// statusSchema is the version of the `status --json` layout. Consumers such as
// the mod check it before reading anything else.
const statusSchema = 1

type statusReport struct {
	Schema   int             `json:"schema"`
	Version  string          `json:"version"`
	Active   string          `json:"active"`
	Profiles []profileStatus `json:"profiles"`
	// Update is set when a newer release is known from the last check.
	Update *updateInfo `json:"update"`
}

type profileStatus struct {
	Name          string     `json:"name"`
	Active        bool       `json:"active"`
	LastUsed      *time.Time `json:"last_used"`
	CooldownUntil *time.Time `json:"cooldown_until"`
	FiveHour      *usageInfo `json:"five_hour"`
	SevenDay      *usageInfo `json:"seven_day"`
}

// usageInfo is the last known usage of a limit window.
type usageInfo struct {
	UsedPercent float64   `json:"used_percent"`
	ResetsAt    time.Time `json:"resets_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func newStatusCmd() *cobra.Command {
	var asJSON, short bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the active profile and the state of all profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := newProfileManager()
			if err != nil {
				return err
			}
			store, err := newStateStore()
			if err != nil {
				return err
			}
			report, err := buildStatus(m, store)
			if err != nil {
				return err
			}
			if short {
				if line := shortStatus(report, time.Now()); line != "" {
					println(cmd, line)
				}
				return nil
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(report)
			}
			return printStatus(cmd, report)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print machine-readable JSON")
	cmd.Flags().BoolVar(&short, "short", false, "print one line for a shell prompt or tmux; nothing without an active profile")
	cmd.MarkFlagsMutuallyExclusive("json", "short")
	return cmd
}

func buildStatus(m *profiles.Manager, store *state.Store) (statusReport, error) {
	list, err := m.List()
	if err != nil {
		return statusReport{}, err
	}
	st, err := store.Read()
	if err != nil {
		return statusReport{}, err
	}
	report := statusReport{Schema: statusSchema, Version: currentVersion(), Active: st.Active, Profiles: []profileStatus{}, Update: pendingUpdate()}
	for _, p := range list {
		entry := st.Profiles[p.Name]
		report.Profiles = append(report.Profiles, profileStatus{
			Name:          p.Name,
			Active:        p.Name == st.Active,
			LastUsed:      optionalTime(entry.LastUsed),
			CooldownUntil: optionalTime(entry.CooldownUntil),
			FiveHour:      usageOf(entry.FiveHour),
			SevenDay:      usageOf(entry.SevenDay),
		})
	}
	return report, nil
}

func usageOf(u *state.Usage) *usageInfo {
	if u == nil {
		return nil
	}
	return &usageInfo{UsedPercent: u.UsedPercent, ResetsAt: u.ResetsAt, UpdatedAt: u.UpdatedAt}
}

func optionalTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func printStatus(cmd *cobra.Command, report statusReport) error {
	return writeStatus(cmd.OutOrStdout(), report)
}

func writeStatus(w io.Writer, report statusReport) error {
	if len(report.Profiles) == 0 {
		_, err := fmt.Fprintln(w, "No profiles yet. Create one with: switchyard add <name>")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "\tNAME\t5H\t7D\tLAST USED\tCOOLDOWN UNTIL")
	for _, p := range report.Profiles {
		marker := " "
		if p.Active {
			marker = "*"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", marker, p.Name,
			formatUsage(p.FiveHour), formatUsage(p.SevenDay), formatTime(p.LastUsed), formatTime(p.CooldownUntil))
	}
	return tw.Flush()
}

func formatTime(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04")
}

func formatUsage(u *usageInfo) string {
	if u == nil {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", u.UsedPercent)
}

// updateInfo is a newer release than the running binary.
type updateInfo struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

func pendingUpdate() *updateInfo {
	release, ok := newerRelease()
	if !ok {
		return nil
	}
	return &updateInfo{Version: release.Version, URL: release.URL}
}

// shortStatus is the one line of `status --short`: the active profile with its usage, or the time its
// limit ends. It is empty when no profile is active, so a prompt segment can stay blank.
func shortStatus(report statusReport, now time.Time) string {
	for _, p := range report.Profiles {
		if !p.Active {
			continue
		}
		if p.CooldownUntil != nil && p.CooldownUntil.After(now) {
			return fmt.Sprintf("%s limit until %s", p.Name, p.CooldownUntil.Local().Format("15:04"))
		}
		line := p.Name
		if p.FiveHour != nil {
			line += " 5h " + formatUsage(p.FiveHour)
		}
		if p.SevenDay != nil {
			line += " 7d " + formatUsage(p.SevenDay)
		}
		return line
	}
	return ""
}
