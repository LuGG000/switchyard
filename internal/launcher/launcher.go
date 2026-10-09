// Package launcher runs claude for one profile with a scrubbed environment.
package launcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"time"

	"github.com/LuGG000/switchyard/internal/claudeenv"
	"github.com/LuGG000/switchyard/internal/profiles"
)

// Launcher starts the claude executable.
type Launcher struct {
	// Claude is the claude executable to run.
	Claude string
	// Environ is the environment before scrubbing.
	Environ []string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	// Stop, when closed, ends the running claude: it is asked to terminate and
	// killed if it is still alive after StopGrace. Run then returns its exit code.
	Stop <-chan struct{}
	// StopGrace is how long claude gets to exit after being asked to.
	StopGrace time.Duration
}

// Run starts claude with args for profile p and waits for it to exit. It
// returns claude's exit code; err is set only if claude could not be run.
//
// Interrupts are left to claude: in a terminal Ctrl+C reaches the whole
// foreground process group, and switchyard must outlive claude to report its
// exit code.
func (l *Launcher) Run(ctx context.Context, p profiles.Profile, args []string) (int, error) {
	cmd := exec.Command(l.Claude, args...)
	cmd.Env = claudeenv.ForProfile(l.Environ, p.Dir)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = l.Stdin, l.Stdout, l.Stderr

	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start claude: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	for {
		select {
		case err := <-done:
			return exitCode(err)
		case <-interrupts:
			// Swallow: claude received the same signal.
		case <-l.Stop:
			return exitCode(l.stop(cmd.Process, done))
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			return exitCode(<-done)
		}
	}
}

// stop asks the process to terminate, kills it after StopGrace and returns the
// result of its Wait.
func (l *Launcher) stop(p *os.Process, done <-chan error) error {
	_ = terminate(p)
	select {
	case err := <-done:
		return err
	case <-time.After(l.StopGrace):
		_ = p.Kill()
		return <-done
	}
}

func exitCode(err error) (int, error) {
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return 0, fmt.Errorf("wait for claude: %w", err)
}

// ResumeArgs returns args continuing a conversation: the session with the
// given ID, or the most recent one of the directory if the ID is empty. Any
// resume option in args is replaced. An option without a value, such as a bare
// --resume, would swallow a following prompt; switchyard always resumes an
// explicit session or none.
func ResumeArgs(args []string, sessionID string) []string {
	kept := make([]string, 0, len(args)+2)
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--continue" || a == "-c":
		case a == "--resume" || a == "-r" || a == "--session-id":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
			}
		case strings.HasPrefix(a, "--resume=") || strings.HasPrefix(a, "--session-id="):
		default:
			kept = append(kept, a)
		}
	}
	if sessionID == "" {
		return append([]string{"--continue"}, kept...)
	}
	return append([]string{"--resume", sessionID}, kept...)
}
