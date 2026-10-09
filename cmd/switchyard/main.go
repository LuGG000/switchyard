package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
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
	root.AddCommand(newAddCmd(), newLoginCmd(), newListCmd(), newRepairCmd(), newRunCmd(), newStatusCmd(), newSwitchCmd(), newHandoffCmd(), newModCmd(), newConfigCmd(), newInitCmd(), newDoctorCmd(), newUpdateCmd(), newHookCmd())
	return root
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	err := newRootCmd().ExecuteContext(ctx)
	stop()
	var exit exitError
	switch {
	case errors.As(err, &exit):
		os.Exit(exit.code)
	case err != nil:
		fmt.Fprintln(os.Stderr, "switchyard:", err)
		os.Exit(1)
	}
}
