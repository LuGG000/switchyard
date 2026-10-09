package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/LuGG000/switchyard/internal/config"
	"github.com/LuGG000/switchyard/internal/launcher"
	"github.com/LuGG000/switchyard/internal/profiles"
	"github.com/LuGG000/switchyard/internal/state"
)

const stateFile = "state.json"

// exitError carries claude's exit code up to main.
type exitError struct{ code int }

func (e exitError) Error() string { return fmt.Sprintf("claude exited with status %d", e.code) }

func newStateStore() (*state.Store, error) {
	dataDir, err := config.DataDir()
	if err != nil {
		return nil, err
	}
	return state.NewStore(filepath.Join(dataDir, stateFile)), nil
}

func newRunCmd() *cobra.Command {
	var profileName string
	cmd := &cobra.Command{
		Use:   "run [flags] [-- claude args...]",
		Short: "Run claude with the active profile",
		Long: "Run claude with the credentials of a profile. Arguments after -- are passed to claude.\n" +
			"Without --profile the active profile is used, or the first profile if none is active.",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := newProfileManager()
			if err != nil {
				return err
			}
			store, err := newStateStore()
			if err != nil {
				return err
			}
			p, err := resolveProfile(m, store, profileName)
			if err != nil {
				return err
			}
			return runClaude(cmd, m, store, p, args)
		},
	}
	cmd.Flags().StringVarP(&profileName, "profile", "p", "", "profile to use instead of the active one")
	return cmd
}

// resolveProfile picks the named profile, else the active one, else the first.
func resolveProfile(m *profiles.Manager, store *state.Store, name string) (profiles.Profile, error) {
	if name == "" {
		st, err := store.Read()
		if err != nil {
			return profiles.Profile{}, err
		}
		name = st.Active
	}
	if name != "" {
		return m.Get(name)
	}
	list, err := m.List()
	if err != nil {
		return profiles.Profile{}, err
	}
	if len(list) == 0 {
		return profiles.Profile{}, errors.New("no profiles yet; create one with: switchyard add <name>")
	}
	return list[0], nil
}

// runClaude records p as active, runs claude and turns a non-zero exit into an exitError.
func runClaude(cmd *cobra.Command, m *profiles.Manager, store *state.Store, p profiles.Profile, args []string) error {
	err := store.Update(func(st *state.State) error {
		st.Active = p.Name
		entry := st.Profiles[p.Name]
		entry.LastUsed = time.Now().UTC()
		st.Profiles[p.Name] = entry
		return nil
	})
	if err != nil {
		return err
	}
	l := &launcher.Launcher{
		Claude:  m.Claude,
		Environ: os.Environ(),
		Stdin:   cmd.InOrStdin(),
		Stdout:  cmd.OutOrStdout(),
		Stderr:  cmd.ErrOrStderr(),
	}
	code, err := l.Run(cmd.Context(), p, args)
	if err != nil {
		return err
	}
	if code != 0 {
		return exitError{code: code}
	}
	return nil
}
