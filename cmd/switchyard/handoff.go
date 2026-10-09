package main

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/LuGG000/switchyard/internal/config"
	"github.com/LuGG000/switchyard/internal/profiles"
	"github.com/LuGG000/switchyard/internal/state"
)

func newHandoffCmd() *cobra.Command {
	var resume, fresh bool
	var sessionID string
	cmd := &cobra.Command{
		Use:   "handoff <name>",
		Short: "Ask the running session to continue in another profile",
		Long: "Ask the switchyard session that runs the active profile to end claude and continue in\n" +
			"the named profile, without asking. It only has an effect while claude was started with\n" +
			"switchyard run or switch. --resume continues the conversation in the new profile (name\n" +
			"its session with --session, else the most recent one of the directory), --fresh starts\n" +
			"a new one; without either, carry_context from the config decides.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			dataDir, err := config.DataDir()
			if err != nil {
				return err
			}
			// Handoff must not depend on claude being in PATH, so no full manager is built.
			m := &profiles.Manager{Root: filepath.Join(dataDir, profilesSubdir)}
			store, err := newStateStore()
			if err != nil {
				return err
			}
			target, err := m.Get(args[0])
			if err != nil {
				return err
			}
			carry := cfg.CarryContext
			if resume || fresh {
				carry = resume
			}
			err = store.Update(func(st *state.State) error {
				if st.Active == "" {
					return fmt.Errorf("no profile is active; there is no session to hand off")
				}
				if st.Active == target.Name {
					return fmt.Errorf("%s is already the active profile", target.Name)
				}
				st.SwitchRequest = &state.SwitchRequest{
					Profile:     st.Active,
					Reason:      state.ReasonManual,
					SessionID:   sessionID,
					Target:      target.Name,
					Carry:       &carry,
					RequestedAt: time.Now().UTC(),
				}
				return nil
			})
			if err != nil {
				return err
			}
			printf(cmd, "Handoff to %s requested\n", target.Name)
			return nil
		},
	}
	cmd.Flags().BoolVar(&resume, "resume", false, "continue the conversation in the new profile")
	cmd.Flags().BoolVar(&fresh, "fresh", false, "start a new conversation")
	cmd.Flags().StringVar(&sessionID, "session", "", "ID of the session to continue (default: the most recent one)")
	cmd.MarkFlagsMutuallyExclusive("resume", "fresh")
	return cmd
}
