//go:build !windows

package mcpserver

import (
	"os/exec"
	"syscall"
	"time"
)

// processGroup is the process group the command leads, which every process it starts joins.
type processGroup struct {
	pid int
}

func startProcessGroup(command *exec.Cmd) (processGroup, error) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		return processGroup{}, err
	}
	return processGroup{pid: command.Process.Pid}, nil
}

// stop sends SIGTERM to the group, and SIGKILL when the command is still running after killDelay.
func (group processGroup) stop(waitResult <-chan error, killDelay time.Duration) error {
	syscall.Kill(-group.pid, syscall.SIGTERM)
	select {
	case err := <-waitResult:
		syscall.Kill(-group.pid, syscall.SIGKILL)
		return err
	case <-time.After(killDelay):
		syscall.Kill(-group.pid, syscall.SIGKILL)
		return <-waitResult
	}
}

func (processGroup) close() {}
