//go:build windows

package linker

import (
	"fmt"
	"os"
	"os/exec"
)

// createLink makes a directory junction for directories, which needs no
// elevated rights. Files use a symlink when the account may create one and a
// hard link otherwise; a hard link stops sharing if a tool replaces the file,
// which `repair` detects as a conflict.
func createLink(src, dst string, isDir bool) error {
	if isDir {
		out, err := exec.Command("cmd", "/c", "mklink", "/J", dst, src).CombinedOutput()
		if err != nil {
			return fmt.Errorf("mklink /J: %w: %s", err, out)
		}
		return nil
	}
	if err := os.Symlink(src, dst); err == nil {
		return nil
	}
	return os.Link(src, dst)
}
