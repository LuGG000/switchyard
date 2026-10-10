package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/LuGG000/switchyard/internal/config"
	"github.com/LuGG000/switchyard/internal/history"
)

const historyFile = "history.jsonl"

// newHistoryLog is the history file in the data dir.
func newHistoryLog() (*history.Log, error) {
	dataDir, err := config.DataDir()
	if err != nil {
		return nil, err
	}
	return history.New(filepath.Join(dataDir, historyFile)), nil
}

// recordSwitch returns the function the runners call for every switch. A history that
// cannot be written never stops a switch.
func recordSwitch() func(history.Entry) {
	log, err := newHistoryLog()
	if err != nil {
		return nil
	}
	return func(e history.Entry) { _ = log.Append(e) }
}

func newHistoryCmd() *cobra.Command {
	var asJSON bool
	var count int
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show the recent profile switches and why they happened",
		Long: "Show the recent switches: when, from which profile to which, why (rate_limit, threshold or\n" +
			"manual) and how (auto: switchyard decided; asked: you answered the question; button: you used\n" +
			"the mod's buttons; handoff: you asked for it), and whether the conversation went along.\n" +
			"The history keeps the newest entries only and holds names and times, nothing else.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			log, err := newHistoryLog()
			if err != nil {
				return err
			}
			entries, err := log.Last(count)
			if err != nil {
				return err
			}
			if asJSON {
				if entries == nil {
					entries = []history.Entry{}
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(entries)
			}
			if len(entries) == 0 {
				println(cmd, "No switches yet.")
				return nil
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "TIME\tFROM\tTO\tWHY\tHOW\tCONVERSATION")
			for _, e := range entries {
				conversation := "new"
				if e.Carry {
					conversation = "continued"
				}
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", e.Time.Local().Format("2006-01-02 15:04"), e.From, e.To, e.Reason, e.How, conversation)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().IntVarP(&count, "number", "n", 20, "how many of the newest switches to show, 0 for all")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print machine-readable JSON")
	return cmd
}
