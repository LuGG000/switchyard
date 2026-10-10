// Package history keeps a short log of the profile switches, so one can see when and why
// switchyard moved from one profile to another. It is a JSON lines file next to state.json;
// it holds names and times only.
package history

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// How a switch came about.
const (
	// Auto is a switch switchyard decided alone.
	Auto = "auto"
	// Asked is a switch the user chose at the terminal question.
	Asked = "asked"
	// Button is a switch the user chose with the mod's buttons.
	Button = "button"
	// Handoff is a switch the user asked for with `handoff` or the mod.
	Handoff = "handoff"
)

const (
	// maxBytes is the size at which the file is cut back to keepEntries entries.
	maxBytes    = 128 << 10
	keepEntries = 200
)

// Entry is one switch.
type Entry struct {
	Time time.Time `json:"time"`
	From string    `json:"from"`
	To   string    `json:"to"`
	// Reason is why a switch was needed: rate_limit, threshold or manual.
	Reason string `json:"reason"`
	// How is Auto, Asked, Button or Handoff.
	How string `json:"how"`
	// Carry says whether the conversation went along.
	Carry bool `json:"carry"`
}

// Log is the history file.
type Log struct {
	path string
}

// New returns the log stored at path.
func New(path string) *Log {
	return &Log{path: path}
}

// Append adds an entry and cuts the file back when it has grown large. A log that cannot
// be written is not worth failing a switch over, so the error is only returned.
func (l *Log) Append(e Entry) error {
	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return fmt.Errorf("create history dir: %w", err)
	}
	line, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("encode history entry: %w", err)
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open history: %w", err)
	}
	_, werr := f.Write(append(line, '\n'))
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return fmt.Errorf("write history: %w", werr)
	}
	return l.trim()
}

// Last returns up to n of the newest entries, oldest first. Lines that are not valid
// entries are skipped. A missing file is an empty history.
func (l *Log) Last(n int) ([]Entry, error) {
	entries, err := l.read()
	if err != nil {
		return nil, err
	}
	if n > 0 && len(entries) > n {
		entries = entries[len(entries)-n:]
	}
	return entries, nil
}

func (l *Log) read() ([]Entry, error) {
	f, err := os.Open(l.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open history: %w", err)
	}
	defer func() { _ = f.Close() }()
	var entries []Entry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e Entry
		if json.Unmarshal(sc.Bytes(), &e) == nil && !e.Time.IsZero() {
			entries = append(entries, e)
		}
	}
	return entries, sc.Err()
}

func (l *Log) trim() error {
	info, err := os.Stat(l.path)
	if err != nil || info.Size() <= maxBytes {
		return err
	}
	entries, err := l.read()
	if err != nil {
		return err
	}
	if len(entries) > keepEntries {
		entries = entries[len(entries)-keepEntries:]
	}
	var buf bytes.Buffer
	for _, e := range entries {
		line, err := json.Marshal(e)
		if err != nil {
			return err
		}
		buf.Write(append(line, '\n'))
	}
	tmp, err := os.CreateTemp(filepath.Dir(l.path), filepath.Base(l.path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp history: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write history: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write history: %w", err)
	}
	return os.Rename(tmp.Name(), l.path)
}
