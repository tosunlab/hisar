// Package portable runs Hisar from an AppImage or a Flatpak. Their files are not where the rest of the system can run
// them (an AppImage is mounted at a new path each time, a Flatpak's are inside its sandbox), so at every start the app
// copies the browser host and hisar to ~/.local/share/hisar/bin, with a hisar-app launcher that starts the package again.
// The browser, the terminal and the login item use those.
package portable

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type Kind string

const (
	KindNone     Kind = ""
	KindAppImage Kind = "appimage"
	KindFlatpak  Kind = "flatpak"

	// LauncherName differs from hisar by more than case, so the two never collide on a file system that ignores case.
	LauncherName    = "hisar-app"
	browserHostName = "hisar-browser-host"
	commandLineName = "hisar"
)

// Environment is how this process runs: from a package the system installed, an AppImage or a Flatpak.
type Environment struct {
	Kind          Kind
	AppImagePath  string
	FlatpakID     string
	HomeDirectory string
	// hostConfigDirectory is the user's ~/.config as the rest of the system sees it; a Flatpak's own points elsewhere.
	hostConfigDirectory string
}

// Detect reads the variables the AppImage runtime and Flatpak set.
func Detect(getEnvironment func(string) string, homeDirectory string) Environment {
	environment := Environment{HomeDirectory: homeDirectory, hostConfigDirectory: filepath.Join(homeDirectory, ".config")}
	if configDirectory := getEnvironment("XDG_CONFIG_HOME"); filepath.IsAbs(configDirectory) {
		environment.hostConfigDirectory = configDirectory
	}
	switch {
	case getEnvironment("FLATPAK_ID") != "":
		environment.Kind, environment.FlatpakID = KindFlatpak, getEnvironment("FLATPAK_ID")
		environment.hostConfigDirectory = filepath.Join(homeDirectory, ".config")
		if configDirectory := getEnvironment("HOST_XDG_CONFIG_HOME"); filepath.IsAbs(configDirectory) {
			environment.hostConfigDirectory = configDirectory
		}
	case filepath.IsAbs(getEnvironment("APPIMAGE")):
		environment.Kind, environment.AppImagePath = KindAppImage, getEnvironment("APPIMAGE")
	}
	return environment
}

func (environment Environment) IsPortable() bool { return environment.Kind != KindNone }

// GetBinDirectory is where the helpers and the launcher are copied; the Flatpak may write there (--filesystem).
func (environment Environment) GetBinDirectory() string {
	return filepath.Join(environment.HomeDirectory, ".local", "share", "hisar", "bin")
}

func (environment Environment) GetLauncherPath() string {
	return filepath.Join(environment.GetBinDirectory(), LauncherName)
}

// GetHostConfigDirectory is where the login item goes, so the desktop session finds it.
func (environment Environment) GetHostConfigDirectory() string {
	return environment.hostConfigDirectory
}

func quoteShell(text string) string {
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}

// buildLauncher is a shell script that starts this package again, with its arguments (--hidden for a hidden launch).
func (environment Environment) buildLauncher() ([]byte, error) {
	switch environment.Kind {
	case KindAppImage:
		return []byte("#!/bin/sh\n# Written by Hisar: starts the AppImage it runs from.\nexec " + quoteShell(environment.AppImagePath) + " \"$@\"\n"), nil
	case KindFlatpak:
		return []byte("#!/bin/sh\n# Written by Hisar: starts its Flatpak.\nexec flatpak run " + quoteShell(environment.FlatpakID) + " \"$@\"\n"), nil
	}
	return nil, errors.New("portable: not an AppImage or a Flatpak")
}

// Paths are the copies the rest of the system runs.
type Paths struct {
	BrowserHost string
	CommandLine string
	Launcher    string
}

// Install copies the browser host and hisar from the package (bin and lib/hisar beside executableDirectory) and writes
// the launcher; each file is rewritten only when it changed, so a running copy is never replaced for nothing.
func Install(environment Environment, executableDirectory string) (Paths, error) {
	launcher, err := environment.buildLauncher()
	if err != nil {
		return Paths{}, err
	}
	directory := environment.GetBinDirectory()
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return Paths{}, err
	}
	paths := Paths{
		BrowserHost: filepath.Join(directory, browserHostName),
		CommandLine: filepath.Join(directory, commandLineName),
		Launcher:    filepath.Join(directory, LauncherName),
	}
	copies := []struct{ source, target string }{
		{filepath.Join(filepath.Dir(executableDirectory), "lib", "hisar", browserHostName), paths.BrowserHost},
		{filepath.Join(executableDirectory, commandLineName), paths.CommandLine},
	}
	var errs []error
	for _, copy := range copies {
		content, err := os.ReadFile(copy.source)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		errs = append(errs, writeExecutableIfChanged(copy.target, content))
	}
	errs = append(errs, writeExecutableIfChanged(paths.Launcher, launcher))
	return paths, errors.Join(errs...)
}

func writeExecutableIfChanged(path string, content []byte) error {
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, content) {
		return nil
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	// A new file renamed over the old one: a hisar that is running keeps its own, and nothing sees half a file.
	partialPath := path + ".partial"
	if err := os.WriteFile(partialPath, content, 0o755); err != nil {
		return fmt.Errorf("portable: writing %s: %w", path, err)
	}
	if err := os.Chmod(partialPath, 0o755); err != nil {
		os.Remove(partialPath)
		return err
	}
	if err := os.Rename(partialPath, path); err != nil {
		os.Remove(partialPath)
		return err
	}
	return nil
}
