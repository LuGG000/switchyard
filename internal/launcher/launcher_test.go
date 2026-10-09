package launcher

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/LuGG000/switchyard/internal/profiles"
)

var fakeClaude string

// TestMain builds fakeclaude once for all tests.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fakeclaude")
	if err != nil {
		panic(err)
	}
	name := "fakeclaude"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	fakeClaude = filepath.Join(dir, name)
	if out, err := exec.Command("go", "build", "-o", fakeClaude, "../../fakeclaude").CombinedOutput(); err != nil {
		panic(string(out))
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

type report struct {
	Args      []string `json:"args"`
	ConfigDir string   `json:"configDir"`
	EnvNames  []string `json:"envNames"`
}

func run(t *testing.T, env []string, args ...string) (report, int) {
	t.Helper()
	var out bytes.Buffer
	l := &Launcher{Claude: fakeClaude, Environ: env, Stdout: &out, Stderr: os.Stderr}
	p := profiles.Profile{Name: "a", Dir: t.TempDir()}
	code, err := l.Run(context.Background(), p, args)
	if err != nil {
		t.Fatal(err)
	}
	var r report
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("decode report %q: %v", out.String(), err)
	}
	if r.ConfigDir != "" && r.ConfigDir != p.Dir {
		t.Errorf("config dir %q, want %q", r.ConfigDir, p.Dir)
	}
	return r, code
}

func TestRunPassesArgsAndProfileDir(t *testing.T) {
	r, code := run(t, os.Environ(), "--model", "haiku")
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if !slices.Equal(r.Args, []string{"--model", "haiku"}) {
		t.Errorf("args %v", r.Args)
	}
	if r.ConfigDir == "" {
		t.Error("CLAUDE_CONFIG_DIR not set")
	}
}

func TestRunScrubsCredentials(t *testing.T) {
	env := append(os.Environ(),
		"ANTHROPIC_API_KEY=x", "ANTHROPIC_AUTH_TOKEN=x", "CLAUDE_CODE_OAUTH_TOKEN=x",
		"CLAUDE_CODE_USE_BEDROCK=1", "CLAUDE_CODE_USE_VERTEX=1")
	r, _ := run(t, env)
	if len(r.EnvNames) != 0 {
		t.Errorf("credential variables reached claude: %v", r.EnvNames)
	}
}

func TestRunReturnsExitCode(t *testing.T) {
	_, code := run(t, append(os.Environ(), "FAKE_EXIT=3"))
	if code != 3 {
		t.Errorf("exit code %d, want 3", code)
	}
}

func TestRunReportsMissingExecutable(t *testing.T) {
	l := &Launcher{Claude: filepath.Join(t.TempDir(), "absent")}
	if _, err := l.Run(context.Background(), profiles.Profile{Name: "a", Dir: t.TempDir()}, nil); err == nil {
		t.Error("expected an error")
	}
}
