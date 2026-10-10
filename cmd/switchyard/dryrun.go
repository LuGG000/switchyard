package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/LuGG000/switchyard/internal/claudeenv"
	"github.com/LuGG000/switchyard/internal/profiles"
	"github.com/LuGG000/switchyard/internal/selector"
	"github.com/LuGG000/switchyard/internal/state"
)

// rootOnlyManager is a profile manager that does not need claude, for commands that start nothing.
func rootOnlyManager() (*profiles.Manager, error) {
	root, err := profilesRoot()
	if err != nil {
		return nil, err
	}
	return &profiles.Manager{Root: root}, nil
}

// printPlan shows what starting claude in p with args would do, without starting it or
// changing any file: the profile, the failover settings, where a limit would lead and the
// exact claude command line including the settings switchyard injects.
func printPlan(cmd *cobra.Command, m *profiles.Manager, store *state.Store, p profiles.Profile, args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	list, err := m.List()
	if err != nil {
		return err
	}
	claudeArgs, err := withSignalSettings(p, args)
	if err != nil {
		return err
	}

	kind := "interactive"
	if isHeadless(args) {
		kind = "headless (no terminal to ask: a limit always switches on, see the limits below)"
	}
	claude, lookErr := exec.LookPath("claude")
	if lookErr != nil {
		claude = "claude (not found in PATH)"
	}
	printf(cmd, "Profile:     %s (%s)\n", p.Name, p.Dir)
	printf(cmd, "Run:         %s\n", kind)
	printf(cmd, "At a limit:  mode %s, strategy %s, carry_context %t\n", cfg.Mode, cfg.Strategy, cfg.CarryContext)
	printf(cmd, "Thresholds:  five-hour %s, weekly %s\n", percentOrOff(cfg.ProactiveThreshold), percentOrOff(cfg.ProactiveThresholdWeekly))
	printf(cmd, "Limits:      %s between automatic switches, %s per 24 hours\n", minutesOrOff(cfg.MinSwitchInterval), countOrOff(cfg.MaxAutoSwitches))
	printf(cmd, "If %s hits its limit: %s\n", p.Name, nextAfterLimit(store, list, p, cfg.Strategy, selector.ThresholdsFromConfig(cfg)))
	printf(cmd, "Environment: CLAUDE_CONFIG_DIR=%s", p.Dir)
	if names := credentialVariables(os.Environ()); len(names) > 0 {
		printf(cmd, "; removed from the environment: %s", strings.Join(names, ", "))
	}
	printf(cmd, "\n%s\n", claude)
	for _, a := range claudeArgs {
		printf(cmd, "  %s\n", a)
	}
	println(cmd, "Dry run: nothing was started and nothing was changed.")
	return nil
}

// nextAfterLimit says which profile would take over if p hit its limit now.
func nextAfterLimit(store *state.Store, list []profiles.Profile, p profiles.Profile, strategy string, thresholds selector.Thresholds) string {
	st, err := store.Read()
	if err != nil {
		return "unknown (state.json cannot be read)"
	}
	now := time.Now()
	candidates := make([]selector.Candidate, len(list))
	for i, other := range list {
		candidates[i] = selector.Candidate{Name: other.Name, State: st.Profiles[other.Name]}
		if other.Name == p.Name {
			candidates[i].State.CooldownUntil = now.Add(time.Hour)
		}
	}
	name, err := selector.Next(strategy, p.Name, candidates, thresholds, now)
	var locked *selector.AllLockedError
	switch {
	case errors.As(err, &locked):
		return fmt.Sprintf("no other profile is available; the first one is back at %s", locked.EarliestReset.Local().Format("15:04"))
	case err != nil:
		return "unknown (" + err.Error() + ")"
	case name == p.Name:
		return "no other profile to move to"
	}
	return name
}

// credentialVariables lists the names, never the values, of the variables switchyard removes.
func credentialVariables(environ []string) []string {
	var names []string
	for _, entry := range environ {
		if name, _, _ := strings.Cut(entry, "="); claudeenv.IsCredentialVar(name) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

func percentOrOff(n int) string {
	if n == 0 {
		return "off"
	}
	return fmt.Sprintf("%d%%", n)
}

func minutesOrOff(n int) string {
	if n == 0 {
		return "no minimum"
	}
	return fmt.Sprintf("at least %d minutes", n)
}

func countOrOff(n int) string {
	if n == 0 {
		return "no maximum"
	}
	return fmt.Sprintf("at most %d", n)
}

// dryRunPlan is `run --dry-run`: the plan for the profile run would start with.
func dryRunPlan(cmd *cobra.Command, profileName string, args []string) error {
	m, err := rootOnlyManager()
	if err != nil {
		return err
	}
	store, err := newStateStore()
	if err != nil {
		return err
	}
	p, err := resolveProfile(m, store, profileName)
	if err != nil {
		return err
	}
	return printPlan(cmd, m, store, p, args)
}

// dryRunSwitch is `switch --dry-run`: what switching to args[0] would do.
func dryRunSwitch(cmd *cobra.Command, carryContext bool, args []string, resume, fresh, noLaunch bool) error {
	m, err := rootOnlyManager()
	if err != nil {
		return err
	}
	store, err := newStateStore()
	if err != nil {
		return err
	}
	p, err := m.Get(args[0])
	if err != nil {
		return err
	}
	if noLaunch {
		printf(cmd, "Would make %s the active profile and start nothing.\n", p.Name)
		println(cmd, "Dry run: nothing was changed.")
		return nil
	}
	carry := carryContext
	if resume || fresh {
		carry = resume
	}
	claudeArgs := args[1:]
	if carry {
		claudeArgs = append([]string{"--continue"}, claudeArgs...)
		println(cmd, "The conversation is continued in the new profile (--continue).")
	} else {
		println(cmd, "A new conversation is started.")
	}
	return printPlan(cmd, m, store, p, claudeArgs)
}
