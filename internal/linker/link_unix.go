//go:build !windows

package linker

import "os"

func createLink(src, dst string, _ bool) error {
	return os.Symlink(src, dst)
}
