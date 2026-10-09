//go:build !windows

package launcher

// saveTerminal restores nothing: on Unix claude resets the terminal itself when
// it is asked to terminate.
func saveTerminal() func() {
	return func() {}
}
