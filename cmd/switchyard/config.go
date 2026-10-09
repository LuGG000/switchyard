package main

import (
	"encoding/json"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/LuGG000/switchyard/internal/config"
)

// configSchema is the version of the `config --json` output. New fields may be
// added; consumers check it first.
const configSchema = 1

type configReport struct {
	Schema             int    `json:"schema"`
	Mode               string `json:"mode"`
	CarryContext       bool   `json:"carry_context"`
	Strategy           string `json:"strategy"`
	ProactiveThreshold int    `json:"proactive_threshold"`
}

func newConfigCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show the failover settings",
		Long: "Show the settings that decide what happens at a limit:\n\n" +
			"  mode                 auto switches at once, ask asks in the terminal\n" +
			"  carry_context        continue the conversation in the next profile\n" +
			"  strategy             sequential, most-headroom or round-robin\n" +
			"  proactive_threshold  five-hour usage in percent that triggers a switch, 0 = off\n\n" +
			"Change one with: switchyard config set <key> <value>. A running session picks the\n" +
			"change up at its next limit.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			report := configReport{
				Schema:             configSchema,
				Mode:               cfg.Mode,
				CarryContext:       cfg.CarryContext,
				Strategy:           cfg.Strategy,
				ProactiveThreshold: cfg.ProactiveThreshold,
			}
			if asJSON {
				data, err := json.MarshalIndent(report, "", "  ")
				if err != nil {
					return err
				}
				println(cmd, string(data))
				return nil
			}
			printf(cmd, "%-20s %s\n", config.KeyMode, report.Mode)
			printf(cmd, "%-20s %t\n", config.KeyCarryContext, report.CarryContext)
			printf(cmd, "%-20s %s\n", config.KeyStrategy, report.Strategy)
			printf(cmd, "%-20s %s\n", config.KeyProactiveThreshold, strconv.Itoa(report.ProactiveThreshold))
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the settings as JSON (schema 1)")
	cmd.AddCommand(newConfigSetCmd())
	return cmd
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Change one failover setting",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := config.Path()
			if err != nil {
				return err
			}
			if err := config.Set(path, args[0], args[1]); err != nil {
				return err
			}
			printf(cmd, "%s = %s\n", args[0], args[1])
			return nil
		},
	}
}
