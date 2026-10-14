package appinfo

import (
	"path/filepath"
	"strings"
)

func (resolver *Resolver) listRegularAppPaths() map[string]int {
	regularAppPaths := map[string]int{}
	if resolver.system.ListRunningApps == nil {
		return regularAppPaths
	}
	for _, runningApp := range resolver.system.ListRunningApps() {
		if runningApp.IsRegular && runningApp.BundlePath != "" {
			regularAppPaths[strings.TrimSuffix(runningApp.BundlePath, "/")] = runningApp.PID
		}
	}
	return regularAppPaths
}

func getAppBundlePath(path string) string {
	if path == "" || IsDeveloperToolPath(path) {
		return ""
	}
	return getOutermostBundlePath(path)
}

// findDockApp returns the index of the nearest process whose bundle, or the app macOS holds responsible for it, is in the Dock.
func (resolver *Resolver) findDockApp(processes []Process, regularAppPaths map[string]int) (int, string, bool) {
	for index, process := range processes {
		if bundlePath := getAppBundlePath(process.Path); bundlePath != "" {
			if _, isRegular := regularAppPaths[bundlePath]; isRegular {
				return index, bundlePath, true
			}
		}
		if resolver.system.GetResponsiblePID == nil {
			continue
		}
		responsiblePID, err := resolver.system.GetResponsiblePID(process.PID)
		if err != nil || responsiblePID <= launchdPID || responsiblePID == process.PID {
			continue
		}
		responsiblePath, err := resolver.system.GetPath(responsiblePID)
		if err != nil {
			continue
		}
		if bundlePath := getAppBundlePath(responsiblePath); bundlePath != "" {
			if _, isRegular := regularAppPaths[bundlePath]; isRegular {
				return index + 1, bundlePath, true
			}
		}
	}
	return 0, "", false
}

// findAppBySigningTeam maps a daemon outside any bundle, like iTermServer, to the one Dock app signed by the same team.
func (resolver *Resolver) findAppBySigningTeam(process Process, regularAppPaths map[string]int) (string, bool) {
	if process.Path == "" || getOutermostBundlePath(process.Path) != "" || resolver.system.GetTeamID == nil || resolver.system.GetSignature == nil {
		return "", false
	}
	teamID := resolver.getTeamID(process.Path)
	if teamID == "" {
		return "", false
	}
	matchingPaths := []string{}
	for bundlePath := range regularAppPaths {
		if resolver.getTeamID(bundlePath) == teamID {
			matchingPaths = append(matchingPaths, bundlePath)
		}
	}
	if len(matchingPaths) != 1 {
		return "", false
	}
	if signature := resolver.getSignature(process.Path); signature.Status != SignatureStatusDeveloper || signature.TeamID != teamID {
		return "", false
	}
	return matchingPaths[0], true
}

// findAppInChain finds the app of a chain without following multiplexers or SSH sessions; the index is where the chain shown ends.
func (resolver *Resolver) findAppInChain(processes []Process, regularAppPaths map[string]int) (int, string, bool) {
	if index, bundlePath, isFound := resolver.findDockApp(processes, regularAppPaths); isFound {
		return index, bundlePath, true
	}
	for index, process := range processes {
		if bundlePath := getAppBundlePath(process.Path); bundlePath != "" {
			return index, bundlePath, true
		}
	}
	if len(processes) == 0 {
		return 0, "", false
	}
	oldest := processes[len(processes)-1]
	if parentPID, err := resolver.system.GetParentPID(oldest.PID); err != nil || parentPID > launchdPID {
		return 0, "", false
	}
	if bundlePath, isFound := resolver.findAppBySigningTeam(oldest, regularAppPaths); isFound {
		return len(processes) - 1, bundlePath, true
	}
	return 0, "", false
}

// findMultiplexerApp resolves every client of a tmux or screen server; they must all be in one app.
func (resolver *Resolver) findMultiplexerApp(server Process, name string, regularAppPaths map[string]int) (string, bool) {
	if resolver.system.ListProcessIDs == nil {
		return "", false
	}
	processIDs, err := resolver.system.ListProcessIDs()
	if err != nil {
		return "", false
	}
	foundPath := ""
	for _, pid := range processIDs {
		if pid == server.PID || pid <= launchdPID {
			continue
		}
		path, err := resolver.system.GetPath(pid)
		if err != nil || getMultiplexerName(filepath.Base(path)) != name {
			continue
		}
		if parentPID, err := resolver.system.GetParentPID(pid); err != nil || parentPID <= launchdPID {
			continue
		}
		clientProcesses := ListProcesses(pid, ProcessReader{GetPath: resolver.system.GetPath, GetParentPID: resolver.system.GetParentPID})
		_, bundlePath, isFound := resolver.findAppInChain(clientProcesses, regularAppPaths)
		if !isFound || (foundPath != "" && foundPath != bundlePath) {
			return "", false
		}
		foundPath = bundlePath
	}
	return foundPath, foundPath != ""
}

func (resolver *Resolver) describeSSHSession(processes []Process, index int) string {
	if resolver.system.GetSSHRemoteHost != nil {
		for _, process := range processes[index:] {
			if !isSSHServer(GetProgramName(process)) {
				break
			}
			if host, err := resolver.system.GetSSHRemoteHost(process.PID); err == nil && host != "" {
				return "An SSH session from " + host
			}
		}
	}
	return "An SSH session"
}
