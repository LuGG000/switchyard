package main

import (
	"github.com/spf13/cobra"

	"github.com/LuGG000/switchyard/internal/interactive"
	"github.com/LuGG000/switchyard/internal/profiles"
	"github.com/LuGG000/switchyard/internal/state"
)

func newSwitchCmd() *cobra.Command {
	var resume, fresh, noLaunch, dryRun bool
	cmd := &cobra.Command{
		Use:   "switch <name> [flags] [-- claude args...]",
		Short: "Switch to another profile and start claude with it",
		Long: "Make a profile the active one and start claude with it.\n\n" +
			"--resume continues the most recent conversation of the current directory in the new\n" +
			"profile (sessions are shared between profiles). --fresh starts a new conversation.\n" +
			"Without either flag, carry_context from the config decides. --no-launch only changes\n" +
			"the active profile. Arguments after -- are passed to claude.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if dryRun {
				return dryRunSwitch(cmd, cfg.CarryContext, args, resume, fresh, noLaunch)
			}
			m, err := newProfileManager()
			if err != nil {
				return err
			}
			store, err := newStateStore()
			if err != nil {
				return err
			}
			p, err := m.Get(args[0])
			if err != nil {
				return err
			}
			if noLaunch {
				return setActive(cmd, store, p)
			}
			carry := cfg.CarryContext
			if resume || fresh {
				carry = resume
			}
			claudeArgs := args[1:]
			if carry {
				println(cmd, interactive.ColdCacheHint)
				claudeArgs = append([]string{"--continue"}, claudeArgs...)
			}
			return runInteractive(cmd, m, store, p, claudeArgs)
		},
	}
	cmd.Flags().BoolVar(&resume, "resume", false, "continue the current conversation in the new profile")
	cmd.Flags().BoolVar(&fresh, "fresh", false, "start a new conversation")
	cmd.Flags().BoolVar(&noLaunch, "no-launch", false, "only change the active profile")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what the switch would do without changing anything or starting claude")
	cmd.MarkFlagsMutuallyExclusive("resume", "fresh", "no-launch")
	return cmd
}

func setActive(cmd *cobra.Command, store *state.Store, p profiles.Profile) error {
	err := store.Update(func(st *state.State) error {
		st.Active = p.Name
		return nil
	})
	if err != nil {
		return err
	}
	printf(cmd, "Active profile: %s\n", p.Name)
	return nil
}
