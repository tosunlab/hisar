//go:build linux

package background

import (
	"os"
	"path/filepath"

	"github.com/tryhisar/hisar/desktop/portable"
)

// NewLoginItem starts an AppImage or Flatpak Hisar through the launcher it writes (portable), since neither has a
// fixed executable the session can run, and puts a Flatpak's entry in the user's own ~/.config/autostart.
func NewLoginItem(homeDirectory string) LoginItem {
	if environment := portable.Detect(os.Getenv, homeDirectory); environment.IsPortable() {
		return &XDGAutostart{ConfigDirectory: environment.GetHostConfigDirectory(), ExecutablePath: environment.GetLauncherPath()}
	}
	return NewXDGAutostart(homeDirectory, getExecutablePath())
}

func getExecutablePath() string {
	executablePath, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolvedPath, err := filepath.EvalSymlinks(executablePath); err == nil {
		return resolvedPath
	}
	return executablePath
}
