// Package sdkproxy runs claude for a client that talks the stream-json protocol of the Agent SDK,
// such as the Claude Code extension for VS Code with switchyard as its claudeCode.claudeProcessWrapper.
//
// Messages pass through unchanged in both directions. When a turn ends at the profile's limit, the
// proxy restarts claude in the next profile with the same conversation (--resume). The client keeps
// its connection and does not see the restart: the new claude gets the initialize request and the
// settings the client sent before, and its answers to them are dropped, because the client already
// has answers to those requests.
package sdkproxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/LuGG000/switchyard/internal/breaker"
	"github.com/LuGG000/switchyard/internal/config"
	"github.com/LuGG000/switchyard/internal/detector"
	"github.com/LuGG000/switchyard/internal/headless"
	"github.com/LuGG000/switchyard/internal/history"
	"github.com/LuGG000/switchyard/internal/hooks"
	"github.com/LuGG000/switchyard/internal/launcher"
	"github.com/LuGG000/switchyard/internal/profiles"
	"github.com/LuGG000/switchyard/internal/selector"
	"github.com/LuGG000/switchyard/internal/state"
)

// replayed lists the control requests a restarted claude needs again: they set up the session or
// change one of its settings, and sending them twice has no other effect.
var replayed = map[string]bool{
	"initialize":                       true,
	"set_permission_mode":              true,
	"set_model":                        true,
	"set_max_thinking_tokens":          true,
	"mcp_set_servers":                  true,
	"mcp_toggle":                       true,
	"set_mcp_permission_mode_override": true,
}

// SpeaksSDK reports whether the claude arguments ask for the stream-json input of an SDK client.
func SpeaksSDK(args []string) bool {
	for i, a := range args {
		if a == "--input-format=stream-json" || (a == "--input-format" && i+1 < len(args) && args[i+1] == "stream-json") {
			return true
		}
	}
	return false
}

// Proxy runs claude between an SDK client and a set of profiles.
type Proxy struct {
	// Launcher starts claude; its Stdin, Stdout and Stderr are the client's.
	Launcher launcher.Launcher
	Store    *state.Store
	// Profiles are the profiles a conversation may move to.
	Profiles []profiles.Profile
	// Config returns the settings; it is read again at every limit.
	Config func() (config.Config, error)
	// Prepare records p as in use before claude starts in it.
	Prepare func(p profiles.Profile) error
	// Now returns the current time.
	Now func() time.Time
	// History, if set, receives an entry for every switch.
	History func(history.Entry)
	// Notify, if set, shows a desktop notification; the client shows nothing of the proxy.
	Notify func(text string)
	// Notice receives a line for every switch and for a limit that could not be switched away from.
	Notice io.Writer
}

// Run serves the client with claude in start until the client closes its input or claude exits
// by itself, and returns claude's exit code.
func (p *Proxy) Run(ctx context.Context, start profiles.Profile, args []string) (int, error) {
	in := readLines(p.Launcher.Stdin)
	limited := make(chan chan struct{})
	var replay [][]byte
	current, runArgs := start, args
	for {
		if err := p.Prepare(current); err != nil {
			return 0, err
		}
		c, err := p.start(ctx, current, runArgs, limited)
		if err != nil {
			return 0, err
		}
		for _, line := range replay {
			c.out.drop(requestID(line))
			_, _ = c.stdin.Write(line)
		}
		next, code, err := p.serve(c, current, in, limited, &replay)
		if next == nil {
			return code, err
		}
		current, runArgs = *next, resumeArgs(args, c.out.sessionID())
	}
}

// serve passes the client's messages to c until claude ends or a limit moves the conversation to
// another profile, which it returns.
func (p *Proxy) serve(c *child, current profiles.Profile, in <-chan []byte, limited <-chan chan struct{}, replay *[][]byte) (*profiles.Profile, int, error) {
	for {
		select {
		case line, ok := <-in:
			if !ok {
				_ = c.stdin.Close()
				e := c.wait(limited)
				return nil, e.code, e.err
			}
			if replayed[controlSubtype(line)] {
				*replay = append(*replay, line)
			}
			// A failed write means claude is gone; its exit arrives next.
			_, _ = c.stdin.Write(line)
		case ack := <-limited:
			next, ok := p.choose(current)
			close(ack)
			if !ok {
				continue
			}
			close(c.stop)
			e := c.wait(limited)
			_ = c.stdin.Close()
			if e.err != nil {
				return nil, e.code, e.err
			}
			return &next, 0, nil
		case e := <-c.exited:
			_ = c.stdin.Close()
			return nil, e.code, e.err
		}
	}
}

// choose puts current into cooldown and returns the profile to continue in, if a switch is
// possible and allowed. Clients cannot ask, so the switch is automatic whatever the mode says.
func (p *Proxy) choose(current profiles.Profile) (profiles.Profile, bool) {
	cfg, err := p.Config()
	if err != nil {
		_, _ = fmt.Fprintf(p.Notice, "switchyard: %v\n", err)
		return current, false
	}
	say := func(text string) {
		_, _ = fmt.Fprintln(p.Notice, "switchyard: "+text)
		if cfg.Notify && p.Notify != nil {
			p.Notify(text)
		}
	}
	now := p.Now()
	h := hooks.Handler{Store: p.Store, Profile: current.Name, Now: p.Now}
	if err := h.MarkLimited(); err != nil {
		say(err.Error())
		return current, false
	}
	next, err := selector.NextProfile(p.Store, p.Profiles, cfg.Strategy, selector.ThresholdsFromConfig(cfg), current.Name, now)
	if err != nil {
		say(fmt.Sprintf("%s reached its limit and no other profile can take over: %v", current.Name, err))
		return current, false
	}
	if next.Name == current.Name {
		return current, false
	}
	if err := breaker.Allowed(p.Store, breaker.FromConfig(cfg), now); err != nil {
		say(fmt.Sprintf("%s reached its limit; automatic switching is paused: %v", current.Name, err))
		return current, false
	}
	if err := breaker.Record(p.Store, now); err != nil {
		say(err.Error())
		return current, false
	}
	if p.History != nil {
		p.History(history.Entry{Time: now.UTC(), From: current.Name, To: next.Name, Reason: state.ReasonRateLimit, How: history.Auto, Carry: true})
	}
	say(fmt.Sprintf("%s reached its limit; the conversation continues with %s. Send your message again.", current.Name, next.Name))
	return next, true
}

// start runs claude in prof with args; its output goes to the client through an output that
// reports limited turns on limited.
func (p *Proxy) start(ctx context.Context, prof profiles.Profile, args []string, limited chan<- chan struct{}) (*child, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("create pipe for claude: %w", err)
	}
	out := &output{dst: p.Launcher.Stdout, turn: &detector.Detector{}, dropped: map[string]bool{}}
	out.endTurn = func(result detector.Result, failed bool) {
		_ = headless.RecordUsage(p.Store, prof.Name, result, p.Now())
		if failed && result.Limited {
			ack := make(chan struct{})
			limited <- ack
			<-ack
		}
	}
	c := &child{stdin: w, out: out, stop: make(chan struct{}), exited: make(chan exit, 1)}
	l := p.Launcher
	l.Stdin, l.Stdout, l.Stop = r, out, c.stop
	go func() {
		code, err := l.Run(ctx, prof, args)
		_ = r.Close()
		out.flush()
		c.exited <- exit{code: code, err: err}
	}()
	return c, nil
}

// child is one running claude.
type child struct {
	stdin  *os.File
	out    *output
	stop   chan struct{}
	exited chan exit
}

type exit struct {
	code int
	err  error
}

// wait returns the exit of c. A limit reported meanwhile is let through: claude is ending anyway.
func (c *child) wait(limited <-chan chan struct{}) exit {
	for {
		select {
		case ack := <-limited:
			close(ack)
		case e := <-c.exited:
			return e
		}
	}
}

// output passes claude's stdout on to the client line by line and watches every turn.
type output struct {
	dst io.Writer
	// endTurn is called at the result of every turn, before the result reaches the client.
	endTurn func(result detector.Result, failed bool)
	partial []byte
	turn    *detector.Detector

	mu      sync.Mutex
	session string
	// dropped holds the IDs of replayed requests whose answers the client already has.
	dropped map[string]bool
}

func (o *output) Write(b []byte) (int, error) {
	n := len(b)
	for len(b) > 0 {
		i := bytes.IndexByte(b, '\n')
		if i < 0 {
			o.partial = append(o.partial, b...)
			break
		}
		line := append(o.partial, b[:i+1]...)
		o.partial = nil
		if err := o.line(line); err != nil {
			return n, err
		}
		b = b[i+1:]
	}
	return n, nil
}

func (o *output) line(line []byte) error {
	var m struct {
		Type      string `json:"type"`
		SessionID string `json:"session_id"`
		IsError   bool   `json:"is_error"`
		Response  *struct {
			RequestID string `json:"request_id"`
		} `json:"response"`
	}
	if json.Unmarshal(line, &m) == nil {
		if m.SessionID != "" {
			o.mu.Lock()
			o.session = m.SessionID
			o.mu.Unlock()
		}
		switch {
		case m.Type == "control_response" && m.Response != nil && o.isDropped(m.Response.RequestID):
			return nil
		case m.Type == "result":
			_, _ = o.turn.Write(line)
			o.endTurn(o.turn.Result(), m.IsError)
			o.turn = &detector.Detector{}
		default:
			_, _ = o.turn.Write(line)
		}
	}
	_, err := o.dst.Write(line)
	return err
}

// flush passes on a last line that claude ended without a newline. It runs after claude's output
// is closed, so nothing writes concurrently.
func (o *output) flush() {
	if len(o.partial) > 0 {
		_, _ = o.dst.Write(o.partial)
		o.partial = nil
	}
}

func (o *output) drop(id string) {
	if id == "" {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.dropped[id] = true
}

func (o *output) isDropped(id string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.dropped[id]
}

func (o *output) sessionID() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.session
}

// controlRequest is the part of a client's control request the proxy reads.
type controlRequest struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	Request   struct {
		Subtype string `json:"subtype"`
	} `json:"request"`
}

// controlSubtype returns the subtype of a control request, or "" for any other message.
func controlSubtype(line []byte) string {
	var r controlRequest
	if json.Unmarshal(line, &r) != nil || r.Type != "control_request" {
		return ""
	}
	return r.Request.Subtype
}

func requestID(line []byte) string {
	var r controlRequest
	_ = json.Unmarshal(line, &r)
	return r.RequestID
}

// resumeArgs continues the conversation in a restarted claude. Options that branch a resumed
// conversation off or cut it short applied to the first start only.
func resumeArgs(args []string, sessionID string) []string {
	kept := make([]string, 0, len(args))
	for _, a := range args {
		if a == "--fork-session" || strings.HasPrefix(a, "--resume-session-at") || strings.HasPrefix(a, "--resume-drops-turn") {
			continue
		}
		kept = append(kept, a)
	}
	return launcher.ResumeArgs(kept, sessionID)
}

// readLines delivers the client's messages one line at a time and closes the channel at the end
// of the input. A last line without a newline gets one.
func readLines(r io.Reader) <-chan []byte {
	lines := make(chan []byte)
	go func() {
		defer close(lines)
		if r == nil {
			return
		}
		br := bufio.NewReader(r)
		for {
			line, err := br.ReadBytes('\n')
			if len(line) > 0 {
				if line[len(line)-1] != '\n' {
					line = append(line, '\n')
				}
				lines <- line
			}
			if err != nil {
				return
			}
		}
	}()
	return lines
}
