package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// fakeOmpScript plays omp's plugin manager: it logs each call and keeps the linked plugin in a file shared by every fake
// omp of a test, so the tests can tell which omp ran.
const fakeOmpScript = `#!/bin/sh
printf '%%s %%s\n' "$0" "$*" >> '%[1]s'
case "$1 $2" in
"plugin list")
  if [ -f '%[2]s/hang' ]; then exec sleep 10; fi
  if [ -f '%[2]s/garbled' ]; then echo 'Loading plugins'; exit 0; fi
  if [ -f '%[2]s/linked' ]; then
    printf '{"npm":[{"name":"@tryhisar/omp-plugin","version":"0.1.0","enabled":%%s}],"marketplace":[]}' "$(cat '%[2]s/linked')"
  else
    printf '{"npm":[{"name":"omp-other-plugin","version":"1.0.0","enabled":true}],"marketplace":[]}'
  fi
  ;;
"plugin link")
  if [ ! -f "$3/package.json" ]; then echo "✘ Failed to link: Error: package.json not found at $3" >&2; exit 1; fi
  echo true > '%[2]s/linked'
  echo "✔ Linked @tryhisar/omp-plugin from $3"
  ;;
"plugin uninstall")
  if [ ! -f '%[2]s/linked' ]; then echo "✘ $3 is not installed" >&2; exit 1; fi
  rm '%[2]s/linked'
  echo "✔ Uninstalled $3"
  ;;
esac
`

// fakeClaudeScript plays `claude mcp add` and `claude mcp remove` on .claude.json in $CLAUDE_CONFIG_DIR or the
// world's home folder, logging each call with the folder it ran in.
const fakeClaudeScript = `#!/bin/sh
printf '%%s %%s (in %%s)\n' "$0" "$*" "$(pwd)" >> '%[1]s'
settings="${CLAUDE_CONFIG_DIR:-%[2]s}/.claude.json"
case "$1 $2" in
"mcp add")
  if grep -q '"hisar"' "$settings" 2>/dev/null; then echo "MCP server hisar already exists in user config" >&2; exit 1; fi
  shift 8
  command="$1"; shift
  args=""
  for argument in "$@"; do args="$args${args:+,}\"$argument\""; done
  printf '{"numStartups":3,"mcpServers":{"hisar":{"type":"stdio","command":"%%s","args":[%%s],"env":{}}}}' "$command" "$args" > "$settings"
  echo "Added stdio MCP server hisar to user config"
  ;;
"mcp remove")
  printf '{"numStartups":3,"mcpServers":{}}' > "$settings"
  echo "Removed MCP server hisar from user config"
  ;;
esac
`

// fakeCodexScript logs each call and runs this test binary as Codex (see TestMain).
const fakeCodexScript = `#!/bin/sh
printf '%%s %%s\n' "$0" "$*" >> '%[1]s'
HISAR_FAKE_CODEX=1 HISAR_FAKE_HOME='%[2]s' HISAR_FAKE_STATE='%[3]s' exec '%[4]s' "$@"
`

func TestMain(m *testing.M) {
	if os.Getenv("HISAR_FAKE_CODEX") == "1" {
		os.Exit(runFakeCodex(os.Args[1:]))
	}
	os.Exit(m.Run())
}

// runFakeCodex answers `codex mcp get hisar --json` in the shape Codex 0.162.1 does and `codex mcp remove hisar`, from
// config.toml. With the state file broken-hisar it reports Hisar's server with another command, as a Codex that read the
// table differently would.
func runFakeCodex(arguments []string) int {
	directory := os.Getenv("CODEX_HOME")
	if directory == "" {
		directory = filepath.Join(os.Getenv("HISAR_FAKE_HOME"), ".codex")
	}
	path := filepath.Join(directory, "config.toml")
	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "Error: failed to read configuration")
		return 1
	}
	statements, err := parseTOML(content)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: failed to load configuration")
		return 1
	}
	start, end, values, hasEnv := -1, 0, map[string]string{}, false
	isInTable := false
	for _, statement := range statements {
		if statement.isHeader {
			isInTable = strings.Join(statement.path, ".") == "mcp_servers.hisar"
			if isInTable {
				start = statement.start
			}
			if strings.Join(statement.path, ".") == "mcp_servers.hisar.env" {
				hasEnv = true
			}
		}
		if isInTable {
			end = statement.end
			if !statement.isHeader {
				values[statement.path[2]] = statement.value
			}
		}
	}
	if start < 0 {
		fmt.Fprintln(os.Stderr, "Error: No MCP server named 'hisar' found.")
		return 1
	}
	switch strings.Join(arguments, " ") {
	case "mcp get hisar --json":
		command, _ := parseTOMLString(values["command"])
		if _, err := os.Stat(filepath.Join(os.Getenv("HISAR_FAKE_STATE"), "broken-hisar")); err == nil {
			command = "/usr/bin/false"
		}
		readArray := func(key string) []string {
			array, _ := parseTOMLStringArray(values[key])
			return array
		}
		readNumber := func(key string) *float64 {
			number, err := strconv.ParseFloat(values[key], 64)
			if err != nil {
				return nil
			}
			return &number
		}
		var cwd *string
		if value, isString := parseTOMLString(values["cwd"]); isString {
			cwd = &value
		}
		var env map[string]string
		if hasEnv {
			env = map[string]string{"A": "b"}
		}
		output, _ := json.Marshal(map[string]any{
			"name": "hisar", "enabled": values["enabled"] != "false", "disabled_reason": nil,
			"transport": map[string]any{
				"type": "stdio", "command": command, "args": append([]string{}, readArray("args")...), "env": env,
				"env_vars": append([]string{}, readArray("env_vars")...), "cwd": cwd,
			},
			"enabled_tools": readArray("enabled_tools"), "disabled_tools": readArray("disabled_tools"),
			"startup_timeout_sec": readNumber("startup_timeout_sec"), "tool_timeout_sec": readNumber("tool_timeout_sec"),
		})
		fmt.Println(string(output))
	case "mcp remove hisar":
		if err := os.WriteFile(path, append(content[:start:start], content[end:]...), 0o600); err != nil {
			return 1
		}
		fmt.Println("Removed global MCP server 'hisar'.")
	}
	return 0
}

type fakeWorld struct {
	t              *testing.T
	rootDirectory  string
	homeDirectory  string
	userPath       []string
	stateDirectory string
	logPath        string
	hisarPath      string
}

func newFakeWorld(t *testing.T) *fakeWorld {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")
	// Resolved, so the folder a fake agent prints as its own is the same path.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	world := &fakeWorld{
		t:              t,
		rootDirectory:  root,
		homeDirectory:  filepath.Join(root, "home"),
		userPath:       []string{filepath.Join(root, "usr-bin"), "relative/bin", "/usr/bin", "/bin"},
		stateDirectory: filepath.Join(root, "state"),
		logPath:        filepath.Join(root, "calls.log"),
		hisarPath:      filepath.Join(root, "Applications", "Hisar.app", "Contents", "Helpers", "hisar"),
	}
	for _, directory := range []string{world.homeDirectory, world.userPath[0], world.stateDirectory} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	world.writeFile(world.hisarPath, "#!/bin/sh\n", 0o755)
	return world
}

func (world *fakeWorld) pathOnUserPath(name string) string {
	return filepath.Join(world.userPath[0], name)
}

func (world *fakeWorld) pathInHome(parts ...string) string {
	return filepath.Join(append([]string{world.homeDirectory}, parts...)...)
}

func (world *fakeWorld) pathInRoot(parts ...string) string {
	return filepath.Join(append([]string{world.rootDirectory}, parts...)...)
}

func (world *fakeWorld) installOmp(path string) {
	world.t.Helper()
	world.writeFile(path, fmt.Sprintf(fakeOmpScript, world.logPath, world.stateDirectory), 0o755)
}

func (world *fakeWorld) installClaude(path string) {
	world.t.Helper()
	world.writeFile(path, fmt.Sprintf(fakeClaudeScript, world.logPath, world.homeDirectory), 0o755)
}

func (world *fakeWorld) installCodex(path string) {
	world.t.Helper()
	testBinary, err := os.Executable()
	if err != nil {
		world.t.Fatal(err)
	}
	world.writeFile(path, fmt.Sprintf(fakeCodexScript, world.logPath, world.homeDirectory, world.stateDirectory, testBinary), 0o755)
}

func (world *fakeWorld) writeFile(path, content string, mode os.FileMode) {
	world.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		world.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		world.t.Fatal(err)
	}
}

func (world *fakeWorld) readFile(path string) string {
	world.t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		world.t.Fatal(err)
	}
	return string(content)
}

func (world *fakeWorld) setState(name, content string) {
	world.t.Helper()
	world.writeFile(filepath.Join(world.stateDirectory, name), content, 0o644)
}

func (world *fakeWorld) newService(pluginPath string) *Service {
	return newService(Config{
		HomeDirectory: world.homeDirectory,
		GetUserPath:   func() []string { return world.userPath },
		OmpPluginPath: pluginPath,
		HisarPath:     world.hisarPath,
	}, world.rootDirectory)
}

// newPlugin returns a plugin folder with a package.json, like Hisar.app's Contents/Resources/omp-plugin.
func (world *fakeWorld) newPlugin() string {
	world.t.Helper()
	pluginPath := filepath.Join(world.t.TempDir(), "omp-plugin")
	world.writeFile(filepath.Join(pluginPath, "package.json"), `{"name":"@tryhisar/omp-plugin"}`, 0o644)
	return pluginPath
}

// readCalls returns each agent run as "<path> <arguments>".
func (world *fakeWorld) readCalls() []string {
	world.t.Helper()
	content, err := os.ReadFile(world.logPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		world.t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
}

// getAgent returns one agent of ListAgents.
func getAgent(t *testing.T, service *Service, id string) Agent {
	t.Helper()
	for _, agent := range service.ListAgents() {
		if agent.ID == id {
			return agent
		}
	}
	t.Fatalf("ListAgents() has no %q", id)
	return Agent{}
}
