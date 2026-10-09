package doctor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LuGG000/switchyard/internal/config"
	"github.com/LuGG000/switchyard/internal/linker"
	"github.com/LuGG000/switchyard/internal/profiles"
)

const fakeEnv = "SWITCHYARD_FAKE_CLAUDE"

// TestMain lets the test binary double as a minimal fake claude for `auth status`.
func TestMain(m *testing.M) {
	if os.Getenv(fakeEnv) != "" {
		loggedIn := os.Getenv("FAKE_LOGGED_IN")
		fmt.Printf(`{"loggedIn":%s,"authMethod":%q,"subscriptionType":"pro"}`, loggedIn, os.Getenv("FAKE_AUTH_METHOD"))
		if loggedIn != "true" {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type fixture struct {
	doctor  *Doctor
	source  string
	profile profiles.Profile
}

func newFixture(t *testing.T, loggedIn bool, method string, environ ...string) *fixture {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), fakeEnv+"=1", fmt.Sprintf("FAKE_LOGGED_IN=%t", loggedIn), "FAKE_AUTH_METHOD="+method)
	m := &profiles.Manager{Root: t.TempDir(), Claude: exe, Environ: env}
	p, err := m.Create("a")
	if err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	cfg := config.Default()
	cfg.Link = []string{"projects"}
	if err := os.Mkdir(filepath.Join(source, "projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := linker.Link(source, p.Dir, cfg.Link); err != nil {
		t.Fatal(err)
	}
	return &fixture{doctor: &Doctor{Profiles: m, Config: cfg, SourceDir: source, Environ: environ}, source: source, profile: p}
}

func (f *fixture) run(t *testing.T) []Finding {
	t.Helper()
	findings, err := f.doctor.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return findings
}

func has(findings []Finding, level Level, subject, fragment string) bool {
	for _, f := range findings {
		if f.Level == level && f.Subject == subject && strings.Contains(f.Message, fragment) {
			return true
		}
	}
	return false
}

func TestHealthySetupHasNoProblems(t *testing.T) {
	f := newFixture(t, true, profiles.SubscriptionAuth)
	findings := f.run(t)
	if Worst(findings) != OK {
		t.Errorf("unexpected findings: %+v", findings)
	}
	if !has(findings, OK, "a", "subscription login (pro)") {
		t.Errorf("missing auth confirmation: %+v", findings)
	}
}

func TestNonSubscriptionLoginIsAnError(t *testing.T) {
	f := newFixture(t, true, "api_key")
	if !has(f.run(t), Error, "a", "not a claude.ai subscription") {
		t.Error("api_key login not reported")
	}
}

func TestLoggedOutProfileIsAnError(t *testing.T) {
	f := newFixture(t, false, "none")
	if !has(f.run(t), Error, "a", "not logged in") {
		t.Error("logged out profile not reported")
	}
}

func TestAPIKeyHelperAndEnvSettingsAreWarnings(t *testing.T) {
	f := newFixture(t, true, profiles.SubscriptionAuth)
	settings := `{"apiKeyHelper":"/bin/key","env":{"ANTHROPIC_API_KEY":"x","OTHER":"y"}}`
	if err := os.WriteFile(filepath.Join(f.profile.Dir, "settings.json"), []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	findings := f.run(t)
	if !has(findings, Warn, "a", "apiKeyHelper") || !has(findings, Warn, "a", "ANTHROPIC_API_KEY") {
		t.Errorf("settings problems not reported: %+v", findings)
	}
	for _, fd := range findings {
		if strings.Contains(fd.Message, "OTHER") || strings.Contains(fd.Message, "/bin/key") {
			t.Errorf("finding leaks settings content: %q", fd.Message)
		}
	}
}

func TestMissingLinkIsAnError(t *testing.T) {
	f := newFixture(t, true, profiles.SubscriptionAuth)
	f.doctor.Config.Link = []string{"projects", "agents"}
	if err := os.Mkdir(filepath.Join(f.source, "agents"), 0o700); err != nil {
		t.Fatal(err)
	}
	if !has(f.run(t), Error, "a", "agents: link missing") {
		t.Error("missing link not reported")
	}
}

func TestCredentialEnvironmentIsWarnedAboutWithoutValues(t *testing.T) {
	f := newFixture(t, true, profiles.SubscriptionAuth, "ANTHROPIC_API_KEY=sk-secret", "PATH=/bin")
	findings := f.run(t)
	if !has(findings, Warn, "environment", "ANTHROPIC_API_KEY") {
		t.Errorf("credential variable not reported: %+v", findings)
	}
	for _, fd := range findings {
		if strings.Contains(fd.Message, "sk-secret") {
			t.Error("finding leaks a credential value")
		}
	}
}

func TestNoProfilesIsAWarning(t *testing.T) {
	d := &Doctor{Profiles: &profiles.Manager{Root: t.TempDir()}, Config: config.Default(), SourceDir: t.TempDir()}
	findings, err := d.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !has(findings, Warn, "profiles", "no profiles") {
		t.Errorf("unexpected findings: %+v", findings)
	}
}

func TestIdenticalCopyOfSharedFileIsAWarning(t *testing.T) {
	f := newFixture(t, true, profiles.SubscriptionAuth)
	f.doctor.Config.Link = []string{"projects", "settings.json"}
	if err := os.WriteFile(filepath.Join(f.source, "settings.json"), []byte(`{"a":1,"b":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.profile.Dir, "settings.json"), []byte(`{"b":2,"a":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if !has(f.run(t), Warn, "a", "settings.json: identical copy") {
		t.Error("identical copy not reported as a warning")
	}
}
