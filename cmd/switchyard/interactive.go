package main

import (
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/LuGG000/switchyard/internal/interactive"
	"github.com/LuGG000/switchyard/internal/launcher"
	"github.com/LuGG000/switchyard/internal/profiles"
	"github.com/LuGG000/switchyard/internal/state"
)

const (
	// failoverPoll is how often a running claude is checked for a switch request.
	failoverPoll = 250 * time.Millisecond
	// stopGrace is how long claude gets to exit before it is killed at a switch.
	stopGrace = 5 * time.Second
	// modWait is how long the mod's buttons get to answer a limit before the terminal asks.
	modWait = 2 * time.Minute
	// modStale is how old the mod's last poll may be for the buttons to be used.
	modStale = 10 * time.Second
)

// runInteractive runs claude in the terminal with p and fails over to the next
// profile at a limit, according to the config. A non-zero exit of claude becomes
// an exitError.
func runInteractive(cmd *cobra.Command, m *profiles.Manager, store *state.Store, p profiles.Profile, args []string) error {
	refreshUpdateCache(cmd.Context())
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	list, err := m.List()
	if err != nil {
		return err
	}
	r := &interactive.Runner{
		Launcher: launcher.Launcher{
			Claude:  m.Claude,
			Environ: os.Environ(),
			Stdin:   cmd.InOrStdin(),
			Stdout:  cmd.OutOrStdout(),
			Stderr:  cmd.ErrOrStderr(),
		},
		Store:          store,
		Profiles:       list,
		Mode:           cfg.Mode,
		Strategy:       cfg.Strategy,
		ModWait:        modWait,
		ModStale:       modStale,
		CarryContext:   cfg.CarryContext,
		ContinuePrompt: cfg.ContinuePrompt,
		Prepare: func(p profiles.Profile, args []string) ([]string, error) {
			return prepareRun(store, p, args)
		},
		Reload:    loadConfig,
		Now:       time.Now,
		Poll:      failoverPoll,
		StopGrace: stopGrace,
		In:        cmd.InOrStdin(),
		Out:       cmd.ErrOrStderr(),
	}
	code, err := r.Run(cmd.Context(), p, args)
	if err != nil {
		return err
	}
	if code != 0 {
		return exitError{code: code}
	}
	return nil
}
