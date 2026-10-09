package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	var install bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Check for a newer release and optionally install it",
		Long: "Ask GitHub for the latest release and compare it with this binary.\n\n" +
			"With --install the release archive for this system is downloaded, checked against the\n" +
			"published checksums and put in place of this binary. A running claude session is not\n" +
			"touched: its hooks pick up the new binary on their next call, and a running\n" +
			"`switchyard run` keeps working until you start it again.\n\n" +
			"status, list and doctor also mention a newer release, at most once a day. Turn that\n" +
			"off with `switchyard config set update_check false` or " + noUpdateCheckEnv + "=1.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			current := currentVersion()
			exe, _ := executablePath()
			update.CleanOld(exe)
			checker, err := newUpdateChecker()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
			defer cancel()
			latest, err := checker.Latest(ctx, true)
			if err != nil {
				return fmt.Errorf("check for updates: %w (releases: %s)", err, update.ReleasesPage)
			}
			switch {
			case !update.Released(current):
				printf(cmd, "This is a development build (%s). Latest release: %s\n%s\n", current, latest.Version, latest.URL)
				if install {
					return errors.New("a development build is not replaced; install a release instead")
				}
			case !update.Newer(current, latest.Version):
				printf(cmd, "switchyard %s is up to date.\n", current)
			case !install:
				printf(cmd, "switchyard %s is available (you have %s).\n%s\nInstall it with: switchyard update --install\n", latest.Version, current, latest.URL)
			default:
				return installUpdate(ctx, cmd, exe, current)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&install, "install", false, "download the latest release and replace this binary")
	return cmd
}

func installUpdate(ctx context.Context, cmd *cobra.Command, exe, current string) error {
	if exe == "" {
		return errors.New("cannot locate this binary; install it again from the releases page: " + update.ReleasesPage)
	}
	installed, err := (&update.Installer{}).Install(ctx, exe)
	if err != nil {
		return fmt.Errorf("install: %w (manual install: %s)", err, update.ReleasesPage)
	}
	printf(cmd, "Updated switchyard %s to %s.\n"+
		"A running claude session keeps going: its hooks use the new version from now on.\n"+
		"Start `switchyard run` again to use the new version for the launcher too.\n"+
		"The mod is updated separately: claude plugin update switchyard-mod@switchyard\n", current, installed)
	return nil
}

// executablePath is the real path of this binary, links resolved.
func executablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// latestRelease returns the cached latest release and refreshes the cache when
// it is older than a day. It returns false when the check is switched off, this
// is a development build or no release is known.
func latestRelease(ctx context.Context, refresh bool) (update.Release, bool) {
	if os.Getenv(noUpdateCheckEnv) != "" {
		return update.Release{}, false
	}
	cfg, err := loadConfig()
	if err != nil || !cfg.UpdateCheck || !update.Released(currentVersion()) {
		return update.Release{}, false
	}
	checker, err := newUpdateChecker()
	if err != nil {
		return update.Release{}, false
	}
	if refresh {
		ctx, cancel := context.WithTimeout(ctx, updateTimeout)
		defer cancel()
		release, _ := checker.Latest(ctx, false)
		return release, release.Version != ""
	}
	return checker.Cached()
}

// newerRelease is the cached release if it is newer than this binary.
func newerRelease() (update.Release, bool) {
	release, ok := latestRelease(context.Background(), false)
	return release, ok && update.Newer(currentVersion(), release.Version)
}

// refreshUpdateCache looks for a release once a day without printing anything,
// so the status line and the mod know about it.
func refreshUpdateCache(ctx context.Context) {
	_, _ = latestRelease(ctx, true)
}

// noticeUpdate prints a one-line hint to stderr when a newer release is known.
// It stays silent when the check is off, stderr is not a terminal (scripts, the
// mod), or the lookup fails; the answer is cached for a day.
func noticeUpdate(cmd *cobra.Command) {
	if !isTerminal(os.Stderr) {
		return
	}
	release, ok := latestRelease(cmd.Context(), true)
	if ok && update.Newer(currentVersion(), release.Version) {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\nswitchyard %s is available (you have %s): %s\nInstall it with `switchyard update --install`; turn this hint off with `switchyard config set update_check false`.\n", release.Version, currentVersion(), release.URL)
	}
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
