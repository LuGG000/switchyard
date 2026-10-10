package main

import (
	"bufio"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/LuGG000/switchyard/internal/config"
	"github.com/LuGG000/switchyard/internal/profiles"
)

// firstProfile is the name offered for an existing login.
const firstProfile = "main"

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create the default config file and offer your existing login as the first profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := config.Path()
			if err != nil {
				return err
			}
			written, err := config.WriteDefault(path)
			if err != nil {
				return err
			}
			if written {
				printf(cmd, "Created %s\n", path)
			} else {
				printf(cmd, "Config already exists: %s\n", path)
			}
			if offerExisting(cmd) {
				return nil
			}
			println(cmd, "Next: switchyard add <name>")
			return nil
		},
	}
}

// offerExisting looks for a subscription login in the default claude config directory when there is
// no profile yet. In a terminal it asks whether to use it as the first profile; elsewhere it names
// the command. It reports whether the login was found and handled, and never fails init: whatever
// goes wrong here only means the user adds a profile themselves.
func offerExisting(cmd *cobra.Command) bool {
	m, err := newProfileManager()
	if err != nil {
		return false
	}
	if list, err := m.List(); err != nil || len(list) > 0 {
		return false
	}
	cfg, err := loadConfig()
	if err != nil {
		return false
	}
	source, err := cfg.ResolveSourceDir()
	if err != nil {
		return false
	}
	st, err := m.Validate(cmd.Context(), profiles.Profile{Name: firstProfile, Dir: source})
	if err != nil {
		return false
	}
	printf(cmd, "Found a subscription login (%s) in %s.\n", describePlan(st), source)
	in, ok := cmd.InOrStdin().(*os.File)
	if !ok || !isTerminal(in) {
		printf(cmd, "Use it as a profile without logging in again: switchyard add %s --existing\n", firstProfile)
		return true
	}
	printf(cmd, "Use it as the profile %q? [Y/n] ", firstProfile)
	answer, err := bufio.NewReader(in).ReadString('\n')
	// No input at all (a closed stdin) is no consent.
	if a := strings.ToLower(strings.TrimSpace(answer)); err != nil || (a != "" && a != "y" && a != "yes") {
		printf(cmd, "\nUse it later with: switchyard add %s --existing\n", firstProfile)
		return true
	}
	if err := adoptExisting(cmd, m, firstProfile); err != nil {
		printf(cmd, "Could not use it: %v\n", err)
	}
	return true
}
