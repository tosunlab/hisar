//go:build darwin || linux

package commandline

import (
	"os"

	"golang.org/x/sys/unix"
)

func isWritableDirectory(path string) bool {
	fileInfo, err := os.Stat(path)
	return err == nil && fileInfo.IsDir() && unix.Access(path, unix.W_OK) == nil
}
