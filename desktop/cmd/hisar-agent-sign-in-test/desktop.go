package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/tryhisar/hisar/core/browserprotocol"
)

// desktopPaths are the development app's own files, none of them the installed app's: its state, cache, sockets and a
// home folder of its own, so nothing it writes on the way reaches the user's.
type desktopPaths struct {
	Executable string
	Home       string
	State      string
	Cache      string
	CLISocket  string
	SSHSocket  string
	Browser    string
	Log        string
}

func newDesktopPaths(directory, executable string) desktopPaths {
	return desktopPaths{
		Executable: executable,
		Home:       filepath.Join(directory, "home"),
		State:      filepath.Join(directory, "state", "desktop.json"),
		Cache:      filepath.Join(directory, "state", "desktop-cache.db"),
		CLISocket:  filepath.Join(directory, "s", "cli.sock"),
		SSHSocket:  filepath.Join(directory, "s", "agent.sock"),
		Browser:    filepath.Join(directory, "s", "browser.sock"),
		Log:        filepath.Join(directory, "logs", "desktop.log"),
	}
}

func (paths desktopPaths) environment(secretKey string) []string {
	environment := []string{
		"HOME=" + paths.Home, "SHELL=/bin/zsh", "PATH=/usr/bin:/bin:/usr/sbin:/sbin", "TMPDIR=" + os.Getenv("TMPDIR"),
		"HISAR_DESKTOP_STATE=" + paths.State, "HISAR_DESKTOP_CACHE=" + paths.Cache, "HISAR_CLI_SOCKET=" + paths.CLISocket,
		"HISAR_SSH_AGENT_SOCKET=" + paths.SSHSocket, "HISAR_BROWSER_SOCKET=" + paths.Browser, "HISAR_SECRET_KEY=" + secretKey,
	}
	return environment
}

type desktopApp struct {
	paths   desktopPaths
	command *exec.Cmd
	exited  chan struct{}
}

func startDesktop(paths desktopPaths, seeded account) (*desktopApp, error) {
	for _, directory := range []string{paths.Home, filepath.Dir(paths.State), filepath.Dir(paths.CLISocket), filepath.Dir(paths.Log)} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return nil, err
		}
	}
	logFile, err := os.OpenFile(paths.Log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	command := exec.Command(paths.Executable, "--hidden")
	command.Env = paths.environment(seeded.SecretKey)
	command.Stdout, command.Stderr = logFile, logFile
	command.Dir = paths.Home
	if err := command.Start(); err != nil {
		logFile.Close()
		return nil, err
	}
	desktop := &desktopApp{paths: paths, command: command, exited: make(chan struct{})}
	go func() {
		command.Wait()
		logFile.Close()
		close(desktop.exited)
	}()
	if err := unlockDesktop(paths.Browser, seeded); err != nil {
		desktop.stop()
		return nil, err
	}
	return desktop, nil
}

func (desktop *desktopApp) pid() int {
	return desktop.command.Process.Pid
}

func (desktop *desktopApp) showMainWindow() error {
	command := exec.Command(desktop.paths.Executable)
	command.Env = desktop.paths.environment("")
	command.Dir = desktop.paths.Home
	return command.Run()
}

func (desktop *desktopApp) stop() {
	desktop.command.Process.Signal(syscall.SIGTERM)
	select {
	case <-desktop.exited:
	case <-time.After(10 * time.Second):
		desktop.command.Process.Kill()
		<-desktop.exited
	}
}

func unlockDesktop(socketPath string, seeded account) error {
	var connection net.Conn
	var err error
	for deadline := time.Now().Add(60 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if connection, err = net.DialTimeout("unix", socketPath, time.Second); err == nil {
			break
		}
	}
	if err != nil {
		return fmt.Errorf("the app's browser socket: %w", err)
	}
	defer connection.Close()
	connection.SetDeadline(time.Now().Add(60 * time.Second))
	for _, message := range []any{
		browserprotocol.ConnectMessage{Type: "connect", ExtensionOrigin: browserprotocol.AllowedOrigin},
		browserprotocol.Request{ID: "unlock", Type: "unlock.password", Token: seeded.BrowserToken, Password: seeded.Password},
	} {
		encoded, _ := json.Marshal(message)
		if err := browserprotocol.WriteMessage(connection, encoded); err != nil {
			return err
		}
	}
	answer, err := browserprotocol.ReadMessage(connection)
	if err != nil {
		return err
	}
	var response struct {
		OK     bool                           `json:"ok"`
		Result browserprotocol.StatusResult   `json:"result"`
		Error  *browserprotocol.ResponseError `json:"error"`
	}
	if err := json.Unmarshal(answer, &response); err != nil || !response.OK || response.Result.State != "unlocked" {
		return fmt.Errorf("the app did not unlock: %s", answer)
	}
	return nil
}
