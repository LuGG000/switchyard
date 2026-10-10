// Package profiles manages switchyard profiles: named claude config directories
// that each hold one subscription login.
package profiles

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"github.com/LuGG000/switchyard/internal/claudeenv"
	"github.com/LuGG000/switchyard/internal/linker"
)

// SubscriptionAuth is the authMethod reported for a claude.ai subscription login.
const SubscriptionAuth = "claude.ai"

var (
	// ErrExists is returned when a profile with the given name already exists.
	ErrExists = errors.New("profile already exists")
	// ErrNotFound is returned when a profile does not exist.
	ErrNotFound = errors.New("profile not found")
	// ErrNotSubscription is returned when a profile is not logged in with a subscription.
	ErrNotSubscription = errors.New("profile is not logged in with a claude.ai subscription")

	namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
)

// Profile is one named config directory.
type Profile struct {
	Name string
	Dir  string
}

// AuthStatus holds the only fields of `claude auth status` that switchyard reads.
// The command also prints email and organization data, which is never parsed.
type AuthStatus struct {
	LoggedIn         bool   `json:"loggedIn"`
	AuthMethod       string `json:"authMethod"`
	SubscriptionType string `json:"subscriptionType"`
	ConfigDirectory  string `json:"configDirectory"`
}

// Manager creates and inspects profiles below Root.
type Manager struct {
	// Root is the directory containing one subdirectory per profile.
	Root string
	// Claude is the claude executable to run.
	Claude string
	// Environ is the environment passed to claude before scrubbing.
	Environ []string
}

// ValidName reports whether name is usable as a profile name.
func ValidName(name string) bool {
	return namePattern.MatchString(name)
}

// Create makes the config directory for a new profile.
func (m *Manager) Create(name string) (Profile, error) {
	if !ValidName(name) {
		return Profile{}, fmt.Errorf("invalid profile name %q: use 1-32 lowercase letters, digits, '-' or '_'", name)
	}
	if err := os.MkdirAll(m.Root, 0o700); err != nil {
		return Profile{}, fmt.Errorf("create profiles dir: %w", err)
	}
	p := m.profile(name)
	if err := os.Mkdir(p.Dir, 0o700); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return Profile{}, fmt.Errorf("%w: %s", ErrExists, name)
		}
		return Profile{}, fmt.Errorf("create profile dir: %w", err)
	}
	return p, nil
}

// Adopt makes an existing claude config directory a profile without copying anything:
// the profile directory is a link to source, so its login and its shared entries are
// the ones already there. The same directory cannot be adopted twice.
func (m *Manager) Adopt(name, source string) (Profile, error) {
	if !ValidName(name) {
		return Profile{}, fmt.Errorf("invalid profile name %q: use 1-32 lowercase letters, digits, '-' or '_'", name)
	}
	info, err := os.Stat(source)
	if err != nil || !info.IsDir() {
		return Profile{}, fmt.Errorf("config directory %s does not exist", source)
	}
	existing, err := m.List()
	if err != nil {
		return Profile{}, err
	}
	for _, p := range existing {
		if pi, err := os.Stat(p.Dir); err == nil && os.SameFile(pi, info) {
			return Profile{}, fmt.Errorf("%s is already the profile %s", source, p.Name)
		}
	}
	if err := os.MkdirAll(m.Root, 0o700); err != nil {
		return Profile{}, fmt.Errorf("create profiles dir: %w", err)
	}
	p := m.profile(name)
	if _, err := os.Lstat(p.Dir); err == nil {
		return Profile{}, fmt.Errorf("%w: %s", ErrExists, name)
	}
	if err := linker.LinkDir(source, p.Dir); err != nil {
		return Profile{}, fmt.Errorf("link profile to %s: %w", source, err)
	}
	return p, nil
}

// Get returns the named profile if it exists.
func (m *Manager) Get(name string) (Profile, error) {
	if !ValidName(name) {
		return Profile{}, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	p := m.profile(name)
	info, err := os.Stat(p.Dir)
	if err != nil || !info.IsDir() {
		return Profile{}, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return p, nil
}

// List returns all profiles in name order.
func (m *Manager) List() ([]Profile, error) {
	entries, err := os.ReadDir(m.Root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list profiles: %w", err)
	}
	var out []Profile
	for _, e := range entries {
		// Get follows links, so a profile directory may be a symlink or junction.
		if p, err := m.Get(e.Name()); err == nil {
			out = append(out, p)
		}
	}
	return out, nil
}

// Login runs the interactive `claude auth login` for p on the given streams.
func (m *Manager) Login(ctx context.Context, p Profile, stdin io.Reader, stdout, stderr io.Writer) error {
	cmd := m.command(ctx, p, "auth", "login")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("claude auth login: %w", err)
	}
	return nil
}

// Status reports the login state of p. claude exits non-zero when nobody is
// logged in but still prints the status JSON, so that case is not an error.
func (m *Manager) Status(ctx context.Context, p Profile) (AuthStatus, error) {
	cmd := m.command(ctx, p, "auth", "status")
	var out bytes.Buffer
	cmd.Stdout = &out
	var exitErr *exec.ExitError
	if err := cmd.Run(); err != nil && !errors.As(err, &exitErr) {
		return AuthStatus{}, fmt.Errorf("claude auth status: %w", err)
	}
	var st AuthStatus
	if err := json.Unmarshal(out.Bytes(), &st); err != nil {
		return AuthStatus{}, errors.New("claude auth status: unexpected output")
	}
	return st, nil
}

// Validate returns ErrNotSubscription unless p holds a claude.ai subscription login.
func (m *Manager) Validate(ctx context.Context, p Profile) (AuthStatus, error) {
	st, err := m.Status(ctx, p)
	if err != nil {
		return st, err
	}
	if !st.LoggedIn || st.AuthMethod != SubscriptionAuth {
		return st, fmt.Errorf("%w: %s", ErrNotSubscription, p.Name)
	}
	return st, nil
}

func (m *Manager) profile(name string) Profile {
	return Profile{Name: name, Dir: filepath.Join(m.Root, name)}
}

func (m *Manager) command(ctx context.Context, p Profile, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, m.Claude, args...)
	cmd.Env = claudeenv.ForProfile(m.Environ, p.Dir)
	return cmd
}
