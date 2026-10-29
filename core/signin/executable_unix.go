//go:build unix

package signin

import (
	"os"
	"path/filepath"
	"syscall"
)

// isSystemExecutable reports a regular file that root owns and only root can write, in directories root owns and only
// root can write, up to /: a file the user's programs could not have put there or changed.
func isSystemExecutable(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil {
			return false
		}
		status, isStatus := info.Sys().(*syscall.Stat_t)
		if !isStatus || status.Uid != 0 || info.Mode().Perm()&0o022 != 0 {
			return false
		}
		if current == path && !info.Mode().IsRegular() {
			return false
		}
		if current == "/" {
			return true
		}
	}
}
