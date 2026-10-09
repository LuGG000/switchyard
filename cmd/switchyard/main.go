package main

import (
	"fmt"
	"os"

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
	return root
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "switchyard:", err)
		os.Exit(1)
	}
}
