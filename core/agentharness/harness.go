// Package agentharness recognizes coding agent harnesses, such as omp, in the process chain behind a request, so the
// Hisar app answers requests from inside one with agent rules. The chain is a snapshot of process instances read from
// the kernel; an agent session is one harness process instance, its PID together with its start time.
package agentharness

import (
	"path/filepath"
	"slices"
	"strings"
)

// Harness is a coding agent harness and the ways to recognize its process. A process matches on any one of them.
type Harness struct {
	ID   string
	Name string
	// Signatures match an executable signed with one of these identifiers by its team.
	Signatures []Signature
	// ExecutableNames match the file name of the executable or of the process's first argument.
	ExecutableNames []string
	// Scripts match a runtime such as Bun running a script of that name.
	Scripts []Script
	// PackagePaths match an argument inside one of these packages.
	PackagePaths []string
	// WorkerArgumentPrefix marks the harness's own worker processes: an argument after the program that starts with it.
	// A worker belongs to the session of the harness process that started it.
	WorkerArgumentPrefix string
}

// Signature is a code signature's identifier and Team ID as macOS verified it; the zero Signature is none.
type Signature struct {
	Identifier string
	TeamID     string
}

// Script is a script a runtime runs: the runtime's executable name and the script's file name.
type Script struct {
	Runtime string
	Name    string
}

// harnesses is every harness Hisar recognizes. omp in Tern is signed and lives in a folder named after its version, so
// only its signature or its first argument names it; omp installed with Bun runs as bun with omp's bin link.
var harnesses = []Harness{
	{
		ID:                   "omp",
		Name:                 "omp",
		Signatures:           []Signature{{Identifier: "omp-darwin-arm64", TeamID: "VTLHGQC72S"}},
		ExecutableNames:      []string{"omp"},
		Scripts:              []Script{{Runtime: "bun", Name: "omp"}},
		PackagePaths:         []string{"@oh-my-pi/pi-coding-agent"},
		WorkerArgumentPrefix: "__omp_worker_",
	},
}

// Process is one process instance of a chain. StartTime is the kernel's start time, so two processes that held the same
// PID differ in it. Path, Arguments, WorkingDirectory and Signature are empty when they cannot be read.
type Process struct {
	PID              int
	ParentPID        int
	StartTime        int64
	Path             string
	Arguments        []string
	WorkingDirectory string
	Signature        Signature
}

// Chain is a process and its ancestors, nearest first. IsKnown is false when the snapshot cannot be trusted: a process
// was gone or changed its parent during the walk, or the walk hit its limit.
type Chain struct {
	Processes []Process
	IsKnown   bool
}

// Session is one harness process instance and the folder it works in.
type Session struct {
	Harness   Harness
	PID       int
	StartTime int64
	Directory string
}

// Find returns the session of the nearest harness in a known chain. A worker of
// the harness, such as omp's daemon broker, belongs to the session of the harness process that started it, so Find
// passes over a run of workers to that process. Any other harness process, even one whose parent is the same harness,
// is its own session.
func Find(chain Chain) (Session, bool) {
	if !chain.IsKnown {
		return Session{}, false
	}
	for index, process := range chain.Processes {
		for _, harness := range harnesses {
			if !harness.matches(process) {
				continue
			}
			for harness.isWorker(process) && index+1 < len(chain.Processes) && harness.matches(chain.Processes[index+1]) {
				index++
				process = chain.Processes[index]
			}
			return Session{Harness: harness, PID: process.PID, StartTime: process.StartTime, Directory: process.WorkingDirectory}, true
		}
	}
	return Session{}, false
}

func (harness Harness) isWorker(process Process) bool {
	return harness.WorkerArgumentPrefix != "" && slices.ContainsFunc(process.Arguments[min(1, len(process.Arguments)):], func(argument string) bool {
		return strings.HasPrefix(argument, harness.WorkerArgumentPrefix)
	})
}

func (harness Harness) matches(process Process) bool {
	if process.Signature != (Signature{}) && slices.Contains(harness.Signatures, process.Signature) {
		return true
	}
	programNames := listProgramNames(process)
	if slices.ContainsFunc(harness.ExecutableNames, func(name string) bool { return slices.Contains(programNames, name) }) {
		return true
	}
	if scriptName := getScriptName(process.Arguments); scriptName != "" {
		for _, script := range harness.Scripts {
			if script.Name == scriptName && slices.Contains(programNames, script.Runtime) {
				return true
			}
		}
	}
	return slices.ContainsFunc(process.Arguments, func(argument string) bool {
		return slices.ContainsFunc(harness.PackagePaths, func(packagePath string) bool {
			return strings.Contains(filepath.ToSlash(argument), "/"+packagePath+"/")
		})
	})
}

// listProgramNames names the program by its executable's file name and by its first argument, without ".exe", which
// some installs of Bun keep.
func listProgramNames(process Process) []string {
	names := []string{}
	if process.Path != "" {
		names = append(names, strings.TrimSuffix(filepath.Base(process.Path), ".exe"))
	}
	if len(process.Arguments) > 0 && process.Arguments[0] != "" {
		names = append(names, strings.TrimSuffix(filepath.Base(process.Arguments[0]), ".exe"))
	}
	return names
}

// getScriptName is the file name of the first argument after the program that is not an option.
func getScriptName(arguments []string) string {
	for _, argument := range arguments[min(1, len(arguments)):] {
		if !strings.HasPrefix(argument, "-") {
			return filepath.Base(argument)
		}
	}
	return ""
}
