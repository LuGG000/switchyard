package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"github.com/LuGG000/switchyard/internal/breaker"
	"github.com/LuGG000/switchyard/internal/selector"
)

var version = "0.0.0-dev"

// noticeCommands are the commands after which a newer release is mentioned.
var noticeCommands = map[string]bool{"status": true, "list": true, "doctor": true}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:     "switchyard",
		Short:   "Account switcher with limit failover for Claude Code subscription logins",
		Version: currentVersion(),
		PersistentPostRun: func(cmd *cobra.Command, _ []string) {
			if asJSON, _ := cmd.Flags().GetBool("json"); !asJSON && noticeCommands[cmd.Name()] {
				noticeUpdate(cmd)
			}
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetVersionTemplate("switchyard {{.Version}}\n")
	root.AddCommand(newAddCmd(), newLoginCmd(), newRemoveCmd(), newListCmd(), newRepairCmd(), newRunCmd(), newStatusCmd(), newSwitchCmd(), newHandoffCmd(), newDecisionCmd(), newHistoryCmd(), newWatchCmd(), newModCmd(), newConfigCmd(), newInitCmd(), newDoctorCmd(), newUpdateCmd(), newShellInitCmd(), newHookCmd())
	return root
}

// exitAllLimited is the exit code when every profile is at its limit, so scripts can wait and retry.
const exitAllLimited = 75

// exitSwitchPaused is the exit code when a headless run stops because the limits on automatic switches are reached.
const exitSwitchPaused = 76

// exitCodeFor maps the error of a command to the process exit code and the line to print, if any.
// claude's own exit code passes through; 75 means every profile is at its limit; 76 means the limits on automatic switches stopped a headless run; anything else is 1.
func exitCodeFor(err error) (int, string) {
	var exit exitError
	switch {
	case err == nil:
		return 0, ""
	case errors.As(err, &exit):
		return exit.code, ""
	case errors.As(err, new(*selector.AllLockedError)):
		return exitAllLimited, "switchyard: " + err.Error()
	case errors.As(err, new(*breaker.TrippedError)):
		return exitSwitchPaused, "switchyard: " + err.Error() + " (change the limits with: switchyard config set min_switch_interval_minutes <n> or max_auto_switches_per_day <n>, 0 = off)"
	default:
		return 1, "switchyard: " + err.Error()
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	err := newRootCmd().ExecuteContext(ctx)
	stop()
	code, message := exitCodeFor(err)
	if message != "" {
		fmt.Fprintln(os.Stderr, message)
	}
	os.Exit(code)
}
