package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/LuGG000/switchyard/internal/breaker"
	"github.com/LuGG000/switchyard/internal/config"
	"github.com/LuGG000/switchyard/internal/history"
	"github.com/LuGG000/switchyard/internal/state"
)

// recentSwitches is how many switches the watch view lists.
const recentSwitches = 5

func newWatchCmd() *cobra.Command {
	var interval time.Duration
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Show the profiles, their usage and the recent switches, refreshed until Ctrl+C",
		Long: "A live view for people without the mod: the active profile, the usage and cooldown of every\n" +
			"profile, the failover settings, how many automatic switches were made in the last 24 hours and the\n" +
			"recent switches. It refreshes every two seconds (--interval) and ends with Ctrl+C. Without a\n" +
			"terminal it prints one snapshot.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if interval < time.Second {
				return fmt.Errorf("--interval %v is shorter than one second", interval)
			}
			m, err := rootOnlyManager()
			if err != nil {
				return err
			}
			store, err := newStateStore()
			if err != nil {
				return err
			}
			log, err := newHistoryLog()
			if err != nil {
				return err
			}
			frame := func(live bool) (string, error) {
				report, err := buildStatus(m, store)
				if err != nil {
					return "", err
				}
				st, err := store.Read()
				if err != nil {
					return "", err
				}
				cfg, err := loadConfig()
				if err != nil {
					return "", err
				}
				entries, err := log.Last(recentSwitches)
				if err != nil {
					return "", err
				}
				var buf bytes.Buffer
				err = writeWatch(&buf, live, report, st, cfg, entries, time.Now())
				return buf.String(), err
			}

			out, isFile := cmd.OutOrStdout().(*os.File)
			if !isFile || !isTerminal(out) {
				text, err := frame(false)
				if err != nil {
					return err
				}
				_, err = io.WriteString(cmd.OutOrStdout(), text)
				return err
			}
			defer enableANSI(out)()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				text, err := frame(true)
				if err != nil {
					return err
				}
				// Home, then the frame, then clear what is left of a longer previous one.
				_, _ = fmt.Fprint(out, "\x1b[H"+text+"\x1b[J")
				select {
				case <-cmd.Context().Done():
					return nil
				case <-ticker.C:
				}
			}
		},
	}
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "time between refreshes")
	return cmd
}

// writeWatch draws one frame of the watch view; live adds the hint how to leave it.
func writeWatch(w io.Writer, live bool, report statusReport, st state.State, cfg config.Config, recent []history.Entry, now time.Time) error {
	hint := ""
	if live {
		hint = "  (Ctrl+C quits)"
	}
	_, _ = fmt.Fprintf(w, "switchyard %s  %s%s\n\n", report.Version, now.Format("15:04:05"), hint)
	if err := writeStatus(w, report); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "\nOn a limit: mode %s, strategy %s, thresholds %s / %s\n", cfg.Mode, cfg.Strategy,
		percentOrOff(cfg.ProactiveThreshold), percentOrOff(cfg.ProactiveThresholdWeekly))
	_, _ = fmt.Fprintf(w, "Automatic switches in the last 24 hours: %d%s\n", autoSwitchesIn24h(st, now), maxSuffix(cfg.MaxAutoSwitches))
	if err := breaker.FromConfig(cfg).Check(st, now); err != nil {
		_, _ = fmt.Fprintf(w, "Automatic switching is paused: %v\n", err)
	}
	_, _ = fmt.Fprintln(w, "\nRecent switches:")
	if len(recent) == 0 {
		_, _ = fmt.Fprintln(w, "  none yet")
	}
	for _, e := range recent {
		conversation := "new conversation"
		if e.Carry {
			conversation = "conversation continued"
		}
		_, _ = fmt.Fprintf(w, "  %s  %s -> %s  %s, %s, %s\n", e.Time.Local().Format("2006-01-02 15:04"), e.From, e.To, e.Reason, e.How, conversation)
	}
	return nil
}

func autoSwitchesIn24h(st state.State, now time.Time) int {
	n := 0
	for _, at := range st.AutoSwitches {
		if now.Sub(at) < breaker.Window {
			n++
		}
	}
	return n
}

func maxSuffix(max int) string {
	if max == 0 {
		return ""
	}
	return fmt.Sprintf(" of %d", max)
}
