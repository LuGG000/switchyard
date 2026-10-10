package main

import (
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/LuGG000/switchyard/internal/config"
	"github.com/LuGG000/switchyard/internal/hooks"
	"github.com/LuGG000/switchyard/internal/profiles"
)

// newHookCmd returns the commands claude calls back; they are not meant to be run by hand.
func newHookCmd() *cobra.Command {
	var profileName string
	hook := &cobra.Command{
		Use:    "hook",
		Short:  "Commands called by claude (statusLine, StopFailure)",
		Hidden: true,
	}
	hook.PersistentFlags().StringVar(&profileName, "profile", "", "profile claude is running with")
	_ = hook.MarkPersistentFlagRequired("profile")

	hook.AddCommand(
		&cobra.Command{
			Use:   "statusline",
			Short: "Record usage from the statusLine JSON on stdin and print the status line",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				h, err := newHookHandler(profileName)
				if err != nil {
					return err
				}
				return h.Statusline(cmd.InOrStdin(), cmd.OutOrStdout())
			},
		},
		&cobra.Command{
			Use:   "stop-failure",
			Short: "Mark the profile as rate limited",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				h, err := newHookHandler(profileName)
				if err != nil {
					return err
				}
				return h.StopFailure(cmd.InOrStdin())
			},
		},
	)
	return hook
}

func newHookHandler(profileName string) (*hooks.Handler, error) {
	dataDir, err := config.DataDir()
	if err != nil {
		return nil, err
	}
	// Hooks must not depend on claude being in PATH, so no full manager is built.
	m := &profiles.Manager{Root: filepath.Join(dataDir, profilesSubdir)}
	p, err := m.Get(profileName)
	if err != nil {
		return nil, err
	}
	store, err := newStateStore()
	if err != nil {
		return nil, err
	}
	h := &hooks.Handler{Store: store, Profile: p.Name, ProfileDir: p.Dir, Now: time.Now}
	// A broken config must not break claude's status line; it only drops the threshold and the mode.
	if cfg, err := loadConfig(); err == nil {
		h.Threshold, h.ThresholdWeekly, h.Mode = cfg.ProactiveThreshold, cfg.ProactiveThresholdWeekly, cfg.Mode
		if release, ok := newerRelease(); ok {
			h.Update = release.Version
		}
	}
	return h, nil
}
