//go:build linux

package background

import (
	"os"
	"path/filepath"
)

func CreateLoginItem(homeDirectory string) LoginItem {
	return CreateXDGAutostart(homeDirectory, getExecutablePath())
}

// getExecutablePath prefers the AppImage file, since the running binary lives in its temporary mount.
func getExecutablePath() string {
	if appImagePath := os.Getenv("APPIMAGE"); filepath.IsAbs(appImagePath) {
		return appImagePath
	}
	executablePath, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolvedPath, err := filepath.EvalSymlinks(executablePath); err == nil {
		return resolvedPath
	}
	return executablePath
}
