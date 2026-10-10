package main

import (
	"os"
	"slices"
	"time"

	"github.com/spf13/cobra"

	"github.com/LuGG000/switchyard/internal/breaker"
	"github.com/LuGG000/switchyard/internal/headless"
	"github.com/LuGG000/switchyard/internal/launcher"
	"github.com/LuGG000/switchyard/internal/profiles"
	"github.com/LuGG000/switchyard/internal/state"
)

// isHeadless reports whether the claude arguments ask for a non-interactive run.
func isHeadless(args []string) bool {
	return slices.Contains(args, "-p") || slices.Contains(args, "--print")
}

// runHeadless runs claude non-interactively and fails over to the next profile
// at a rate limit. There is no terminal to ask, so the failover is automatic
// whatever the config mode says.
func runHeadless(cmd *cobra.Command, m *profiles.Manager, store *state.Store, p profiles.Profile, args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	list, err := m.List()
	if err != nil {
		return err
	}
	r := &headless.Runner{
		Launcher: launcher.Launcher{
			Claude:  m.Claude,
			Environ: os.Environ(),
			Stdin:   cmd.InOrStdin(),
			Stdout:  cmd.OutOrStdout(),
			Stderr:  cmd.ErrOrStderr(),
		},
		Store:        store,
		Profiles:     list,
		Strategy:     cfg.Strategy,
		CarryContext: cfg.CarryContext,
		Limits:       breaker.FromConfig(cfg),
		Prepare: func(p profiles.Profile, args []string) ([]string, error) {
			return prepareRun(store, p, args)
		},
		Now:    time.Now,
		Notice: cmd.ErrOrStderr(),
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
