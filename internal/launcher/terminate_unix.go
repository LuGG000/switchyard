//go:build !windows

package launcher

import (
	"os"
	"syscall"
)

// terminate asks claude to exit; the caller kills it if it does not.
func terminate(p *os.Process) error {
	return p.Signal(syscall.SIGTERM)
}
