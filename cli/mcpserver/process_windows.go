package mcpserver

import (
	"os/exec"
	"time"
)

func startInProcessGroup(*exec.Cmd) {}

func stopProcessGroup(command *exec.Cmd, waitResult <-chan error, _ time.Duration) error {
	command.Process.Kill()
	return <-waitResult
}
