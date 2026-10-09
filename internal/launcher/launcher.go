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
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			return exitCode(<-done)
		}
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
