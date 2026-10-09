package main

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"time"

	"github.com/spf13/cobra"

	"github.com/LuGG000/switchyard/internal/config"
	"github.com/LuGG000/switchyard/internal/update"
)

const (
	noUpdateCheckEnv = "SWITCHYARD_NO_UPDATE_CHECK"
	updateTimeout    = 2 * time.Second
	installCommand   = "go install github.com/" + update.Repo + "/cmd/switchyard@latest"
)

// currentVersion is the release this binary was built from. A binary built with
// `go install` carries no linker flag, so its module version is used instead.
func currentVersion() string {
	if update.Released(version) {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && update.Released(info.Main.Version) {
		return info.Main.Version
	}
	return version
}

func newUpdateChecker() (*update.Checker, error) {
	dataDir, err := config.DataDir()
	if err != nil {
		return nil, err
	}
	return &update.Checker{Dir: dataDir}, nil
}

func newUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Check for a newer release and show how to install it",
		Long: "Ask GitHub for the latest release and compare it with this binary. Nothing is\n" +
			"installed: the command prints where to get the new version.\n\n" +
			"status, list and doctor also mention a newer release, at most once a day. Turn that\n" +
			"off with `switchyard config set update_check false` or " + noUpdateCheckEnv + "=1.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			current := currentVersion()
			checker, err := newUpdateChecker()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()
			latest, err := checker.Latest(ctx, true)
			if err != nil {
				return fmt.Errorf("check for updates: %w (releases: %s)", err, update.ReleasesPage)
			}
			switch {
			case !update.Released(current):
				printf(cmd, "This is a development build (%s). Latest release: %s\n%s\n", current, latest.Version, latest.URL)
			case update.Newer(current, latest.Version):
				printf(cmd, "switchyard %s is available (you have %s).\n%s\nUpdate with: %s\n", latest.Version, current, latest.URL, installCommand)
			default:
				printf(cmd, "switchyard %s is up to date.\n", current)
			}
			return nil
		},
	}
}

// noticeUpdate prints a one-line hint to stderr when a newer release is known.
// It stays silent when the check is off, stderr is not a terminal (scripts, the
// mod), or the lookup fails; the answer is cached for a day.
func noticeUpdate(cmd *cobra.Command) {
	if os.Getenv(noUpdateCheckEnv) != "" {
		return
	}
	cfg, err := loadConfig()
	if err != nil || !cfg.UpdateCheck {
		return
	}
	current := currentVersion()
	if !update.Released(current) || !isTerminal(os.Stderr) {
		return
	}
	checker, err := newUpdateChecker()
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), updateTimeout)
	defer cancel()
	latest, _ := checker.Latest(ctx, false)
	if update.Newer(current, latest.Version) {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\nswitchyard %s is available (you have %s): %s\nRun `switchyard update` for details; turn this hint off with `switchyard config set update_check false`.\n", latest.Version, current, latest.URL)
	}
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
