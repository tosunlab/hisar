package agent

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	configSectionStart = "# Added by Hisar: use the Hisar SSH agent"
	configSectionEnd   = "# End of Hisar section"
)

func getSSHConfigPath(homeDirectory string) string {
	return filepath.Join(homeDirectory, ".ssh", "config")
}

// formatSocketPath writes paths under the home directory with ~ so the config stays portable.
func formatSocketPath(homeDirectory, socketPath string) string {
	relativePath, err := filepath.Rel(homeDirectory, socketPath)
	if err != nil || relativePath == ".." || strings.HasPrefix(relativePath, "../") || filepath.IsAbs(relativePath) {
		return socketPath
	}
	return "~/" + filepath.ToSlash(relativePath)
}

func BuildSSHConfigSection(homeDirectory, socketPath string) string {
	return configSectionStart + "\nHost *\n  IdentityAgent \"" + formatSocketPath(homeDirectory, socketPath) + "\"\n" + configSectionEnd + "\n"
}

func readSSHConfig(homeDirectory string) (string, bool, error) {
	content, err := os.ReadFile(getSSHConfigPath(homeDirectory))
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return string(content), true, nil
}

func IsSSHConfigured(homeDirectory string) (bool, error) {
	content, _, err := readSSHConfig(homeDirectory)
	if err != nil {
		return false, err
	}
	return strings.Contains(content, configSectionStart), nil
}

// backUpSSHConfig never overwrites an earlier backup, which may hold the config from before Hisar.
func backUpSSHConfig(homeDirectory, content string, now time.Time) (string, error) {
	basePath := getSSHConfigPath(homeDirectory) + ".hisar-backup-" + now.Format("20060102-150405")
	for attempt := 1; attempt <= 100; attempt++ {
		backupPath := basePath
		if attempt > 1 {
			backupPath = fmt.Sprintf("%s-%d", basePath, attempt)
		}
		file, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("could not back up the SSH config: %w", err)
		}
		_, writeError := file.WriteString(content)
		closeError := file.Close()
		if err := errors.Join(writeError, closeError); err != nil {
			return "", fmt.Errorf("could not back up the SSH config: %w", err)
		}
		return backupPath, nil
	}
	return "", errors.New("could not back up the SSH config: too many backups")
}

// writeSSHConfig replaces the file atomically and keeps it private to the user.
func writeSSHConfig(homeDirectory, content string) error {
	configPath := getSSHConfigPath(homeDirectory)
	if resolvedPath, err := filepath.EvalSymlinks(configPath); err == nil {
		configPath = resolvedPath
	}
	partialPath := configPath + ".hisar-partial"
	if err := os.WriteFile(partialPath, []byte(content), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(partialPath, 0o600); err != nil {
		os.Remove(partialPath)
		return err
	}
	if err := os.Rename(partialPath, configPath); err != nil {
		os.Remove(partialPath)
		return err
	}
	return nil
}

// ConfigureSSHConfig adds the Hisar section at the top of ~/.ssh/config after backing the file up.
// It returns the backup path, empty when there was no file or the section is already present.
func ConfigureSSHConfig(homeDirectory, socketPath string, now time.Time) (string, error) {
	content, exists, err := readSSHConfig(homeDirectory)
	if err != nil {
		return "", err
	}
	if strings.Contains(content, configSectionStart) {
		return "", nil
	}
	if err := os.MkdirAll(filepath.Join(homeDirectory, ".ssh"), 0o700); err != nil {
		return "", err
	}
	backupPath := ""
	if exists {
		if backupPath, err = backUpSSHConfig(homeDirectory, content, now); err != nil {
			return "", err
		}
	}
	section := BuildSSHConfigSection(homeDirectory, socketPath)
	if content != "" {
		section += "\n"
	}
	if err := writeSSHConfig(homeDirectory, section+content); err != nil {
		return backupPath, err
	}
	return backupPath, nil
}

// RemoveSSHConfig deletes the Hisar section after backing the file up; other lines stay unchanged.
func RemoveSSHConfig(homeDirectory string, now time.Time) (string, error) {
	content, _, err := readSSHConfig(homeDirectory)
	if err != nil {
		return "", err
	}
	if !strings.Contains(content, configSectionStart) {
		return "", nil
	}
	_, afterStart, _ := strings.Cut(content, configSectionStart)
	if !strings.Contains(afterStart, configSectionEnd) {
		return "", errors.New("the Hisar section in ~/.ssh/config has no end line; remove it by hand")
	}
	backupPath, err := backUpSSHConfig(homeDirectory, content, now)
	if err != nil {
		return "", err
	}
	allLine := strings.SplitAfter(content, "\n")
	allKeptLine := make([]string, 0, len(allLine))
	isInSection := false
	isAfterSection := false
	for _, line := range allLine {
		trimmedLine := strings.TrimSpace(line)
		switch {
		case trimmedLine == configSectionStart:
			isInSection = true
		case isInSection:
			if trimmedLine == configSectionEnd {
				isInSection = false
				isAfterSection = true
			}
		case isAfterSection && trimmedLine == "":
			isAfterSection = false
		default:
			isAfterSection = false
			allKeptLine = append(allKeptLine, line)
		}
	}
	return backupPath, writeSSHConfig(homeDirectory, strings.Join(allKeptLine, ""))
}
