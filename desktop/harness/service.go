// Package harness connects coding agents to Hisar from Settings → Coding agents: omp through the Hisar plugin that ships
// inside the app, Claude Code, Codex and Hermes through Hisar's MCP server, the hisar inside the app run as
// `hisar mcp --no-reveal`.
package harness

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
)

// State is where a coding agent stands with Hisar.
type State string

const (
	StateNotInstalled State = "not-installed"
	StateAvailable    State = "available"
	StateConnected    State = "connected"
	// StateConflict: the agent already has an MCP server named hisar that Hisar did not add. Hisar never changes it.
	StateConflict State = "conflict"
	// StateUnknown: Hisar could not read the agent's settings; Detail says why.
	StateUnknown State = "unknown"
)

// Agent is one coding agent as Settings shows it.
type Agent struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State State  `json:"state"`
	// Detail is the sentence for a conflict or an unknown state.
	Detail string `json:"detail"`
}

// mcpServerName is the name Hisar's server has in every agent's MCP settings.
const mcpServerName = "hisar"

// mcpServerArguments start hisar's MCP server without get_secret and get_one_time_code, as the omp plugin does.
var mcpServerArguments = []string{"mcp", "--no-reveal"}

// errConflict: the agent has an MCP server named hisar that is not Hisar's.
var errConflict = errors.New("the agent already has an MCP server named hisar")

// A connector is how Hisar connects one agent. connect and disconnect converge: connecting a connected agent and
// disconnecting one that is not connected change nothing.
type connector interface {
	getState(agent program) (State, error)
	connect(agent program) error
	disconnect(agent program) error
}

type agentKind struct {
	id   string
	name string
	// programName is the agent's command-line tool, found on the login shell's PATH and then in locations.
	programName string
	locations   []string
	// directory is where the tool runs: outside any project, so a project's settings do not change what it reads.
	directory string
	connector connector
	// mutex keeps two clicks from changing the agent's settings at once: omp writes its plugin list without a lock.
	mutex sync.Mutex
}

type Config struct {
	HomeDirectory string
	// GetUserPath returns the login shell's PATH, which an app started from Finder does not get.
	GetUserPath func() []string
	// GetUserVariable returns one of ListUserVariables as the login shell has it, empty when unset.
	GetUserVariable func(name string) string
	// OmpPluginPath is the plugin folder Connect links for omp; empty when this copy of Hisar has none.
	OmpPluginPath string
	// HisarPath is the hisar that Claude Code, Codex and Hermes start as their MCP server: Hisar.app's
	// Contents/Helpers/hisar, or on Linux the package's. A development build outside Hisar.app (desktop/bin/Hisar) looks
	// for desktop/Helpers/hisar and refuses to connect them without it; `make package-desktop` builds desktop/bin/Hisar.app
	// with its hisar.
	HisarPath string
}

// Service is bound to the UI.
type Service struct {
	config   Config
	timeout  time.Duration
	allAgent []*agentKind
}

func NewService(config Config) *Service {
	return newService(config, "/")
}

// newService takes the root that the agents' system-wide install locations are under, so tests never find the
// agents installed on the machine that runs them.
func newService(config Config, rootDirectory string) *Service {
	home := config.HomeDirectory
	inRoot := func(path string) string { return filepath.Join(rootDirectory, path) }
	if config.GetUserVariable == nil {
		config.GetUserVariable = os.Getenv
	}
	return &Service{config: config, timeout: programTimeout, allAgent: []*agentKind{
		{
			id: "omp", name: "omp", programName: "omp",
			locations: ompLocations(home),
			directory: "/",
			connector: ompConnector{pluginPath: config.OmpPluginPath},
		},
		{
			id: "claude-code", name: "Claude Code", programName: "claude",
			locations: []string{
				filepath.Join(home, ".local", "bin", "claude"),
				inRoot("opt/homebrew/bin/claude"),
				inRoot("usr/local/bin/claude"),
				filepath.Join(home, ".claude", "local", "claude"),
			},
			directory: home,
			connector: claudeCodeConnector{hisarPath: config.HisarPath, homeDirectory: home},
		},
		{
			id: "codex", name: "Codex", programName: "codex",
			locations: []string{
				inRoot("opt/homebrew/bin/codex"),
				inRoot("usr/local/bin/codex"),
				filepath.Join(home, ".npm-global", "bin", "codex"),
				filepath.Join(home, ".local", "bin", "codex"),
			},
			directory: home,
			connector: codexConnector{hisarPath: config.HisarPath, homeDirectory: home},
		},
		{
			id: "hermes", name: "Hermes", programName: "hermes",
			locations: []string{
				filepath.Join(home, ".local", "bin", "hermes"),
				filepath.Join(home, ".hermes", "hermes-agent", ".hermes", "bin", "hermes"),
				inRoot("opt/homebrew/bin/hermes"),
				inRoot("usr/local/bin/hermes"),
			},
			directory: home,
			connector: hermesConnector{
				server:        hermesServer{hisarPath: config.HisarPath, socketPath: getCustomSocketPath(home)},
				getHermesHome: func() string { return config.GetUserVariable(hermesHomeVariable) },
			},
		},
	}}
}

func getCustomSocketPath(home string) string {
	path := os.Getenv(cliprotocol.SocketEnvironment)
	if path == cliprotocol.GetDefaultSocketPath(home) {
		return ""
	}
	return path
}

// ListAgents reads every agent at once: each runs its own command-line tool.
func (service *Service) ListAgents() []Agent {
	allAgent := make([]Agent, len(service.allAgent))
	var group sync.WaitGroup
	for i, kind := range service.allAgent {
		group.Go(func() {
			kind.mutex.Lock()
			defer kind.mutex.Unlock()
			allAgent[i] = service.describe(kind)
		})
	}
	group.Wait()
	return allAgent
}

// Connect adds Hisar to the agent, so every session it starts afterwards loads Hisar.
func (service *Service) Connect(id string) (Agent, error) {
	kind, err := service.findKind(id)
	if err != nil {
		return Agent{}, err
	}
	kind.mutex.Lock()
	defer kind.mutex.Unlock()
	agent, isFound := service.findProgram(kind)
	if !isFound {
		return Agent{}, fmt.Errorf("%s is not installed", kind.name)
	}
	if err := kind.connector.connect(agent); err != nil {
		if errors.Is(err, errConflict) {
			return Agent{}, errors.New(describeConflict(kind))
		}
		return Agent{}, err
	}
	return service.describe(kind), nil
}

// Disconnect removes only what Hisar added, and succeeds when that is already gone.
func (service *Service) Disconnect(id string) (Agent, error) {
	kind, err := service.findKind(id)
	if err != nil {
		return Agent{}, err
	}
	kind.mutex.Lock()
	defer kind.mutex.Unlock()
	if agent, isFound := service.findProgram(kind); isFound {
		if err := kind.connector.disconnect(agent); err != nil {
			return Agent{}, err
		}
	}
	return service.describe(kind), nil
}

func (service *Service) findKind(id string) (*agentKind, error) {
	index := slices.IndexFunc(service.allAgent, func(kind *agentKind) bool { return kind.id == id })
	if index < 0 {
		return nil, fmt.Errorf("Hisar cannot connect a coding agent named %q", id)
	}
	return service.allAgent[index], nil
}

func (service *Service) findProgram(kind *agentKind) (program, bool) {
	userPath := service.config.GetUserPath()
	path, isFound := findProgram(kind.programName, userPath, kind.locations)
	return program{name: kind.name, path: path, userPath: userPath, directory: kind.directory, timeout: service.timeout}, isFound
}

func (service *Service) describe(kind *agentKind) Agent {
	agent := Agent{ID: kind.id, Name: kind.name, State: StateNotInstalled}
	program, isFound := service.findProgram(kind)
	if !isFound {
		return agent
	}
	state, err := kind.connector.getState(program)
	switch {
	case err != nil:
		agent.State, agent.Detail = StateUnknown, err.Error()
	case state == StateConflict:
		agent.State, agent.Detail = state, describeConflict(kind)
	default:
		agent.State = state
	}
	return agent
}

func describeConflict(kind *agentKind) string {
	return fmt.Sprintf("%s already has a server named hisar. Remove it to connect Hisar.", kind.name)
}

// requireHisar refuses to point an agent at a hisar that is not there.
func requireHisar(hisarPath string) error {
	if hisarPath == "" || !isExecutable(hisarPath) {
		return errors.New("this copy of Hisar does not include the hisar command-line tool")
	}
	return nil
}

// FindOmpPlugin returns Hisar.app's Contents/Resources/omp-plugin, a Linux package's share/hisar/omp-plugin beside its
// bin (/usr/share/hisar/omp-plugin for /usr/bin/Hisar), or for a development build the repository's omp folder above
// the executable.
func FindOmpPlugin(executableDirectory string) string {
	for _, packaged := range []string{
		filepath.Join(filepath.Dir(executableDirectory), "Resources", "omp-plugin"),
		filepath.Join(filepath.Dir(executableDirectory), "share", "hisar", "omp-plugin"),
	} {
		if isFile(filepath.Join(packaged, "package.json")) {
			return packaged
		}
	}
	for directory := executableDirectory; ; directory = filepath.Dir(directory) {
		if candidate := filepath.Join(directory, "omp"); isFile(filepath.Join(candidate, "package.json")) {
			return candidate
		}
		if filepath.Dir(directory) == directory {
			return ""
		}
	}
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
