package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/LuGG000/switchyard/internal/config"
	"github.com/LuGG000/switchyard/internal/hooks"
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
	var waitForReset bool
	cmd := &cobra.Command{
		Use:   "run [flags] [-- claude args...]",
		Short: "Run claude with the active profile",
		Long: "Run claude with the credentials of a profile. Arguments after -- are passed to claude.\n" +
			"Without --profile the active profile is used, or the first profile if none is active.\n\n" +
			"A headless run (switchyard run -- -p \"prompt\") moves on to the next profile when the\n" +
			"current one hits its limit and continues the conversation there (see carry_context).\n\n" +
			"If every profile is at its limit, a headless run stops with exit code 75; with --wait it waits for\n" +
			"the first reset and goes on (Ctrl+C stops). An interactive run waits for it by itself.",
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
			if isHeadless(args) {
				return runHeadless(cmd, m, store, p, args, waitForReset)
			}
			return runInteractive(cmd, m, store, p, args)
		},
	}
	cmd.Flags().StringVarP(&profileName, "profile", "p", "", "profile to use instead of the active one")
	cmd.Flags().BoolVar(&waitForReset, "wait", false, "headless: when every profile is at its limit, wait for the first reset and continue")
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

// markUsed records p as the active profile and as just used.
func markUsed(store *state.Store, p profiles.Profile) error {
	return store.Update(func(st *state.State) error {
		st.Active = p.Name
		entry := st.Profiles[p.Name]
		entry.LastUsed = time.Now().UTC()
		st.Profiles[p.Name] = entry
		return nil
	})
}

// prepareRun records p as in use and returns the claude arguments for it.
func prepareRun(store *state.Store, p profiles.Profile, args []string) ([]string, error) {
	if err := markUsed(store, p); err != nil {
		return nil, err
	}
	relinkShared(p)
	return withSignalSettings(p, args)
}

// withSignalSettings prepends --settings so claude reports usage and rate limit
// errors back to switchyard. The user's own settings files stay untouched.
func withSignalSettings(p profiles.Profile, args []string) ([]string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate switchyard executable: %w", err)
	}
	settings, err := hooks.Settings(exe, p.Name)
	if err != nil {
		return nil, err
	}
	return append([]string{"--settings", settings}, args...), nil
}
