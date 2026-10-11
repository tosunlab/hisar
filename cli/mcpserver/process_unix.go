//go:build !windows

package mcpserver

import (
	"os/exec"
	"syscall"
	"time"
)

func startInProcessGroup(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// stopProcessGroup sends SIGTERM to the command's group, and SIGKILL when it is still running after killDelay.
func stopProcessGroup(command *exec.Cmd, waitResult <-chan error, killDelay time.Duration) error {
	syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
	select {
	case err := <-waitResult:
		syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		return err
	case <-time.After(killDelay):
		syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		return <-waitResult
	}
}
