// Package doctor inspects a switchyard setup for problems that would make
// failover pick the wrong login or lose shared sessions.
package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/LuGG000/switchyard/internal/claudeenv"
	"github.com/LuGG000/switchyard/internal/config"
	"github.com/LuGG000/switchyard/internal/linker"
	"github.com/LuGG000/switchyard/internal/profiles"
)

// Level ranks a finding.
type Level int

// Finding levels.
const (
	OK Level = iota
	Warn
	Error
)

func (l Level) String() string {
	switch l {
	case OK:
		return "ok"
	case Warn:
		return "warn"
	}
	return "error"
}

// Finding is one result of a check.
type Finding struct {
	Level   Level
	Subject string
	Message string
}

// Doctor runs the checks.
type Doctor struct {
	Profiles *profiles.Manager
	Config   config.Config
	// SourceDir is the resolved config.SourceDir.
	SourceDir string
	// Environ is the process environment to inspect.
	Environ []string
}

// Run executes all checks and returns the findings in a stable order.
func (d *Doctor) Run(ctx context.Context) ([]Finding, error) {
	list, err := d.Profiles.List()
	if err != nil {
		return nil, err
	}
	findings := d.checkEnvironment()
	if len(list) == 0 {
		return append(findings, Finding{Warn, "profiles", "no profiles yet; create one with: switchyard add <name>"}), nil
	}
	for _, p := range list {
		findings = append(findings, d.checkAuth(ctx, p)...)
		findings = append(findings, d.checkSettings(p)...)
		findings = append(findings, d.checkLinks(p)...)
	}
	return findings, nil
}

// Worst returns the highest level among findings.
func Worst(findings []Finding) Level {
	worst := OK
	for _, f := range findings {
		worst = max(worst, f.Level)
	}
	return worst
}

func (d *Doctor) checkEnvironment() []Finding {
	var set []string
	for _, entry := range d.Environ {
		name, value, _ := strings.Cut(entry, "=")
		if value != "" && claudeenv.IsCredentialVar(name) {
			set = append(set, name)
		}
	}
	if len(set) == 0 {
		return []Finding{{OK, "environment", "no credential variables set"}}
	}
	slices.Sort(set)
	return []Finding{{Warn, "environment", fmt.Sprintf(
		"%s set; they override the subscription login when claude is started directly. switchyard run removes them",
		strings.Join(set, ", "))}}
}

func (d *Doctor) checkAuth(ctx context.Context, p profiles.Profile) []Finding {
	st, err := d.Profiles.Status(ctx, p)
	switch {
	case err != nil:
		return []Finding{{Error, p.Name, "cannot read login state: " + err.Error()}}
	case !st.LoggedIn:
		return []Finding{{Error, p.Name, "not logged in; run: switchyard login " + p.Name}}
	case st.AuthMethod != profiles.SubscriptionAuth:
		return []Finding{{Error, p.Name, fmt.Sprintf("login is %q, not a claude.ai subscription", st.AuthMethod)}}
	}
	return []Finding{{OK, p.Name, "subscription login (" + st.SubscriptionType + ")"}}
}

// checkSettings looks for settings that bypass the subscription login. Only
// the key names are read from settings.json.
func (d *Doctor) checkSettings(p profiles.Profile) []Finding {
	data, err := os.ReadFile(filepath.Join(p.Dir, "settings.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return []Finding{{Warn, p.Name, "cannot read settings.json: " + err.Error()}}
	}
	var settings struct {
		APIKeyHelper string                     `json:"apiKeyHelper"`
		Env          map[string]json.RawMessage `json:"env"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return []Finding{{Warn, p.Name, "settings.json is not valid JSON"}}
	}
	var findings []Finding
	if settings.APIKeyHelper != "" {
		findings = append(findings, Finding{Warn, p.Name, "settings.json sets apiKeyHelper, which can replace the subscription login"})
	}
	var names []string
	for name := range settings.Env {
		if claudeenv.IsCredentialVar(name) {
			names = append(names, name)
		}
	}
	if len(names) > 0 {
		slices.Sort(names)
		findings = append(findings, Finding{Warn, p.Name, "settings.json env sets " + strings.Join(names, ", ")})
	}
	return findings
}

func (d *Doctor) checkLinks(p profiles.Profile) []Finding {
	var findings []Finding
	for _, r := range linker.Check(d.SourceDir, p.Dir, d.Config.Link) {
		switch r.Outcome {
		case linker.Unchanged, linker.SourceMissing:
		case linker.Missing, linker.Broken:
			findings = append(findings, Finding{Error, p.Name, fmt.Sprintf("%s: link %s; run: switchyard repair %s", r.Name, r.Outcome, p.Name)})
		default:
			findings = append(findings, Finding{Error, p.Name, fmt.Sprintf("%s: %s", r.Name, r.Detail)})
		}
	}
	if len(findings) == 0 {
		findings = append(findings, Finding{OK, p.Name, "shared entries are linked"})
	}
	return findings
}
