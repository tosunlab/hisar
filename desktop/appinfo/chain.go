package appinfo

import (
	"path/filepath"
	"strings"
)

func GetProgramName(process Process) string {
	if process.Path != "" {
		return filepath.Base(process.Path)
	}
	if len(process.AllArgument) > 0 {
		return filepath.Base(process.AllArgument[0])
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
func buildChain(allProcess []Process, limit int) []Process {
	allChainProcess := []Process{}
	for _, process := range allProcess[:limit] {
		if isHiddenFromChain(process) {
			continue
		}
		if len(allChainProcess) == maximumChainLength {
			break
		}
		allChainProcess = append(allChainProcess, process)
	}
	return allChainProcess
}
