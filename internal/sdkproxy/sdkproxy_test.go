package sdkproxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LuGG000/switchyard/internal/config"
	"github.com/LuGG000/switchyard/internal/history"
	"github.com/LuGG000/switchyard/internal/launcher"
	"github.com/LuGG000/switchyard/internal/profiles"
	"github.com/LuGG000/switchyard/internal/state"
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

var sdkArgs = []string{"--output-format", "stream-json", "--verbose", "--input-format", "stream-json", "--session-id=s-1"}

// client is the SDK side of a test: it writes messages to the proxy and reads its output.
type client struct {
	t     *testing.T
	in    *io.PipeWriter
	lines chan map[string]any
	// seen holds every message read so far, including those next skipped.
	seen []map[string]any
}

func (c *client) send(msg string) {
	c.t.Helper()
	if _, err := io.WriteString(c.in, msg+"\n"); err != nil {
		c.t.Fatal(err)
	}
}

// next returns the next message of the given type, failing after a few seconds.
func (c *client) next(typ string) map[string]any {
	c.t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case m, ok := <-c.lines:
			if !ok {
				c.t.Fatalf("output ended before a %s message", typ)
			}
			c.seen = append(c.seen, m)
			if m["type"] == typ {
				return m
			}
		case <-timeout:
			c.t.Fatalf("no %s message", typ)
		}
	}
}

// drain reads the output to its end.
func (c *client) drain() {
	for m := range c.lines {
		c.seen = append(c.seen, m)
	}
}

// responses counts the control responses to the request with the given ID.
func (c *client) responses(id string) int {
	n := 0
	for _, m := range c.seen {
		if r, ok := m["response"].(map[string]any); ok && m["type"] == "control_response" && r["request_id"] == id {
			n++
		}
	}
	return n
}

type fixture struct {
	proxy    *Proxy
	store    *state.Store
	notice   *bytes.Buffer
	notified []string
	switches []history.Entry
	prepared []string
	mu       sync.Mutex
}

// newFixture sets up the named profiles; those whose name contains "limited" hit a simulated limit.
func newFixture(t *testing.T, names ...string) (*fixture, []profiles.Profile) {
	t.Helper()
	t.Setenv("FAKE_LIMIT_DIR", "limited")
	root := t.TempDir()
	var list []profiles.Profile
	for _, n := range names {
		dir := filepath.Join(root, n)
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		list = append(list, profiles.Profile{Name: n, Dir: dir})
	}
	f := &fixture{store: state.NewStore(filepath.Join(t.TempDir(), "state.json")), notice: &bytes.Buffer{}}
	cfg := config.Default()
	cfg.MinSwitchInterval = 0
	f.proxy = &Proxy{
		Launcher: launcher.Launcher{Claude: fakeClaude, Environ: os.Environ(), Stderr: io.Discard, StopGrace: time.Second},
		Store:    f.store,
		Profiles: list,
		Config:   func() (config.Config, error) { return cfg, nil },
		Prepare: func(p profiles.Profile) error {
			f.mu.Lock()
			f.prepared = append(f.prepared, p.Name)
			f.mu.Unlock()
			return nil
		},
		Now:     time.Now,
		History: func(e history.Entry) { f.switches = append(f.switches, e) },
		Notify:  func(text string) { f.notified = append(f.notified, text) },
		Notice:  f.notice,
	}
	return f, list
}

// run starts the proxy in start and returns the client and a channel with its exit code.
func (f *fixture) run(t *testing.T, start profiles.Profile) (*client, <-chan int) {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	f.proxy.Launcher.Stdin, f.proxy.Launcher.Stdout = inR, outW
	c := &client{t: t, in: inW, lines: make(chan map[string]any, 100)}
	go func() {
		defer close(c.lines)
		sc := bufio.NewScanner(outR)
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for sc.Scan() {
			var m map[string]any
			if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
				t.Errorf("output line is not JSON: %q", sc.Text())
				continue
			}
			c.lines <- m
		}
	}()
	done := make(chan int, 1)
	go func() {
		code, err := f.proxy.Run(context.Background(), start, sdkArgs)
		if err != nil {
			t.Errorf("Run: %v", err)
		}
		_ = outW.Close()
		done <- code
	}()
	return c, done
}

func configDirOf(t *testing.T, result map[string]any) (string, []string) {
	t.Helper()
	report, ok := result["result"].(map[string]any)
	if !ok {
		t.Fatalf("result carries no report: %v", result)
	}
	var args []string
	for _, a := range report["args"].([]any) {
		args = append(args, a.(string))
	}
	return filepath.Base(report["configDir"].(string)), args
}

const (
	initialize = `{"type":"control_request","request_id":"init-1","request":{"subtype":"initialize"}}`
	setModel   = `{"type":"control_request","request_id":"model-1","request":{"subtype":"set_model","model":"haiku"}}`
	interrupt  = `{"type":"control_request","request_id":"stop-1","request":{"subtype":"interrupt"}}`
	userTurn   = `{"type":"user","message":{"role":"user","content":"hi"},"parent_tool_use_id":null,"session_id":""}`
)

func TestLimitMovesTheConversationToTheNextProfile(t *testing.T) {
	f, list := newFixture(t, "a-limited", "b")
	c, done := f.run(t, list[0])

	c.send(initialize)
	if r := c.next("control_response"); r["response"].(map[string]any)["request_id"] != "init-1" {
		t.Fatalf("unexpected response %v", r)
	}
	c.send(setModel)
	c.next("control_response")
	c.send(interrupt)
	c.next("control_response")
	c.send(userTurn)
	if r := c.next("result"); r["is_error"] != true {
		t.Fatalf("the limited turn must reach the client as it is: %v", r)
	}

	c.send(userTurn)
	result := c.next("result")
	dir, args := configDirOf(t, result)
	if dir != "b" {
		t.Fatalf("second turn answered by %s, want b", dir)
	}
	if !slices.Contains(args, "--resume") || args[slices.Index(args, "--resume")+1] != "s-1" || slices.ContainsFunc(args, func(a string) bool { return strings.HasPrefix(a, "--session-id") }) {
		t.Errorf("restart must resume the conversation instead of naming a new one: %v", args)
	}
	_ = c.in.Close()
	c.drain()
	for _, id := range []string{"init-1", "model-1", "stop-1"} {
		if n := c.responses(id); n != 1 {
			t.Errorf("client got %d answers to %s, want 1", n, id)
		}
	}
	if code := <-done; code != 0 {
		t.Errorf("exit code %d", code)
	}

	if !slices.Equal(f.prepared, []string{"a-limited", "b"}) {
		t.Errorf("prepared %v", f.prepared)
	}
	st, err := f.store.Read()
	if err != nil {
		t.Fatal(err)
	}
	if !st.Profiles["a-limited"].CooldownUntil.After(time.Now()) {
		t.Error("the limited profile is not cooling down")
	}
	if u := st.Profiles["b"].FiveHour; u == nil || u.UsedPercent != 25 {
		t.Errorf("usage of b not recorded: %+v", u)
	}
	if len(f.switches) != 1 || f.switches[0].From != "a-limited" || f.switches[0].To != "b" || f.switches[0].How != history.Auto {
		t.Errorf("history %+v", f.switches)
	}
	if len(f.notified) != 1 || !strings.Contains(f.notified[0], "continues with b") {
		t.Errorf("notifications %q", f.notified)
	}
}

func TestRestartReplaysOnlyTheSettingRequests(t *testing.T) {
	var got []string
	for _, line := range []string{initialize, setModel, interrupt, userTurn} {
		if replayed[controlSubtype([]byte(line))] {
			got = append(got, requestID([]byte(line)))
		}
	}
	if !slices.Equal(got, []string{"init-1", "model-1"}) {
		t.Errorf("replayed %v", got)
	}
}

func TestLimitWithoutAnotherProfileKeepsClaudeRunning(t *testing.T) {
	f, list := newFixture(t, "a-limited")
	c, done := f.run(t, list[0])

	c.send(initialize)
	c.next("control_response")
	c.send(userTurn)
	c.next("result")
	c.send(setModel)
	if r := c.next("control_response"); r["response"].(map[string]any)["request_id"] != "model-1" {
		t.Fatalf("claude did not keep serving: %v", r)
	}
	_ = c.in.Close()
	c.drain()
	if code := <-done; code != 0 {
		t.Errorf("exit code %d", code)
	}
	if len(f.prepared) != 1 || len(f.switches) != 0 {
		t.Errorf("no switch expected: prepared %v, switches %v", f.prepared, f.switches)
	}
	if !strings.Contains(f.notice.String(), "no other profile can take over") {
		t.Errorf("notice %q", f.notice.String())
	}
}

func TestNormalTurnsStayInTheProfile(t *testing.T) {
	f, list := newFixture(t, "a", "b")
	c, done := f.run(t, list[0])

	c.send(initialize)
	c.next("control_response")
	for range 2 {
		c.send(userTurn)
		if dir, _ := configDirOf(t, c.next("result")); dir != "a" {
			t.Fatalf("turn answered by %s", dir)
		}
	}
	_ = c.in.Close()
	c.drain()
	if code := <-done; code != 0 {
		t.Errorf("exit code %d", code)
	}
	if len(f.prepared) != 1 {
		t.Errorf("prepared %v", f.prepared)
	}
}

func TestSpeaksSDK(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want bool
	}{
		{sdkArgs, true},
		{[]string{"--input-format=stream-json"}, true},
		{[]string{"-p", "hi", "--output-format", "stream-json"}, false},
		{[]string{"--version"}, false},
	} {
		if got := SpeaksSDK(tt.args); got != tt.want {
			t.Errorf("SpeaksSDK(%v) = %v", tt.args, got)
		}
	}
}

func TestResumeArgsDropsOneOffOptions(t *testing.T) {
	got := resumeArgs([]string{"--output-format", "stream-json", "--fork-session", "--resume=old", "--resume-session-at=u1", "--resume-drops-turn=true", "--model", "haiku"}, "s-9")
	want := []string{"--resume", "s-9", "--output-format", "stream-json", "--model", "haiku"}
	if !slices.Equal(got, want) {
		t.Errorf("resumeArgs = %v, want %v", got, want)
	}
}
