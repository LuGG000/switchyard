package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/LuGG000/switchyard/internal/config"
	"github.com/LuGG000/switchyard/internal/linker"
	"github.com/LuGG000/switchyard/internal/profiles"
)

func newRepairCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "repair [name...]",
		Short: "Re-create the shared links of all or the named profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := newProfileManager()
			if err != nil {
				return err
			}
			selected, err := selectProfiles(m, args)
			if err != nil {
				return err
			}
			return linkProfiles(cmd, selected)
		},
	}
}

func selectProfiles(m *profiles.Manager, names []string) ([]profiles.Profile, error) {
	if len(names) == 0 {
		return m.List()
	}
	out := make([]profiles.Profile, 0, len(names))
	for _, name := range names {
		p, err := m.Get(name)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// linkProfiles shares the configured entries with each profile and reports
// every result. All profiles are processed even if one has conflicts.
func linkProfiles(cmd *cobra.Command, list []profiles.Profile) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	source, err := cfg.ResolveSourceDir()
	if err != nil {
		return err
	}
	failed := 0
	for _, p := range list {
		results, err := linker.Link(source, p.Dir, cfg.Link)
		for _, r := range results {
			printf(cmd, "%s: %s %s\n", p.Name, r.Name, describeResult(r))
		}
		if err != nil {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d profile(s) could not be fully linked", failed)
	}
	return nil
}

// relinkShared restores the shared entries of p before claude starts, for
// example a settings.json that claude replaced by an identical copy. It is best
// effort: whatever it cannot fix is reported by doctor and repair.
func relinkShared(p profiles.Profile) {
	cfg, err := loadConfig()
	if err != nil {
		return
	}
	source, err := cfg.ResolveSourceDir()
	if err != nil {
		return
	}
	_, _ = linker.Link(source, p.Dir, cfg.Link)
}

func describeResult(r linker.Result) string {
	if r.Detail != "" {
		return fmt.Sprintf("%s (%s)", r.Outcome, r.Detail)
	}
	return string(r.Outcome)
}

func loadConfig() (config.Config, error) {
	path, err := config.Path()
	if err != nil {
		return config.Config{}, err
	}
	return config.Load(path)
}
