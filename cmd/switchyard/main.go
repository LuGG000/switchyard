package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
)

var version = "0.0.0-dev"

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "switchyard",
		Short:         "Account switcher with limit failover for Claude Code subscription logins",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetVersionTemplate("switchyard {{.Version}}\n")
	root.AddCommand(newAddCmd(), newLoginCmd(), newListCmd(), newRepairCmd())
	return root
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	err := newRootCmd().ExecuteContext(ctx)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "switchyard:", err)
		os.Exit(1)
	}
}
