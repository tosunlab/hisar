package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	noLaunchEnvironment    = "HISAR_NO_LAUNCH"
	agentSocketEnvironment = "HISAR_SSH_AGENT_SOCKET"
	hiddenLaunchArgument   = "--hidden"
	appBundleID            = "com.tryhisar.hisar"
	appLaunchPollInterval  = 200 * time.Millisecond
)

var (
	appLaunchTimeout   = 10 * time.Second
	errAppNotInstalled = errors.New("the Hisar app is not installed")
	errAppNotRunning   = errors.New("Hisar isn't running. Open the Hisar app and try again.")
)

// appLauncher starts the desktop app hidden; errAppNotInstalled means there is none to start.
type appLauncher interface {
	LaunchApp() error
}

var selectedAppLauncher appLauncher = systemAppLauncher{}

// appUnreachableError is a socket that is missing or refuses the connection, the case where starting the app helps.
type appUnreachableError struct{ socketPath string }

func (err *appUnreachableError) Error() string {
	return "the Hisar app is not running, or does not offer " + err.socketPath
}

func isLaunchDisabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(noLaunchEnvironment)))
	return value == "1" || value == "true" || value == "yes"
}

// isAppRunning finds an app whose CLI socket is off: its SSH agent still answers, and starting it again would only show its window.
func isAppRunning() bool {
	agentSocketPath := os.Getenv(agentSocketEnvironment)
	if agentSocketPath == "" {
		homeDirectory, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		agentSocketPath = filepath.Join(homeDirectory, ".hisar", "agent.sock")
	}
	connection, err := net.DialTimeout("unix", agentSocketPath, 500*time.Millisecond)
	if err != nil {
		return false
	}
	connection.Close()
	return true
}

// launchAndDialApp starts the app and waits for its socket; isLaunched tells a missing app from one that did not answer.
func launchAndDialApp(socketPath string, launcher appLauncher, notice io.Writer, timeout, interval time.Duration) (client *appClient, isLaunched bool, err error) {
	if err := launcher.LaunchApp(); err != nil {
		if errors.Is(err, errAppNotInstalled) {
			return nil, false, err
		}
		return nil, true, fmt.Errorf("could not start the Hisar app: %w", err)
	}
	fmt.Fprintln(notice, "Starting Hisar…")
	deadline := time.Now().Add(timeout)
	for {
		client, err := dialApp(socketPath)
		var unreachable *appUnreachableError
		if err == nil || !errors.As(err, &unreachable) {
			return client, true, err
		}
		if time.Now().Add(interval).After(deadline) {
			return nil, true, errAppNotRunning
		}
		time.Sleep(interval)
	}
}

// connectToApp dials the app and, when nothing listens, starts it unless launching is off or would not help.
func connectToApp(socketPath string, isLaunchAllowed bool) commandMode {
	client, err := dialApp(socketPath)
	if err == nil {
		return commandMode{client: client, socketPath: socketPath, reason: "the Hisar app answered on " + socketPath}
	}
	var unreachable *appUnreachableError
	if !errors.As(err, &unreachable) || !isLaunchAllowed || isLaunchDisabled() || isAppRunning() {
		return commandMode{socketPath: socketPath, reason: err.Error()}
	}
	client, isLaunched, launchError := launchAndDialApp(socketPath, selectedAppLauncher, os.Stderr, appLaunchTimeout, appLaunchPollInterval)
	switch {
	case launchError == nil:
		return commandMode{client: client, socketPath: socketPath, reason: "the Hisar app answered on " + socketPath + " after hisar started it"}
	case !isLaunched:
		return commandMode{socketPath: socketPath, reason: err.Error()}
	}
	return commandMode{socketPath: socketPath, reason: launchError.Error(), isAppNotRunning: errors.Is(launchError, errAppNotRunning)}
}

// findAppBundlePath returns the Hisar.app that holds this hisar, following the installed symlink.
func findAppBundlePath(executablePath string) string {
	if resolvedPath, err := filepath.EvalSymlinks(executablePath); err == nil {
		executablePath = resolvedPath
	}
	bundlePath, _, isFound := strings.Cut(executablePath, ".app/Contents/")
	if !isFound {
		return ""
	}
	return bundlePath + ".app"
}

func findAppBinary(candidatePaths []string) string {
	for _, candidatePath := range candidatePaths {
		if candidatePath == "" {
			continue
		}
		fileInfo, err := os.Stat(candidatePath)
		if err == nil && fileInfo.Mode().IsRegular() && fileInfo.Mode().Perm()&0o111 != 0 {
			return candidatePath
		}
	}
	return ""
}
