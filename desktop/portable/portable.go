// Package portable runs Hisar from an AppImage or a Flatpak. Their files are not where the rest of the system can run
// them (an AppImage is mounted at a new path each time, a Flatpak's are inside its sandbox), so at every start the app
// copies the browser host and hisar to ~/.local/share/hisar/bin, with a hisar-app launcher that starts the package again,
// and the omp plugin to ~/.local/share/hisar/omp-plugin. The browser, the terminal, the login item and omp use those.
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
	ompPluginName   = "omp-plugin"
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

// Paths are the copies the rest of the system runs. OmpPlugin is empty for a package without the omp plugin.
type Paths struct {
	BrowserHost string
	CommandLine string
	Launcher    string
	OmpPlugin   string
}

// Install copies the browser host, hisar and the omp plugin from the package (bin, lib/hisar and share/hisar beside
// executableDirectory) and writes the launcher; each is rewritten only when it changed, so a running copy is never
// replaced for nothing. The plugin goes beside bin, so its bin/run-hisar finds the copied hisar.
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
	packageDirectory := filepath.Dir(executableDirectory)
	copies := []struct{ source, target string }{
		{filepath.Join(packageDirectory, "lib", "hisar", browserHostName), paths.BrowserHost},
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
	if source := filepath.Join(packageDirectory, "share", "hisar", ompPluginName); isDirectory(source) {
		target := filepath.Join(filepath.Dir(directory), ompPluginName)
		if err := copyTreeIfChanged(source, target); err != nil {
			errs = append(errs, err)
		} else {
			paths.OmpPlugin = target
		}
	}
	return paths, errors.Join(errs...)
}

func isDirectory(path string) bool {
	fileInfo, err := os.Stat(path)
	return err == nil && fileInfo.IsDir()
}

// copyTreeIfChanged makes target a copy of the source folder, files and modes; an unchanged copy is left alone. A new
// copy is written beside it and swapped in, so omp, which links to target, never sees half a plugin.
func copyTreeIfChanged(source, target string) error {
	if isSameTree(source, target) {
		return nil
	}
	partial, old := target+".partial", target+".old"
	os.RemoveAll(partial)
	os.RemoveAll(old)
	if err := os.CopyFS(partial, os.DirFS(source)); err != nil {
		os.RemoveAll(partial)
		return fmt.Errorf("portable: copying the omp plugin: %w", err)
	}
	if err := os.Rename(target, old); err != nil && !errors.Is(err, fs.ErrNotExist) {
		os.RemoveAll(partial)
		return err
	}
	if err := os.Rename(partial, target); err != nil {
		os.Rename(old, target)
		os.RemoveAll(partial)
		return err
	}
	return os.RemoveAll(old)
}

// isSameTree reports whether target has exactly source's files, with the same contents and execute bits (os.CopyFS
// keeps those and leaves the rest to the umask).
func isSameTree(source, target string) bool {
	listFiles := func(root string) (map[string]bool, error) {
		isExecutableByPath := map[string]bool{}
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			fileInfo, err := entry.Info()
			if err != nil {
				return err
			}
			relativePath, _ := filepath.Rel(root, path)
			isExecutableByPath[relativePath] = fileInfo.Mode()&0o111 != 0
			return nil
		})
		return isExecutableByPath, err
	}
	sourceFiles, sourceErr := listFiles(source)
	targetFiles, targetErr := listFiles(target)
	if sourceErr != nil || targetErr != nil || len(sourceFiles) != len(targetFiles) {
		return false
	}
	for relativePath, isExecutable := range sourceFiles {
		if isTargetExecutable, isFound := targetFiles[relativePath]; !isFound || isTargetExecutable != isExecutable {
			return false
		}
		sourceContent, sourceErr := os.ReadFile(filepath.Join(source, relativePath))
		targetContent, targetErr := os.ReadFile(filepath.Join(target, relativePath))
		if sourceErr != nil || targetErr != nil || !bytes.Equal(sourceContent, targetContent) {
			return false
		}
	}
	return true
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
