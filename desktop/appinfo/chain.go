package appinfo

import (
	"path/filepath"
	"strings"
)

func GetProgramName(process Process) string {
	if process.Path != "" {
		return filepath.Base(process.Path)
	}
	if len(process.Arguments) > 0 {
		return filepath.Base(process.Arguments[0])
	}
	return ""
}

func isSSHServer(name string) bool {
	return name == "sshd" || name == "sshd-session" || strings.HasPrefix(name, "sshd:") || strings.HasPrefix(name, "sshd-session:")
}

func getMultiplexerName(name string) string {
	switch {
	case name == "tmux" || strings.HasPrefix(name, "tmux: "):
		return "tmux"
	case strings.EqualFold(name, "screen"):
		return "screen"
	}
	return ""
}

func isHiddenFromChain(process Process) bool {
	return GetProgramName(process) == "login"
}

// buildChain lists the processes nearest first up to limit, without login, at most maximumChainLength.
func buildChain(processes []Process, limit int) []Process {
	chainProcesses := []Process{}
	for _, process := range processes[:limit] {
		if isHiddenFromChain(process) {
			continue
		}
		if len(chainProcesses) == maximumChainLength {
			break
		}
		chainProcesses = append(chainProcesses, process)
	}
	return chainProcesses
}
