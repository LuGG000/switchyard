// Package detector recognizes a subscription rate limit in the output of a
// headless claude run (`claude -p`).
//
// The exact output at a real limit is not observed yet (spike #6). The detector
// therefore looks at every signal known so far: the status of the
// `rate_limit_event` and the `api_retry` error in `stream-json`, an error
// `result`, and, as a last resort, the limit message as plain text. Callers
// must combine the result with a non-zero exit code.
package detector

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"sync"
	"time"
)

// maxLine bounds how much of one output line is kept for inspection.
const maxLine = 1 << 20

// limitText matches the limit message claude prints in plain text.
var limitText = regexp.MustCompile(`(?i)(hit your .{0,40}limit|(usage|session|weekly|rate) limit (reached|hit))`)

// Window is the last reported consumption of one rate limit window.
type Window struct {
	// UsedPercent is the consumed share of the window, 0 to 100.
	UsedPercent float64
	// ResetsAt is when the window resets.
	ResetsAt time.Time
}

// Result is what a Detector has seen so far.
type Result struct {
	// Limited is true if the output contained a rate limit signal.
	Limited bool
	// SessionID is the conversation of the run, empty if the output has none.
	SessionID string
	// FiveHour and SevenDay are the last reported windows, nil if none was reported.
	FiveHour, SevenDay *Window
}

// Detector inspects the output written to it line by line. It is safe for
// concurrent use, so stdout and stderr can share one.
type Detector struct {
	mu       sync.Mutex
	line     []byte
	skipping bool
	result   Result
}

// Write implements io.Writer. It never fails.
func (d *Detector) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	rest := p
	for len(rest) > 0 {
		i := bytes.IndexByte(rest, '\n')
		if i < 0 {
			d.buffer(rest)
			break
		}
		d.buffer(rest[:i])
		d.endLine()
		rest = rest[i+1:]
	}
	return len(p), nil
}

// Result returns what has been seen so far, including an unterminated last line.
func (d *Detector) Result() Result {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.endLine()
	return d.result
}

func (d *Detector) buffer(b []byte) {
	if d.skipping {
		return
	}
	if len(d.line)+len(b) > maxLine {
		d.line, d.skipping = nil, true
		return
	}
	d.line = append(d.line, b...)
}

func (d *Detector) endLine() {
	line := bytes.TrimSpace(d.line)
	d.line, d.skipping = d.line[:0], false
	if len(line) > 0 {
		d.inspect(line)
	}
}

type event struct {
	Type          string `json:"type"`
	Subtype       string `json:"subtype"`
	SessionID     string `json:"session_id"`
	Error         string `json:"error"`
	IsError       bool   `json:"is_error"`
	Result        string `json:"result"`
	RateLimitInfo *struct {
		Status         string `json:"status"`
		UnifiedWindows map[string]struct {
			Utilization float64 `json:"utilization"`
			ResetsAt    int64   `json:"resetsAt"`
		} `json:"unifiedWindows"`
	} `json:"rate_limit_info"`
}

func (d *Detector) inspect(line []byte) {
	var ev event
	if line[0] != '{' || json.Unmarshal(line, &ev) != nil {
		d.matchText(string(line))
		return
	}
	if ev.SessionID != "" {
		d.result.SessionID = ev.SessionID
	}
	switch {
	case ev.Type == "rate_limit_event" && ev.RateLimitInfo != nil:
		d.recordWindows(ev)
		if status := ev.RateLimitInfo.Status; status != "" && !strings.HasPrefix(status, "allowed") {
			d.result.Limited = true
		}
	case ev.Subtype == "api_retry" && ev.Error == "rate_limit":
		d.result.Limited = true
	case ev.Type == "result" && ev.IsError:
		d.matchText(ev.Result)
	}
}

func (d *Detector) recordWindows(ev event) {
	for name, w := range ev.RateLimitInfo.UnifiedWindows {
		window := &Window{UsedPercent: w.Utilization * 100, ResetsAt: time.Unix(w.ResetsAt, 0).UTC()}
		switch name {
		case "five_hour":
			d.result.FiveHour = window
		case "seven_day":
			d.result.SevenDay = window
		}
	}
}

func (d *Detector) matchText(s string) {
	if limitText.MatchString(s) {
		d.result.Limited = true
	}
}
