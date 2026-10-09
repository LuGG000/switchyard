package main

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/LuGG000/switchyard/internal/config"
)

// configSchema is the version of the `config --json` output. New fields may be
// added; consumers check it first.
const configSchema = 1

type configReport struct {
	Schema             int           `json:"schema"`
	Mode               string        `json:"mode"`
	CarryContext       bool          `json:"carry_context"`
	Strategy           string        `json:"strategy"`
	ProactiveThreshold int           `json:"proactive_threshold"`
	UpdateCheck        bool          `json:"update_check"`
	Colors             config.Colors `json:"colors"`
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
			"  proactive_threshold  five-hour usage in percent that triggers a switch, 0 = off\n" +
			"  update_check         mention a newer release once a day (status, list, doctor)\n\n" +
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
				UpdateCheck:        cfg.UpdateCheck,
				Colors:             cfg.Palette(),
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
			colors := report.Colors
			for _, kv := range [][2]string{
				{config.KeyColorBackground, colors.Background}, {config.KeyColorText, colors.Text},
				{config.KeyColorLow, colors.Low}, {config.KeyColorMedium, colors.Medium}, {config.KeyColorHigh, colors.High},
				{config.KeyColorBorderActive, colors.BorderActive}, {config.KeyColorBorder, colors.Border},
			} {
				printf(cmd, "%-20s %s\n", kv[0], colorOrTheme(kv[1]))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the settings as JSON (schema 1)")
	cmd.AddCommand(newConfigSetCmd(), newConfigColorsCmd())
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

// colorOrTheme names an unset color for display.
func colorOrTheme(value string) string {
	if value == "" {
		return "(theme)"
	}
	return value
}

func newConfigColorsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "colors <" + strings.Join(config.PresetNames(), "|") + ">",
		Short: "Set all pane colors to a palette",
		Long: "Set the colors of the mod's accounts pane to a palette: default (the Claude theme's own\n" +
			"colors), dark or light. Single colors: switchyard config set color_background '#1e1e1e'.\n" +
			"A color is a theme color (success, warning, error, subtle, ...), a color name or hex;\n" +
			"an empty value uses the theme's.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := config.Path()
			if err != nil {
				return err
			}
			if err := config.SetPreset(path, args[0]); err != nil {
				return err
			}
			printf(cmd, "Colors set to %s\n", args[0])
			return nil
		},
	}
}
