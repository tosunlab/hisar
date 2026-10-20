package appinfo

import (
	"errors"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/agentharness"
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
// Identifier is the signing identifier, such as a bundle ID.
type Signature struct {
	Status     string
	TeamID     string
	SignerName string
	Identifier string
}

type RunningApp struct {
	PID        int
	BundlePath string
	IsRegular  bool
}

// System is what the resolver reads from the operating system; nil functions leave that step out, and a nil
// ReadInstance makes every chain unknown.
type System struct {
	ProcessReader
	ReadInstance func(pid int) (agentharness.Instance, error)
	// GetProcessSignature is the signing identifier and team of a running process signed by a certificate Apple issued,
	// as macOS verified it; the zero Signature for any other process.
	GetProcessSignature       func(pid int) agentharness.Signature
	GetResponsiblePID         func(pid int) (int, error)
	ListProcessIDs            func() ([]int, error)
	ListRunningApps           func() []RunningApp
	GetSignature              func(path string) Signature
	GetStrictSignature        func(path string) Signature
	GetTeamID                 func(path string) string
	GetBundleDisplayName      func(bundlePath string) string
	GetBundleIconPNG          func(bundlePath string, size int) ([]byte, error)
	GetSSHRemoteHost          func(pid int) (string, error)
	GetModificationTime       func(path string) (time.Time, error)
	GetBundleModificationTime func(path string) (time.Time, error)
}

// Requester is who a request comes from: the app responsible for it, or what stands in for one, and the process chain.
type Requester struct {
	Kind           string
	Name           string
	App            App
	Processes      []Process
	ChainProcesses []Process
}

type cachedSignature struct {
	modificationTime time.Time
	signature        Signature
}

// Resolver finds the app responsible for a process; bundle names, icons and signatures are cached per bundle path.
type Resolver struct {
	system                 System
	mutex                  sync.Mutex
	iconsByKey             map[string][]byte
	namesByPath            map[string]string
	signaturesByPath       map[string]cachedSignature
	strictSignaturesByPath map[string]cachedSignature
	teamIDsByPath          map[string]cachedSignature
}

func NewResolver(system System) *Resolver {
	if system.GetModificationTime == nil {
		system.GetModificationTime = getModificationTime
	}
	if system.GetBundleModificationTime == nil {
		system.GetBundleModificationTime = system.GetModificationTime
	}
	return &Resolver{
		system:                 system,
		iconsByKey:             map[string][]byte{},
		namesByPath:            map[string]string{},
		signaturesByPath:       map[string]cachedSignature{},
		strictSignaturesByPath: map[string]cachedSignature{},
		teamIDsByPath:          map[string]cachedSignature{},
	}
}

var errIconUnavailable = errors.New("appinfo: icons are not available")

// Resolve finds who is behind a process: the app responsible for it, an SSH session, a tmux server or a background process.
func (resolver *Resolver) Resolve(pid int) Requester {
	processes := ListProcesses(pid, resolver.system.ProcessReader)
	requester := Requester{Kind: RequesterKindProcess, Name: "A process without an app", Processes: processes, ChainProcesses: buildChain(processes, len(processes))}
	if len(processes) == 0 {
		return requester
	}
	regularAppPaths := resolver.listRegularAppPaths()
	if index, bundlePath, isFound := resolver.findAppInChain(processes, regularAppPaths); isFound {
		return resolver.buildAppRequester(requester, bundlePath, buildChain(processes, index))
	}
	for index, process := range processes {
		name := GetProgramName(process)
		if isSSHServer(name) {
			requester.Kind, requester.Name, requester.ChainProcesses = RequesterKindSSHSession, resolver.describeSSHSession(processes, index), buildChain(processes, index)
			return requester
		}
		if multiplexerName := getMultiplexerName(name); multiplexerName != "" {
			chainProcesses := buildChain(processes, index+1)
			if bundlePath, isFound := resolver.findMultiplexerApp(process, multiplexerName, regularAppPaths); isFound {
				return resolver.buildAppRequester(requester, bundlePath, chainProcesses)
			}
			requester.Kind, requester.Name, requester.ChainProcesses = RequesterKindMultiplexer, multiplexerName, chainProcesses
			return requester
		}
	}
	oldest := processes[len(processes)-1]
	if parentPID, err := resolver.system.GetParentPID(oldest.PID); err == nil && parentPID <= launchdPID {
		requester.Kind, requester.Name = RequesterKindBackground, "A background process"
	}
	return requester
}

func (resolver *Resolver) buildAppRequester(requester Requester, bundlePath string, chainProcesses []Process) Requester {
	requester.Kind, requester.App, requester.ChainProcesses = RequesterKindApp, resolver.DescribeApp(bundlePath), chainProcesses
	requester.Name = requester.App.Name
	return requester
}

// ReadChain reads the process chain behind pid as a snapshot of process instances, with each running process's signing
// identifier and team where its signature verifies, for finding an agent harness in it.
func (resolver *Resolver) ReadChain(pid int) agentharness.Chain {
	if resolver.system.ReadInstance == nil {
		return agentharness.Chain{}
	}
	return agentharness.ReadChain(pid, agentharness.Reader{
		ReadInstance:        resolver.system.ReadInstance,
		GetPath:             resolver.system.GetPath,
		ListArguments:       resolver.system.ListArguments,
		GetWorkingDirectory: resolver.system.GetWorkingDirectory,
		GetSignature:        resolver.system.GetProcessSignature,
	})
}
