package launcher

import (
	"os"

	"golang.org/x/sys/windows"
)

// saveTerminal remembers the console modes and returns a function that restores
// them. claude switches the console to raw input and virtual terminal output; a
// claude that is killed cannot undo that, which leaves the next prompt without
// line input and echo.
func saveTerminal() func() {
	var restores []func()
	for _, f := range []*os.File{os.Stdin, os.Stdout} {
		handle := windows.Handle(f.Fd())
		var mode uint32
		if windows.GetConsoleMode(handle, &mode) == nil {
			restores = append(restores, func() { _ = windows.SetConsoleMode(handle, mode) })
		}
	}
	return func() {
		for _, restore := range restores {
			restore()
		}
	}
}
