package appinfo

import (
	"errors"
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
