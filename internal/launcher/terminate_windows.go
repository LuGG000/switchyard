package launcher

import "os"

// terminate ends claude at once: Windows has no signal that asks a console
// process to exit cleanly without also reaching switchyard.
func terminate(p *os.Process) error {
	return p.Kill()
}
