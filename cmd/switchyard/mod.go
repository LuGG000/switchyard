package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

const (
	// defaultModSource is the GitHub repository whose marketplace file lists the mod.
	defaultModSource = "LuGG000/switchyard"
	modMarketplace   = "switchyard"
	modPlugin        = "switchyard-mod"
)

func newModCmd() *cobra.Command {
	mod := &cobra.Command{
		Use:   "mod",
		Short: "Manage the optional Claude Code mod",
	}
	var source, scope string
	install := &cobra.Command{
		Use:   "install",
		Short: "Install the switchyard mod into Claude Code",
		Long: "Add the switchyard marketplace to Claude Code and install the mod from it. The mod shows\n" +
			"the account usage, adds /accounts and /switch, and works in every profile because the\n" +
			"plugins directory is shared. --source takes a GitHub repository (owner/repo), a git URL or\n" +
			"a local checkout of this repository.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			claude, err := exec.LookPath("claude")
			if err != nil {
				return fmt.Errorf("claude executable not found in PATH")
			}
			run := func(claudeArgs ...string) error {
				c := exec.CommandContext(cmd.Context(), claude, claudeArgs...)
				c.Env = os.Environ()
				c.Stdin, c.Stdout, c.Stderr = cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()
				return c.Run()
			}
			if err := installMod(run, source, scope); err != nil {
				return err
			}
			println(cmd, "Installed "+modPlugin+". Restart claude to load it.")
			return nil
		},
	}
	install.Flags().StringVar(&source, "source", defaultModSource, "marketplace source: owner/repo, git URL or local path")
	install.Flags().StringVar(&scope, "scope", "user", "installation scope: user, project or local")
	mod.AddCommand(install)
	return mod
}

// installMod adds the marketplace and installs the mod from it using run, which
// executes `claude` with the given arguments.
func installMod(run func(args ...string) error, source, scope string) error {
	if err := run("plugin", "marketplace", "add", source, "--scope", scope); err != nil {
		return fmt.Errorf("add marketplace %s: %w", source, err)
	}
	if err := run("plugin", "install", modPlugin+"@"+modMarketplace, "--scope", scope); err != nil {
		return fmt.Errorf("install %s: %w", modPlugin, err)
	}
	return nil
}
