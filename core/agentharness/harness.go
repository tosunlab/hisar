// Package agentharness recognizes coding agent harnesses, such as omp, in the process chain behind a request, so the
// Hisar app answers requests from inside one with agent rules. The chain is a snapshot of process instances read from
// the kernel; an agent session is one harness process instance, its PID together with its start time.
package agentharness

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
)

// ErrUnknownChain is a chain whose snapshot cannot be trusted, so whether a harness runs in it is unknown.
var ErrUnknownChain = errors.New("agentharness: the process chain changed while it was read, or is too long")

// Harness is a coding agent harness and the ways to recognize its process. A process matches on any one of them.
type Harness struct {
	ID   string
	Name string
	// Signatures match an executable signed with one of these identifiers by its team.
	Signatures []Signature
	// ExecutableNames match the file name of the executable or of the process's first argument.
	ExecutableNames []string
	// VersionFolders match an executable or first argument directly inside a folder whose path ends in one of these: a
	// native install that names each executable after its version.
	VersionFolders []string
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

// harnesses is every harness Hisar recognizes. omp in Tern is signed and lives in Tern's folder of omp versions, named
// after its version, so its signature, that folder or its first argument names it; omp installed with Bun runs as bun
// with omp's bin link. Claude Code's native install is a signed executable in its folder of versions, started through a
// link named claude; installed with npm it runs as node with its bin link or its package's script. Codex runs as node
// with its bin link, which starts the signed native codex; the native process is nearer every command and hisar mcp, so
// it is the session. Codex's release archives, which Homebrew's cask installed before 0.146.1, name the native codex
// after its target. hisar reads no signature, so a native install started by its own path is found by its folder or
// file name.
var harnesses = []Harness{
	{
		ID:                   "omp",
		Name:                 "omp",
		Signatures:           []Signature{{Identifier: "omp-darwin-arm64", TeamID: "VTLHGQC72S"}},
		ExecutableNames:      []string{"omp"},
		VersionFolders:       []string{"Tern/omp/versions"},
		Scripts:              []Script{{Runtime: "bun", Name: "omp"}},
		PackagePaths:         []string{"@oh-my-pi/pi-coding-agent"},
		WorkerArgumentPrefix: "__omp_worker_",
	},
	{
		ID:              "claude-code",
		Name:            "Claude Code",
		Signatures:      []Signature{{Identifier: "com.anthropic.claude-code", TeamID: "Q6L2SF6YDW"}},
		ExecutableNames: []string{"claude"},
		VersionFolders:  []string{"claude/versions"},
		Scripts:         []Script{{Runtime: "node", Name: "claude"}},
		PackagePaths:    []string{"@anthropic-ai/claude-code"},
	},
	{
		ID:         "codex",
		Name:       "Codex",
		Signatures: []Signature{{Identifier: "codex", TeamID: "2DC432GLL2"}},
		ExecutableNames: []string{
			"codex", "codex-aarch64-apple-darwin", "codex-x86_64-apple-darwin", "codex-aarch64-unknown-linux-musl",
			"codex-x86_64-unknown-linux-musl",
		},
		Scripts:      []Script{{Runtime: "node", Name: "codex"}},
		PackagePaths: []string{"@openai/codex"},
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

// Session is one harness process instance and the folder it works in. IsSigned: the process matched its harness by
// code signature, which no other program can copy, rather than by a name or a path any program can take.
type Session struct {
	Harness   Harness
	PID       int
	StartTime int64
	Directory string
	IsSigned  bool
}

// Find returns the session of the nearest harness in a chain, and ErrUnknownChain when the chain is unknown. A worker of
// the harness, such as omp's daemon broker, belongs to the session of the harness process that started it, so Find
// passes over a run of workers to that process. Any other harness process, even one whose parent is the same harness,
// is its own session.
func Find(chain Chain) (Session, bool, error) {
	if !chain.IsKnown {
		return Session{}, false, ErrUnknownChain
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
			return Session{
				Harness: harness, PID: process.PID, StartTime: process.StartTime, Directory: process.WorkingDirectory,
				IsSigned: harness.isSignedBy(process),
			}, true, nil
		}
	}
	return Session{}, false, nil
}

func (harness Harness) isWorker(process Process) bool {
	return harness.WorkerArgumentPrefix != "" && slices.ContainsFunc(process.Arguments[min(1, len(process.Arguments)):], func(argument string) bool {
		return strings.HasPrefix(argument, harness.WorkerArgumentPrefix)
	})
}

func (harness Harness) isSignedBy(process Process) bool {
	return process.Signature != (Signature{}) && slices.Contains(harness.Signatures, process.Signature)
}

func (harness Harness) matches(process Process) bool {
	if harness.isSignedBy(process) {
		return true
	}
	programPaths := listProgramPaths(process)
	// Some installs of Bun keep ".exe" in the program's name.
	programNames := make([]string, len(programPaths))
	for i, path := range programPaths {
		programNames[i] = strings.TrimSuffix(filepath.Base(path), ".exe")
	}
	if slices.ContainsFunc(harness.ExecutableNames, func(name string) bool { return slices.Contains(programNames, name) }) {
		return true
	}
	if slices.ContainsFunc(programPaths, harness.isInVersionFolder) {
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

func (harness Harness) isInVersionFolder(path string) bool {
	directory := filepath.ToSlash(filepath.Dir(path))
	return slices.ContainsFunc(harness.VersionFolders, func(folder string) bool {
		return strings.HasSuffix(directory, "/"+folder)
	})
}

// listProgramPaths is the program's executable and its first argument.
func listProgramPaths(process Process) []string {
	paths := []string{}
	if process.Path != "" {
		paths = append(paths, process.Path)
	}
	if len(process.Arguments) > 0 && process.Arguments[0] != "" {
		paths = append(paths, process.Arguments[0])
	}
	return paths
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
