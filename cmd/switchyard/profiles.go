package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/LuGG000/switchyard/internal/config"
	"github.com/LuGG000/switchyard/internal/profiles"
	"github.com/LuGG000/switchyard/internal/state"
)

const profilesSubdir = "profiles"

// printf and println write to the command's stdout; a failed write to a closed
// terminal is not actionable.
func printf(cmd *cobra.Command, format string, args ...any) {
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), format, args...)
}

func println(cmd *cobra.Command, msg string) {
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), msg)
}

func profilesRoot() (string, error) {
	dataDir, err := config.DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, profilesSubdir), nil
}

func newProfileManager() (*profiles.Manager, error) {
	root, err := profilesRoot()
	if err != nil {
		return nil, err
	}
	claude, err := exec.LookPath("claude")
	if err != nil {
		return nil, errors.New("claude executable not found in PATH")
	}
	return &profiles.Manager{Root: root, Claude: claude, Environ: os.Environ()}, nil
}

func newAddCmd() *cobra.Command {
	var existing bool
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Create a profile and log it in with a claude.ai subscription",
		Long: "Create a profile and log it in with a claude.ai subscription.\n\n" +
			"With --existing the profile is your existing claude config directory (~/.claude, or source_dir\n" +
			"in the config) and its login: nothing is copied and there is no new login. The directory is\n" +
			"linked, so a plain claude and switchyard keep using the same sessions and settings.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := newProfileManager()
			if err != nil {
				return err
			}
			if existing {
				return adoptExisting(cmd, m, args[0])
			}
			p, err := m.Create(args[0])
			if err != nil {
				return err
			}
			printf(cmd, "Created profile %s\n", p.Name)
			if err := linkProfiles(cmd, []profiles.Profile{p}); err != nil {
				return err
			}
			return loginAndValidate(cmd, m, p)
		},
	}
	cmd.Flags().BoolVar(&existing, "existing", false, "use your existing claude config directory and its login as the profile")
	return cmd
}

func newLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login <name>",
		Short: "Log a profile in again",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := newProfileManager()
			if err != nil {
				return err
			}
			p, err := m.Get(args[0])
			if err != nil {
				return err
			}
			return loginAndValidate(cmd, m, p)
		},
	}
}

func newRemoveCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:     "remove <name>",
		Aliases: []string{"rm"},
		Short:   "Remove a profile",
		Long: "Remove a profile. Its login is deleted from this computer; the account itself is not affected.\n\n" +
			"A profile made with --existing is a link to your claude config directory: only the link is\n" +
			"removed, your sessions, settings and login stay where they are. The sessions and settings a\n" +
			"profile shares with the default directory are never deleted either.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := profilesRoot()
			if err != nil {
				return err
			}
			m := &profiles.Manager{Root: root}
			p, err := m.Get(args[0])
			if err != nil {
				return err
			}
			linked := m.Linked(p)
			if !yes {
				question := fmt.Sprintf("Remove profile %s and delete its login from this computer?", p.Name)
				if linked {
					question = fmt.Sprintf("Remove profile %s? Only the link is removed; your claude config directory stays.", p.Name)
				}
				if !confirm(cmd, question) {
					return errors.New("not removed (pass --yes to remove without asking)")
				}
			}
			store, err := newStateStore()
			if err != nil {
				return err
			}
			if err := m.Remove(p.Name); err != nil {
				return err
			}
			err = store.Update(func(st *state.State) error {
				delete(st.Profiles, p.Name)
				if st.Active == p.Name {
					st.Active = ""
				}
				return nil
			})
			if err != nil {
				return err
			}
			printf(cmd, "Removed profile %s\n", p.Name)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "remove without asking")
	return cmd
}

// confirm asks a yes/no question on the terminal; anything but an explicit yes, and any
// input that is not a terminal, counts as no.
func confirm(cmd *cobra.Command, question string) bool {
	in, ok := cmd.InOrStdin().(*os.File)
	if !ok || !isTerminal(in) {
		return false
	}
	printf(cmd, "%s [y/N] ", question)
	answer, err := bufio.NewReader(in).ReadString('\n')
	a := strings.ToLower(strings.TrimSpace(answer))
	return err == nil && (a == "y" || a == "yes")
}

func loginAndValidate(cmd *cobra.Command, m *profiles.Manager, p profiles.Profile) error {
	if err := m.Login(cmd.Context(), p, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		return fmt.Errorf("%w (retry with: switchyard login %s)", err, p.Name)
	}
	st, err := m.Validate(cmd.Context(), p)
	if err != nil {
		return fmt.Errorf("%w (retry with: switchyard login %s)", err, p.Name)
	}
	printf(cmd, "Profile %s is logged in (%s)\n", p.Name, describePlan(st))
	return nil
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List profiles and their login state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := newProfileManager()
			if err != nil {
				return err
			}
			list, err := m.List()
			if err != nil {
				return err
			}
			if len(list) == 0 {
				println(cmd, "No profiles yet. Create one with: switchyard add <name>")
				return nil
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "NAME\tLOGIN")
			for _, p := range list {
				_, _ = fmt.Fprintf(tw, "%s\t%s\n", p.Name, loginState(cmd.Context(), m, p))
			}
			return tw.Flush()
		},
	}
}

func loginState(ctx context.Context, m *profiles.Manager, p profiles.Profile) string {
	st, err := m.Status(ctx, p)
	switch {
	case err != nil:
		return "unknown"
	case !st.LoggedIn:
		return "logged out"
	case st.AuthMethod != profiles.SubscriptionAuth:
		return "not a subscription login (" + st.AuthMethod + ")"
	}
	return describePlan(st)
}

func describePlan(st profiles.AuthStatus) string {
	if st.SubscriptionType == "" {
		return "subscription"
	}
	return st.SubscriptionType
}

// adoptExisting makes the existing claude config directory the profile name and checks its login.
func adoptExisting(cmd *cobra.Command, m *profiles.Manager, name string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	source, err := cfg.ResolveSourceDir()
	if err != nil {
		return err
	}
	p, err := m.Adopt(name, source)
	if err != nil {
		return err
	}
	printf(cmd, "Profile %s uses your existing config directory %s\n", p.Name, source)
	st, err := m.Validate(cmd.Context(), p)
	if err != nil {
		return fmt.Errorf("%w (log in with: switchyard login %s)", err, p.Name)
	}
	printf(cmd, "Profile %s is logged in (%s)\n", p.Name, describePlan(st))
	return nil
}
