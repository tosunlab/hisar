//go:build linux

package updater

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"

	"github.com/tryhisar/hisar/desktop/portable"
)

// The .deb and .rpm install the app here and name their package hisar (build/linux/nfpm/nfpm.yaml).
const (
	packagedAppPath = "/usr/bin/Hisar"
	dpkgFileList    = "/var/lib/dpkg/info/hisar.list"
)

// DetectInstallation finds the package this copy came from: an AppImage or a Flatpak (desktop/portable), or the .deb
// or .rpm when the system's package database owns /usr/bin/Hisar. quit ends the app after an AppImage update.
func DetectInstallation(quit func()) Installation {
	homeDirectory, _ := os.UserHomeDir()
	installation := Installation{
		Architecture:      runtime.GOARCH,
		DownloadDirectory: filepath.Join(homeDirectory, ".local", "share", "hisar", "updates"),
		Open:              openWithDesktop,
		Relaunch:          relaunchAfterExit,
		Quit:              quit,
	}
	if homeDirectory == "" {
		return installation
	}
	environment := portable.Detect(os.Getenv, homeDirectory)
	switch environment.Kind {
	case portable.KindAppImage:
		installation.Kind, installation.AppImagePath = InstallAppImage, environment.AppImagePath
	case portable.KindFlatpak:
		installation.Kind = InstallFlatpak
	default:
		if executablePath, err := os.Executable(); err != nil || executablePath != packagedAppPath {
			break
		}
		if _, err := os.Stat(dpkgFileList); err == nil {
			installation.Kind = InstallDeb
		} else if exec.Command("rpm", "-q", "--quiet", "hisar").Run() == nil {
			installation.Kind = InstallRPM
		}
	}
	return installation
}

// openWithDesktop opens a downloaded package as a double-click in the file manager does: in the software installer
// the desktop has for it (inside a Flatpak, through the desktop portal).
func openWithDesktop(path string) error {
	command := exec.Command("xdg-open", path)
	if err := command.Start(); err != nil {
		return err
	}
	go command.Wait()
	return nil
}

// relaunchAfterExit starts the AppImage again once this process has exited, so the new copy does not find this one
// still holding the single-instance lock. The waiting shell runs in its own session, outliving this process.
func relaunchAfterExit(appImagePath string) error {
	script := `while kill -0 "$1" 2>/dev/null; do sleep 0.2; done; exec "$2" >/dev/null 2>&1`
	command := exec.Command("/bin/sh", "-c", script, "sh", strconv.Itoa(os.Getpid()), appImagePath)
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}
