// Package agentharness recognizes coding agent harnesses, such as omp, in the process chain behind a request, so the
// Hisar app answers requests from inside one with agent rules. The chain is a snapshot of process instances read from
// the kernel; an agent session is one harness process instance, its PID together with its start time.
package agentharness

import (
	"crypto/sha256"
	"encoding/hex"
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
	// PythonModules match a Python running one of these modules: python -m module, or python -c with source that runs
	// the module's main, as Hermes' launcher does with `from hermes_cli.main import main`.
	PythonModules []string
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
// file name. Hermes is unsigned Python: its launcher, which hermes and Hermes Desktop both run, starts the managed Python
// with -I -c and source that imports hermes_cli.main, and a development backend runs python -m hermes_cli.main.
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
	{
		ID:            "hermes",
		Name:          "Hermes",
		PythonModules: []string{"hermes_cli.main"},
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

// Session is one harness process instance, the program it runs and the folder it works in.
type Session struct {
	Harness   Harness
	PID       int
	StartTime int64
	Directory string
	Program   Program
}

// Program binds grants to a verified code signature or kernel-reported executable path.
// Interpreted harnesses also require an Entrypoint to distinguish scripts sharing a runtime.
// The zero Program is unknown and matches no grants.
type Program struct {
	Signature  Signature
	Path       string
	Entrypoint string
}

func (program Program) IsKnown() bool {
	return program != Program{}
}

// IsSigned reports whether the process matched its harness by code signature, which no other program can copy.
func (session Session) IsSigned() bool {
	return session.Program.Signature != Signature{}
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
				Program: harness.getProgram(process),
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

func (harness Harness) getProgram(process Process) Program {
	switch {
	case harness.isSignedBy(process):
		return Program{Signature: process.Signature}
	case process.Path == "":
		return Program{}
	case harness.isNamedByExecutable(process.Path):
		return Program{Path: process.Path}
	}
	entrypoint, isRead := readEntrypoint(process.Path, process.Arguments)
	if !isRead {
		return Program{}
	}
	return Program{Path: process.Path, Entrypoint: entrypoint}
}

func (harness Harness) isNamedByExecutable(path string) bool {
	return slices.Contains(harness.ExecutableNames, strings.TrimSuffix(filepath.Base(path), ".exe")) || harness.isInVersionFolder(path)
}

// Relative scripts and Python modules have no externally verifiable entry path.
// Loader options, repeated eval options and Python without -I can run other code.
// Replaced scripts and code injected through the environment remain outside this identity check.
func readEntrypoint(path string, arguments []string) (string, bool) {
	arguments = arguments[min(1, len(arguments)):]
	switch runtime := strings.TrimSuffix(filepath.Base(path), ".exe"); {
	case runtime == "node" || runtime == "bun":
		return readJavaScriptEntrypoint(arguments)
	case isPythonName(runtime) && len(arguments) > 2 && arguments[0] == "-I" && arguments[1] == "-c":
		return hashSource(arguments[2]), true
	}
	return "", false
}

// jsLoaderOptions are the Node and Bun options that load code besides the entry: preloads, loaders, and env files,
// whose NODE_OPTIONS can add them.
var jsLoaderOptions = []string{"-r", "--require", "--import", "--loader", "--experimental-loader", "--preload", "--env-file", "--env-file-if-exists"}

var jsEvalOptions = []string{"-e", "--eval", "-p", "--print"}

func readJavaScriptEntrypoint(arguments []string) (string, bool) {
	if len(arguments) == 0 || slices.ContainsFunc(arguments, func(argument string) bool {
		name, _, _ := strings.Cut(argument, "=")
		return slices.Contains(jsLoaderOptions, name)
	}) {
		return "", false
	}
	first := arguments[0]
	switch {
	case slices.Contains(jsEvalOptions, first) && len(arguments) > 1:
		if slices.ContainsFunc(arguments[2:], isJavaScriptEvalOption) {
			return "", false
		}
		return hashSource(arguments[1]), true
	case strings.HasPrefix(first, "-") || !filepath.IsAbs(first):
		return "", false
	}
	path, err := filepath.EvalSymlinks(first)
	if err != nil {
		return "", false
	}
	return "script:" + path, true
}

func isJavaScriptEvalOption(argument string) bool {
	name, _, _ := strings.Cut(argument, "=")
	return slices.Contains(jsEvalOptions, name) || strings.HasPrefix(name, "-") && !strings.HasPrefix(name, "--") && strings.ContainsAny(name, "ep")
}

func hashSource(source string) string {
	sum := sha256.Sum256([]byte(source))
	return "source:" + hex.EncodeToString(sum[:])
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
	if slices.ContainsFunc(process.Arguments, func(argument string) bool {
		return slices.ContainsFunc(harness.PackagePaths, func(packagePath string) bool {
			return strings.Contains(filepath.ToSlash(argument), "/"+packagePath+"/")
		})
	}) {
		return true
	}
	return harness.runsPythonModule(programNames, process.Arguments)
}

func (harness Harness) runsPythonModule(programNames, arguments []string) bool {
	if len(harness.PythonModules) == 0 || !slices.ContainsFunc(programNames, isPythonName) {
		return false
	}
	module, source := getPythonCode(arguments)
	return slices.ContainsFunc(harness.PythonModules, func(name string) bool {
		if module == name {
			return true
		}
		for line := range strings.Lines(source) {
			if strings.TrimSpace(line) == "from "+name+" import main" {
				return true
			}
		}
		return false
	})
}

func isPythonName(name string) bool {
	version, isPython := strings.CutPrefix(strings.ToLower(name), "python")
	return isPython && strings.Trim(version, "0123456789.") == ""
}

func getPythonCode(arguments []string) (module, source string) {
	for i := 1; i < len(arguments); i++ {
		switch argument := arguments[i]; {
		case argument == "-m" && i+1 < len(arguments):
			return arguments[i+1], ""
		case argument == "-c" && i+1 < len(arguments):
			return "", arguments[i+1]
		case argument == "-W" || argument == "-X":
			i++
		case !strings.HasPrefix(argument, "-") || argument == "-" || argument == "--":
			return "", ""
		}
	}
	return "", ""
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
