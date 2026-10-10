package profiles

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LuGG000/switchyard/internal/linker"
)

const fakeEnv = "SWITCHYARD_FAKE_CLAUDE"

// TestMain lets the test binary double as a minimal fake claude. It prints the
// status JSON from SWITCHYARD_FAKE_STATUS and fails if credentials leaked in.
func TestMain(m *testing.M) {
	if os.Getenv(fakeEnv) != "" {
		os.Exit(fakeClaude(os.Args[1:]))
	}
	os.Exit(m.Run())
}

func fakeClaude(args []string) int {
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		fmt.Fprintln(os.Stderr, "credentials leaked into the environment")
		return 99
	}
	switch strings.Join(args, " ") {
	case "auth status":
		fmt.Printf(`{"loggedIn":%s,"authMethod":%q,"subscriptionType":"pro","configDirectory":%q,"email":"secret@example.com"}`,
			os.Getenv("FAKE_LOGGED_IN"), os.Getenv("FAKE_AUTH_METHOD"), os.Getenv("CLAUDE_CONFIG_DIR"))
		if os.Getenv("FAKE_LOGGED_IN") != "true" {
			return 1
		}
		return 0
	case "auth login":
		fmt.Printf("login in %s\n", os.Getenv("CLAUDE_CONFIG_DIR"))
		return 0
	}
	return 2
}

func newManager(t *testing.T, loggedIn bool, method string) *Manager {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(),
		fakeEnv+"=1",
		fmt.Sprintf("FAKE_LOGGED_IN=%t", loggedIn),
		"FAKE_AUTH_METHOD="+method,
		"ANTHROPIC_API_KEY=leak-check",
	)
	return &Manager{Root: t.TempDir(), Claude: exe, Environ: env}
}

func TestValidName(t *testing.T) {
	for name, want := range map[string]bool{
		"acc1": true, "a": true, "work_2": true, "my-acc": true,
		"": false, "Acc": false, "-a": false, "a b": false, "../x": false, "a/b": false,
		strings.Repeat("a", 33): false,
	} {
		if got := ValidName(name); got != want {
			t.Errorf("ValidName(%q) = %t, want %t", name, got, want)
		}
	}
}

func TestCreateGetList(t *testing.T) {
	m := newManager(t, true, SubscriptionAuth)
	for _, name := range []string{"b", "a"} {
		if _, err := m.Create(name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.Create("a"); !errors.Is(err, ErrExists) {
		t.Errorf("duplicate: got %v, want ErrExists", err)
	}
	if _, err := m.Create("Bad Name"); err == nil {
		t.Error("expected an error for an invalid name")
	}
	if _, err := m.Get("missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing: got %v, want ErrNotFound", err)
	}
	list, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Name != "a" || list[1].Name != "b" {
		t.Errorf("unexpected list: %+v", list)
	}
}

func TestListWithoutRootIsEmpty(t *testing.T) {
	m := &Manager{Root: t.TempDir() + "/none"}
	list, err := m.List()
	if err != nil || len(list) != 0 {
		t.Errorf("got %v, %v", list, err)
	}
}

func TestStatusParsesOnlyKnownFields(t *testing.T) {
	m := newManager(t, true, SubscriptionAuth)
	p, _ := m.Create("a")
	st, err := m.Status(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	want := AuthStatus{LoggedIn: true, AuthMethod: SubscriptionAuth, SubscriptionType: "pro", ConfigDirectory: p.Dir}
	if st != want {
		t.Errorf("got %+v, want %+v", st, want)
	}
}

func TestStatusNotLoggedInIsNotAnError(t *testing.T) {
	m := newManager(t, false, "none")
	p, _ := m.Create("a")
	st, err := m.Status(context.Background(), p)
	if err != nil || st.LoggedIn {
		t.Errorf("got %+v, %v", st, err)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name     string
		loggedIn bool
		method   string
		wantErr  bool
	}{
		{"subscription", true, SubscriptionAuth, false},
		{"api key", true, "api_key", true},
		{"logged out", false, "none", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newManager(t, tc.loggedIn, tc.method)
			p, _ := m.Create("a")
			_, err := m.Validate(context.Background(), p)
			if tc.wantErr && !errors.Is(err, ErrNotSubscription) {
				t.Errorf("got %v, want ErrNotSubscription", err)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestLoginUsesProfileDirAndScrubbedEnv(t *testing.T) {
	m := newManager(t, true, SubscriptionAuth)
	p, _ := m.Create("a")
	var out, errOut bytes.Buffer
	if err := m.Login(context.Background(), p, strings.NewReader(""), &out, &errOut); err != nil {
		t.Fatalf("%v: %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), p.Dir) {
		t.Errorf("login did not run in the profile dir: %q", out.String())
	}
}

func TestListFollowsLinkedProfileDirectories(t *testing.T) {
	m := &Manager{Root: t.TempDir()}
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(m.Root, "linked")); err != nil {
		t.Skipf("cannot create a symlink: %v", err)
	}
	if _, err := m.Create("plain"); err != nil {
		t.Fatal(err)
	}
	got, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "linked" || got[1].Name != "plain" {
		t.Errorf("List = %+v, want linked and plain", got)
	}
}

func TestAdoptLinksAnExistingConfigDirectory(t *testing.T) {
	m := newManager(t, true, SubscriptionAuth)
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "marker.txt"), []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}

	p, err := m.Adopt("main", source)
	if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(p.Dir, "marker.txt")); err != nil || string(data) != "mine" {
		t.Fatalf("the profile does not show the source directory: %q, %v", data, err)
	}
	list, err := m.List()
	if err != nil || len(list) != 1 || list[0].Name != "main" {
		t.Fatalf("List = %v, %v", list, err)
	}
	st, err := m.Validate(context.Background(), p)
	if err != nil || st.ConfigDirectory != p.Dir {
		t.Fatalf("Validate = %+v, %v", st, err)
	}
}

func TestAdoptRefusesWhatCannotBeAdopted(t *testing.T) {
	m := newManager(t, true, SubscriptionAuth)
	source := t.TempDir()
	if _, err := m.Adopt("main", source); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create("taken"); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()

	tests := []struct {
		name, profile, source, want string
	}{
		{"the same directory twice", "second", source, "already the profile main"},
		{"a name in use", "taken", other, "already exists"},
		{"a bad name", "Bad Name", other, "invalid profile name"},
		{"a missing directory", "third", filepath.Join(other, "nope"), "does not exist"},
	}
	for _, tt := range tests {
		if _, err := m.Adopt(tt.profile, tt.source); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: err = %v, want it to mention %q", tt.name, err, tt.want)
		}
	}
	if list, _ := m.List(); len(list) != 2 {
		t.Errorf("a refused adoption left a profile behind: %v", list)
	}
}

func TestRemoveOfALinkedProfileLeavesTheSourceAlone(t *testing.T) {
	m := newManager(t, true, SubscriptionAuth)
	source := t.TempDir()
	marker := filepath.Join(source, "marker.txt")
	if err := os.WriteFile(marker, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := m.Adopt("main", source)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Linked(p) {
		t.Fatal("an adopted profile is not reported as linked")
	}

	if err := m.Remove("main"); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "mine" {
		t.Fatalf("the source directory was touched: %q, %v", data, err)
	}
	if _, err := m.Get("main"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Remove = %v, want ErrNotFound", err)
	}
}

func TestRemoveDoesNotFollowSharedEntries(t *testing.T) {
	m := newManager(t, true, SubscriptionAuth)
	source := t.TempDir()
	shared := filepath.Join(source, "projects", "session.jsonl")
	if err := os.MkdirAll(filepath.Dir(shared), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shared, []byte("conversation"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := m.Create("zweit")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Dir, "own.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := linker.Link(source, p.Dir, []string{"projects"}); err != nil {
		t.Fatal(err)
	}
	if m.Linked(p) {
		t.Fatal("a created profile is reported as linked")
	}

	if err := m.Remove("zweit"); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(shared); err != nil || string(data) != "conversation" {
		t.Fatalf("the shared sessions were deleted: %q, %v", data, err)
	}
	if _, err := os.Lstat(p.Dir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the profile directory is still there: %v", err)
	}
}

func TestRemoveUnknownProfile(t *testing.T) {
	m := newManager(t, true, SubscriptionAuth)
	if err := m.Remove("nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Remove = %v, want ErrNotFound", err)
	}
}
