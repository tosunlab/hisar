package appinfo

import (
	"path/filepath"
	"strings"
)

func (resolver *Resolver) listAllRegularAppPath() map[string]int {
	allRegularAppPath := map[string]int{}
	if resolver.system.ListAllRunningApp == nil {
		return allRegularAppPath
	}
	for _, runningApp := range resolver.system.ListAllRunningApp() {
		if runningApp.IsRegular && runningApp.BundlePath != "" {
			allRegularAppPath[strings.TrimSuffix(runningApp.BundlePath, "/")] = runningApp.PID
		}
	}
	return allRegularAppPath
}

func getAppBundlePath(path string) string {
	if path == "" || IsDeveloperToolPath(path) {
		return ""
	}
	return getOutermostBundlePath(path)
}

// findDockApp returns the index of the nearest process whose bundle, or the app macOS holds responsible for it, is in the Dock.
func (resolver *Resolver) findDockApp(allProcess []Process, allRegularAppPath map[string]int) (int, string, bool) {
	for index, process := range allProcess {
		if bundlePath := getAppBundlePath(process.Path); bundlePath != "" {
			if _, isRegular := allRegularAppPath[bundlePath]; isRegular {
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
			if _, isRegular := allRegularAppPath[bundlePath]; isRegular {
				return index + 1, bundlePath, true
			}
		}
	}
	return 0, "", false
}

// findAppBySigningTeam maps a daemon outside any bundle, like iTermServer, to the one Dock app signed by the same team.
func (resolver *Resolver) findAppBySigningTeam(process Process, allRegularAppPath map[string]int) (string, bool) {
	if process.Path == "" || getOutermostBundlePath(process.Path) != "" || resolver.system.GetTeamID == nil || resolver.system.GetSignature == nil {
		return "", false
	}
	teamID := resolver.getTeamID(process.Path)
	if teamID == "" {
		return "", false
	}
	allMatchingPath := []string{}
	for bundlePath := range allRegularAppPath {
		if resolver.getTeamID(bundlePath) == teamID {
			allMatchingPath = append(allMatchingPath, bundlePath)
		}
	}
	if len(allMatchingPath) != 1 {
		return "", false
	}
	if signature := resolver.getSignature(process.Path); signature.Status != SignatureStatusDeveloper || signature.TeamID != teamID {
		return "", false
	}
	return allMatchingPath[0], true
}

// findAppInChain finds the app of a chain without following multiplexers or SSH sessions; the index is where the chain shown ends.
func (resolver *Resolver) findAppInChain(allProcess []Process, allRegularAppPath map[string]int) (int, string, bool) {
	if index, bundlePath, isFound := resolver.findDockApp(allProcess, allRegularAppPath); isFound {
		return index, bundlePath, true
	}
	for index, process := range allProcess {
		if bundlePath := getAppBundlePath(process.Path); bundlePath != "" {
			return index, bundlePath, true
		}
	}
	if len(allProcess) == 0 {
		return 0, "", false
	}
	oldest := allProcess[len(allProcess)-1]
	if parentPID, err := resolver.system.GetParentPID(oldest.PID); err != nil || parentPID > launchdPID {
		return 0, "", false
	}
	if bundlePath, isFound := resolver.findAppBySigningTeam(oldest, allRegularAppPath); isFound {
		return len(allProcess) - 1, bundlePath, true
	}
	return 0, "", false
}

// findMultiplexerApp resolves every client of a tmux or screen server; they must all be in one app.
func (resolver *Resolver) findMultiplexerApp(server Process, name string, allRegularAppPath map[string]int) (string, bool) {
	if resolver.system.ListAllProcessID == nil {
		return "", false
	}
	allProcessID, err := resolver.system.ListAllProcessID()
	if err != nil {
		return "", false
	}
	foundPath := ""
	for _, pid := range allProcessID {
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
		allClientProcess := ListAllProcess(pid, ProcessReader{GetPath: resolver.system.GetPath, GetParentPID: resolver.system.GetParentPID})
		_, bundlePath, isFound := resolver.findAppInChain(allClientProcess, allRegularAppPath)
		if !isFound || (foundPath != "" && foundPath != bundlePath) {
			return "", false
		}
		foundPath = bundlePath
	}
	return foundPath, foundPath != ""
}

func (resolver *Resolver) describeSSHSession(allProcess []Process, index int) string {
	if resolver.system.GetSSHRemoteHost != nil {
		for _, process := range allProcess[index:] {
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
