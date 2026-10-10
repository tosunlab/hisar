package commandline

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	commandName           = "hisar"
	bundleMarker          = "/Hisar.app/"
	loginShellTimeout     = 3 * time.Second
	userPathCacheLifetime = 30 * time.Second
	userPathMarker        = "__HISAR_PATH__"
)

const (
	InstallationThisApp  = "this-app"
	InstallationOtherApp = "other-app"
	InstallationOther    = "other"
)

var DefaultSystemDirectories = []string{"/opt/homebrew/bin", "/usr/local/bin"}

var errUnavailable = errors.New("the command-line tool is only bundled with the packaged Hisar app")

// Installation is a hisar found in one of the install directories; Target is where a symlink points.
type Installation struct {
	Path        string `json:"path"`
	Kind        string `json:"kind"`
	Target      string `json:"target"`
	IsSymlink   bool   `json:"isSymlink"`
	Description string `json:"description"`
}

type InstallStatus struct {
	IsAvailable              bool           `json:"isAvailable"`
	BundledPath              string         `json:"bundledPath"`
	InstalledPath            string         `json:"installedPath"`
	AllInstallation          []Installation `json:"allInstallation"`
	PathCommandPath          string         `json:"pathCommandPath"`
	PathCommandTarget        string         `json:"pathCommandTarget"`
	IsPathCommandFromThisApp bool           `json:"isPathCommandFromThisApp"`
	// PathHint is the shell command that puts the install directory on the PATH, when it is not.
	PathHint string `json:"pathHint"`
}

type InstallerConfig struct {
	BundledPath       string
	HomeDirectory     string
	SystemDirectories []string
	// GetUserPath returns the directories of the user's PATH; ReadUserPath reads it from the login shell.
	GetUserPath func() []string
	Shell       string
}

type Installer struct {
	config InstallerConfig
}

func CreateInstaller(config InstallerConfig) *Installer {
	if config.SystemDirectories == nil {
		config.SystemDirectories = DefaultSystemDirectories
	}
	if config.GetUserPath == nil {
		config.GetUserPath = func() []string { return filepath.SplitList(os.Getenv("PATH")) }
	}
	return &Installer{config: config}
}

func (installer *Installer) getUserDirectory() string {
	return filepath.Join(installer.config.HomeDirectory, ".local", "bin")
}

func (installer *Installer) getAllInstallDirectory() []string {
	return append(slices.Clone(installer.config.SystemDirectories), installer.getUserDirectory())
}

func (installer *Installer) isAvailable() bool {
	if installer.config.BundledPath == "" {
		return false
	}
	fileInfo, err := os.Stat(installer.config.BundledPath)
	return err == nil && fileInfo.Mode().IsRegular()
}

func resolveLinkTarget(linkPath string) (string, error) {
	target, err := os.Readlink(linkPath)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(linkPath), target)
	}
	return filepath.Clean(target), nil
}

func isSamePath(first, second string) bool {
	if first == second {
		return true
	}
	firstResolved, firstError := filepath.EvalSymlinks(first)
	secondResolved, secondError := filepath.EvalSymlinks(second)
	return firstError == nil && secondError == nil && firstResolved == secondResolved
}

func (installer *Installer) describe(path string) (Installation, bool) {
	fileInfo, err := os.Lstat(path)
	if err != nil {
		return Installation{}, false
	}
	installation := Installation{Path: path, Kind: InstallationOther, IsSymlink: fileInfo.Mode()&fs.ModeSymlink != 0}
	if !installation.IsSymlink {
		installation.Description = "a file that is not from Hisar"
		return installation, true
	}
	installation.Target, _ = resolveLinkTarget(path)
	switch {
	case installer.config.BundledPath != "" && isSamePath(installation.Target, installer.config.BundledPath):
		installation.Kind, installation.Description = InstallationThisApp, "this app's command-line tool"
	case strings.Contains(installation.Target, bundleMarker):
		installation.Kind, installation.Description = InstallationOtherApp, "a link to another copy of Hisar"
	default:
		installation.Description = "a link to " + installation.Target
	}
	return installation, true
}

func (installer *Installer) findPathCommand(allDirectory []string) string {
	for _, directory := range allDirectory {
		if directory == "" {
			continue
		}
		path := filepath.Join(directory, commandName)
		if fileInfo, err := os.Stat(path); err == nil && fileInfo.Mode().IsRegular() && fileInfo.Mode().Perm()&0o111 != 0 {
			return path
		}
	}
	return ""
}

func (installer *Installer) buildPathHint(directory string, allUserDirectory []string) string {
	if slices.ContainsFunc(allUserDirectory, func(candidate string) bool { return filepath.Clean(candidate) == directory }) {
		return ""
	}
	shownDirectory := directory
	if relative, err := filepath.Rel(installer.config.HomeDirectory, directory); err == nil && !strings.HasPrefix(relative, "..") {
		shownDirectory = "$HOME/" + relative
	}
	profile := "~/.zshrc"
	if filepath.Base(installer.config.Shell) == "bash" {
		profile = "~/.bash_profile"
	}
	return fmt.Sprintf(`echo 'export PATH="%s:$PATH"' >> %s`, shownDirectory, profile)
}

func (installer *Installer) GetStatus() InstallStatus {
	status := InstallStatus{IsAvailable: installer.isAvailable(), BundledPath: installer.config.BundledPath, AllInstallation: []Installation{}}
	for _, directory := range installer.getAllInstallDirectory() {
		installation, isFound := installer.describe(filepath.Join(directory, commandName))
		if !isFound {
			continue
		}
		status.AllInstallation = append(status.AllInstallation, installation)
		if installation.Kind == InstallationThisApp && status.InstalledPath == "" {
			status.InstalledPath = installation.Path
		}
	}
	allUserDirectory := installer.config.GetUserPath()
	status.PathCommandPath = installer.findPathCommand(allUserDirectory)
	if status.PathCommandPath != "" {
		status.PathCommandTarget, _ = filepath.EvalSymlinks(status.PathCommandPath)
		status.IsPathCommandFromThisApp = status.IsAvailable && isSamePath(status.PathCommandPath, installer.config.BundledPath)
	}
	if status.InstalledPath != "" {
		status.PathHint = installer.buildPathHint(filepath.Dir(status.InstalledPath), allUserDirectory)
	}
	return status
}

func (installer *Installer) chooseDirectory() (string, error) {
	for _, directory := range installer.config.SystemDirectories {
		if isWritableDirectory(directory) {
			return directory, nil
		}
	}
	directory := installer.getUserDirectory()
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", fmt.Errorf("could not create %s: %w", directory, err)
	}
	return directory, nil
}

// Install links hisar to the bundled binary; it replaces only a link into a Hisar.app.
func (installer *Installer) Install() (InstallStatus, error) {
	if !installer.isAvailable() {
		return installer.GetStatus(), errUnavailable
	}
	directory, err := installer.chooseDirectory()
	if err != nil {
		return installer.GetStatus(), err
	}
	path := filepath.Join(directory, commandName)
	if existing, isFound := installer.describe(path); isFound {
		switch existing.Kind {
		case InstallationThisApp:
			return installer.GetStatus(), nil
		case InstallationOther:
			return installer.GetStatus(), fmt.Errorf("%s is already %s; Hisar leaves it alone. Remove it yourself to install Hisar's", path, existing.Description)
		}
		if err := os.Remove(path); err != nil {
			return installer.GetStatus(), fmt.Errorf("could not replace %s: %w", path, err)
		}
	}
	if err := os.Symlink(installer.config.BundledPath, path); err != nil {
		return installer.GetStatus(), fmt.Errorf("could not create %s: %w", path, err)
	}
	return installer.GetStatus(), nil
}

// Uninstall removes only hisar symlinks that point into a Hisar.app.
func (installer *Installer) Uninstall() (InstallStatus, error) {
	var allError []error
	for _, directory := range installer.getAllInstallDirectory() {
		installation, isFound := installer.describe(filepath.Join(directory, commandName))
		if !isFound || installation.Kind == InstallationOther {
			continue
		}
		if err := os.Remove(installation.Path); err != nil {
			allError = append(allError, fmt.Errorf("could not remove %s: %w", installation.Path, err))
		}
	}
	return installer.GetStatus(), errors.Join(allError...)
}

// UserPathReader reads the PATH of the user's login shell, which apps started from Finder do not get.
type UserPathReader struct {
	Shell       string
	mutex       sync.Mutex
	cachedPath  []string
	cachedUntil time.Time
}

func (reader *UserPathReader) Read() []string {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()
	if time.Now().Before(reader.cachedUntil) {
		return reader.cachedPath
	}
	allDirectory, err := readLoginShellPath(reader.Shell)
	if err != nil || len(allDirectory) == 0 {
		allDirectory = readSystemPath()
	}
	reader.cachedPath, reader.cachedUntil = allDirectory, time.Now().Add(userPathCacheLifetime)
	return allDirectory
}

func readLoginShellPath(shell string) ([]string, error) {
	if shell == "" {
		return nil, errors.New("no login shell")
	}
	ctx, cancel := context.WithTimeout(context.Background(), loginShellTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, shell, "-l", "-i", "-c", `printf '\n`+userPathMarker+`%s\n' "$PATH"`)
	command.Stdin = nil
	output, err := command.Output()
	if err != nil && len(output) == 0 {
		return nil, err
	}
	return parseUserPathOutput(string(output))
}

func parseUserPathOutput(output string) ([]string, error) {
	for line := range strings.SplitSeq(output, "\n") {
		if path, isFound := strings.CutPrefix(line, userPathMarker); isFound {
			return filepath.SplitList(strings.TrimSpace(path)), nil
		}
	}
	return nil, errors.New("the login shell did not print its PATH")
}

func readSystemPath() []string {
	var allDirectory []string
	allPathFile := []string{"/etc/paths"}
	if allExtraFile, err := filepath.Glob("/etc/paths.d/*"); err == nil {
		allPathFile = append(allPathFile, allExtraFile...)
	}
	for _, pathFile := range allPathFile {
		file, err := os.Open(pathFile)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			if line := strings.TrimSpace(scanner.Text()); line != "" && !strings.HasPrefix(line, "#") {
				allDirectory = append(allDirectory, line)
			}
		}
		file.Close()
	}
	for _, directory := range filepath.SplitList(os.Getenv("PATH")) {
		if !slices.Contains(allDirectory, directory) {
			allDirectory = append(allDirectory, directory)
		}
	}
	return allDirectory
}
