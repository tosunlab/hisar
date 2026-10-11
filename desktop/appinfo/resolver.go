package appinfo

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	RequesterKindApp         = "app"
	RequesterKindSSHSession  = "ssh-session"
	RequesterKindMultiplexer = "multiplexer"
	RequesterKindBackground  = "background"
	RequesterKindProcess     = "process"

	SignatureStatusApple     = "apple"
	SignatureStatusDeveloper = "developer"
	SignatureStatusUnsigned  = "unsigned"
	SignatureStatusInvalid   = "invalid"

	DialogIconSize = 128
	PhoneIconSize  = 32

	maximumChainLength = 8
	launchdPID         = 1
)

// Signature is a code signature as macOS checked it; an empty Status means the platform cannot check signatures.
type Signature struct {
	Status     string
	TeamID     string
	SignerName string
}

type RunningApp struct {
	PID        int
	BundlePath string
	IsRegular  bool
}

// System is what the resolver reads from the operating system; nil functions leave that step out.
type System struct {
	ProcessReader
	GetResponsiblePID    func(pid int) (int, error)
	ListAllProcessID     func() ([]int, error)
	ListAllRunningApp    func() []RunningApp
	GetSignature         func(path string) Signature
	GetTeamID            func(path string) string
	GetBundleDisplayName func(bundlePath string) string
	GetBundleIconPNG     func(bundlePath string, size int) ([]byte, error)
	GetSSHRemoteHost     func(pid int) (string, error)
	GetModificationTime  func(path string) (time.Time, error)
}

// Requester is who a request comes from: the app responsible for it, or what stands in for one, and the process chain.
type Requester struct {
	Kind            string
	Name            string
	App             App
	AllProcess      []Process
	AllChainProcess []Process
}

type cachedSignature struct {
	modificationTime time.Time
	signature        Signature
}

// Resolver finds the app responsible for a process; bundle names, icons and signatures are cached per bundle path.
type Resolver struct {
	system             System
	mutex              sync.Mutex
	allIconByKey       map[string][]byte
	allNameByPath      map[string]string
	allSignatureByPath map[string]cachedSignature
	allTeamIDByPath    map[string]cachedSignature
}

func CreateResolver(system System) *Resolver {
	if system.GetModificationTime == nil {
		system.GetModificationTime = getModificationTime
	}
	return &Resolver{
		system:             system,
		allIconByKey:       map[string][]byte{},
		allNameByPath:      map[string]string{},
		allSignatureByPath: map[string]cachedSignature{},
		allTeamIDByPath:    map[string]cachedSignature{},
	}
}

var errIconUnavailable = errors.New("appinfo: icons are not available")

// getModificationTime is the latest change to a file, or to a bundle's executables, Info.plist and signature.
func getModificationTime(path string) (time.Time, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	latest := fileInfo.ModTime()
	if !fileInfo.IsDir() {
		return latest, nil
	}
	contentsPath := filepath.Join(path, "Contents")
	executableDirectory := filepath.Join(contentsPath, "MacOS")
	allPath := []string{contentsPath, filepath.Join(contentsPath, "Info.plist"), executableDirectory, filepath.Join(contentsPath, "_CodeSignature", "CodeResources")}
	if allEntry, err := os.ReadDir(executableDirectory); err == nil {
		for _, entry := range allEntry {
			allPath = append(allPath, filepath.Join(executableDirectory, entry.Name()))
		}
	}
	for _, candidatePath := range allPath {
		if candidateInfo, err := os.Stat(candidatePath); err == nil && candidateInfo.ModTime().After(latest) {
			latest = candidateInfo.ModTime()
		}
	}
	return latest, nil
}

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

// Resolve finds who is behind a process: the app responsible for it, an SSH session, a tmux server or a background process.
func (resolver *Resolver) Resolve(pid int) Requester {
	allProcess := ListAllProcess(pid, resolver.system.ProcessReader)
	requester := Requester{Kind: RequesterKindProcess, Name: "A process without an app", AllProcess: allProcess, AllChainProcess: buildChain(allProcess, len(allProcess))}
	if len(allProcess) == 0 {
		return requester
	}
	allRegularAppPath := resolver.listAllRegularAppPath()
	if index, bundlePath, isFound := resolver.findAppInChain(allProcess, allRegularAppPath); isFound {
		return resolver.buildAppRequester(requester, bundlePath, buildChain(allProcess, index))
	}
	for index, process := range allProcess {
		name := GetProgramName(process)
		if isSSHServer(name) {
			requester.Kind, requester.Name, requester.AllChainProcess = RequesterKindSSHSession, resolver.describeSSHSession(allProcess, index), buildChain(allProcess, index)
			return requester
		}
		if multiplexerName := getMultiplexerName(name); multiplexerName != "" {
			allChainProcess := buildChain(allProcess, index+1)
			if bundlePath, isFound := resolver.findMultiplexerApp(process, multiplexerName, allRegularAppPath); isFound {
				return resolver.buildAppRequester(requester, bundlePath, allChainProcess)
			}
			requester.Kind, requester.Name, requester.AllChainProcess = RequesterKindMultiplexer, multiplexerName, allChainProcess
			return requester
		}
	}
	oldest := allProcess[len(allProcess)-1]
	if parentPID, err := resolver.system.GetParentPID(oldest.PID); err == nil && parentPID <= launchdPID {
		requester.Kind, requester.Name = RequesterKindBackground, "A background process"
	}
	return requester
}

func (resolver *Resolver) buildAppRequester(requester Requester, bundlePath string, allChainProcess []Process) Requester {
	requester.Kind, requester.App, requester.AllChainProcess = RequesterKindApp, resolver.DescribeApp(bundlePath), allChainProcess
	requester.Name = requester.App.Name
	return requester
}

// DescribeApp reads an app's name, icon and code signature from its bundle on disk.
func (resolver *Resolver) DescribeApp(bundlePath string) App {
	app := App{Path: bundlePath, Name: resolver.getBundleName(bundlePath), Signature: resolver.getSignature(bundlePath)}
	if icon, err := resolver.GetIconPNG(bundlePath, DialogIconSize); err == nil {
		app.IconURL = "data:image/png;base64," + base64.StdEncoding.EncodeToString(icon)
	}
	return app
}

func (resolver *Resolver) getBundleName(bundlePath string) string {
	resolver.mutex.Lock()
	name, isCached := resolver.allNameByPath[bundlePath]
	resolver.mutex.Unlock()
	if isCached {
		return name
	}
	if resolver.system.GetBundleDisplayName != nil {
		name = resolver.system.GetBundleDisplayName(bundlePath)
	}
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(bundlePath), ".app")
	}
	resolver.mutex.Lock()
	resolver.allNameByPath[bundlePath] = name
	resolver.mutex.Unlock()
	return name
}

// GetIconPNG renders an app's icon at size pixels square, once per bundle path and size.
func (resolver *Resolver) GetIconPNG(bundlePath string, size int) ([]byte, error) {
	key := bundlePath + "\x00" + strconv.Itoa(size)
	resolver.mutex.Lock()
	icon, isCached := resolver.allIconByKey[key]
	resolver.mutex.Unlock()
	if isCached {
		return icon, nil
	}
	if resolver.system.GetBundleIconPNG == nil || bundlePath == "" {
		return nil, errIconUnavailable
	}
	icon, err := resolver.system.GetBundleIconPNG(bundlePath, size)
	if err != nil {
		return nil, err
	}
	resolver.mutex.Lock()
	resolver.allIconByKey[key] = icon
	resolver.mutex.Unlock()
	return icon, nil
}

// getSignature checks a code signature again whenever the bundle or file changed on disk.
func (resolver *Resolver) getSignature(path string) Signature {
	if resolver.system.GetSignature == nil {
		return Signature{}
	}
	return resolver.getCachedSignature(resolver.allSignatureByPath, path, resolver.system.GetSignature)
}

// getTeamID reads the Team ID a file is signed with, without checking the signature.
func (resolver *Resolver) getTeamID(path string) string {
	return resolver.getCachedSignature(resolver.allTeamIDByPath, path, func(path string) Signature {
		return Signature{TeamID: resolver.system.GetTeamID(path)}
	}).TeamID
}

func (resolver *Resolver) getCachedSignature(allCachedByPath map[string]cachedSignature, path string, read func(path string) Signature) Signature {
	modificationTime, err := resolver.system.GetModificationTime(path)
	if err != nil {
		return read(path)
	}
	resolver.mutex.Lock()
	cached, isCached := allCachedByPath[path]
	resolver.mutex.Unlock()
	if isCached && cached.modificationTime.Equal(modificationTime) {
		return cached.signature
	}
	signature := read(path)
	resolver.mutex.Lock()
	allCachedByPath[path] = cachedSignature{modificationTime: modificationTime, signature: signature}
	resolver.mutex.Unlock()
	return signature
}
