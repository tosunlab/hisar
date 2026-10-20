package harness

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const codexConflict = "Codex already has a server named hisar. Remove it to connect Hisar."

func (world *fakeWorld) codexConfigPath() string { return world.pathInHome(".codex", "config.toml") }

func (world *fakeWorld) ourCodexTable() string {
	return strings.Replace(hisarTable, testHisarPath, world.hisarPath, 1)
}

func TestFindCodexWhereItsInstallersPutIt(t *testing.T) {
	for _, test := range []struct {
		name     string
		location func(*fakeWorld) string
	}{
		{"the login shell's PATH", func(world *fakeWorld) string { return world.pathOnUserPath("codex") }},
		{"Homebrew", func(world *fakeWorld) string { return world.pathInRoot("opt", "homebrew", "bin", "codex") }},
		{"npm in /usr/local", func(world *fakeWorld) string { return world.pathInRoot("usr", "local", "bin", "codex") }},
		{"npm in the home folder", func(world *fakeWorld) string { return world.pathInHome(".npm-global", "bin", "codex") }},
		{"~/.local/bin", func(world *fakeWorld) string { return world.pathInHome(".local", "bin", "codex") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			world := newFakeWorld(t)
			world.installCodex(test.location(world))
			if agent := getAgent(t, world.newService(""), "codex"); agent.State != StateAvailable {
				t.Fatalf("Codex = %+v; want available", agent)
			}
			if calls := world.readCalls(); !slices.Equal(calls, []string{test.location(world) + " mcp get hisar --json"}) {
				t.Fatalf("codex calls = %q; want one mcp get by %s", calls, test.location(world))
			}
		})
	}
}

func TestConnectCodexAddsItsTableAndKeepsTheRest(t *testing.T) {
	world := newFakeWorld(t)
	codexPath := world.pathOnUserPath("codex")
	world.installCodex(codexPath)
	userConfig := "# mine\nmodel = \"o4\"\n\n[mcp_servers.github]\ncommand = \"gh\"\n"
	world.writeFile(world.codexConfigPath(), userConfig, 0o640)
	service := world.newService("")
	agent, err := service.Connect("codex")
	if err != nil || agent.State != StateConnected {
		t.Fatalf("Connect() = %+v, %v; want connected", agent, err)
	}
	if got := world.readFile(world.codexConfigPath()); got != userConfig+"\n"+world.ourCodexTable() {
		t.Fatalf("config.toml =\n%s", got)
	}
	if info, err := os.Stat(world.codexConfigPath()); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("config.toml mode = %v, %v; want 0640", info.Mode(), err)
	}
	if entries, _ := os.ReadDir(filepath.Dir(world.codexConfigPath())); len(entries) != 1 {
		t.Fatalf("Codex's folder holds %v; want only config.toml", entries)
	}
	if agent, err := service.Connect("codex"); err != nil || agent.State != StateConnected {
		t.Fatalf("second Connect() = %+v, %v", agent, err)
	}
	get := codexPath + " mcp get hisar --json"
	if calls := world.readCalls(); !slices.Equal(calls, []string{get, get, get, get, get}) {
		t.Fatalf("codex calls = %q; want only mcp get", calls)
	}
}

func TestConnectCodexCreatesItsConfig(t *testing.T) {
	world := newFakeWorld(t)
	world.installCodex(world.pathOnUserPath("codex"))
	codexHome := filepath.Join(t.TempDir(), "codex home")
	t.Setenv("CODEX_HOME", codexHome)
	if agent, err := world.newService("").Connect("codex"); err != nil || agent.State != StateConnected {
		t.Fatalf("Connect() = %+v, %v; want connected", agent, err)
	}
	path := filepath.Join(codexHome, "config.toml")
	if got := world.readFile(path); got != world.ourCodexTable() {
		t.Fatalf("config.toml =\n%s", got)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("config.toml mode = %v, %v; want 0600", info.Mode(), err)
	}
}

func TestConnectCodexKeepsALinkedConfig(t *testing.T) {
	world := newFakeWorld(t)
	world.installCodex(world.pathOnUserPath("codex"))
	target := filepath.Join(t.TempDir(), "dotfiles", "codex.toml")
	world.writeFile(target, "model = \"o4\"\n", 0o600)
	if err := os.MkdirAll(filepath.Dir(world.codexConfigPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, world.codexConfigPath()); err != nil {
		t.Fatal(err)
	}
	if _, err := world.newService("").Connect("codex"); err != nil {
		t.Fatal(err)
	}
	if link, err := os.Readlink(world.codexConfigPath()); err != nil || link != target {
		t.Fatalf("config.toml links to %q, %v; want %q", link, err, target)
	}
	if got := world.readFile(target); got != "model = \"o4\"\n\n"+world.ourCodexTable() {
		t.Fatalf("linked config =\n%s", got)
	}
}

func TestConnectCodexLeavesAnotherHisarServer(t *testing.T) {
	world := newFakeWorld(t)
	codexPath := world.pathOnUserPath("codex")
	world.installCodex(codexPath)
	userConfig := "[mcp_servers.hisar]\ncommand = \"hisar\"\nargs = [\"mcp\"]\n"
	world.writeFile(world.codexConfigPath(), userConfig, 0o600)
	service := world.newService("")
	if agent := getAgent(t, service, "codex"); agent.State != StateConflict || agent.Detail != codexConflict {
		t.Fatalf("Codex = %+v; want the conflict", agent)
	}
	if _, err := service.Connect("codex"); err == nil || err.Error() != codexConflict {
		t.Fatalf("Connect() error = %v; want %q", err, codexConflict)
	}
	if agent, err := service.Disconnect("codex"); err != nil || agent.State != StateConflict {
		t.Fatalf("Disconnect() = %+v, %v; want the conflict left alone", agent, err)
	}
	if got := world.readFile(world.codexConfigPath()); got != userConfig {
		t.Fatalf("config.toml = %s; want it unchanged", got)
	}
	if calls := world.readCalls(); slices.ContainsFunc(calls, func(call string) bool { return !strings.HasSuffix(call, " mcp get hisar --json") }) {
		t.Fatalf("codex calls = %q; want only mcp get", calls)
	}
}

// TestCodexLeavesACustomizedHisarServer: Hisar owns its table only as Connect writes it. A table with Hisar's command and
// arguments and anything else that changes how Codex runs it is the user's, so Disconnect never removes it.
func TestCodexLeavesACustomizedHisarServer(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(table string) string
	}{
		{"environment variables passed through", func(table string) string { return table + "env_vars = [\"CUSTOM_VAR\"]\n" }},
		{"a working folder", func(table string) string { return table + "cwd = \"/tmp\"\n" }},
		{"tools turned on", func(table string) string { return table + "enabled_tools = [\"run_with_secrets\"]\n" }},
		{"tools turned off", func(table string) string { return table + "disabled_tools = [\"list_vaults\"]\n" }},
		{"turned off", func(table string) string { return table + "enabled = false\n" }},
		{"required", func(table string) string { return table + "required = true\n" }},
		{"another tool timeout", func(table string) string {
			return strings.Replace(table, "tool_timeout_sec = 4200", "tool_timeout_sec = 60", 1)
		}},
		{"no startup timeout", func(table string) string { return strings.Replace(table, "startup_timeout_sec = 15\n", "", 1) }},
		{"environment", func(table string) string { return table + "\n[mcp_servers.hisar.env]\nA = \"b\"\n" }},
		{"a tool's settings", func(table string) string {
			return table + "\n[mcp_servers.hisar.tools.run_with_secrets]\napproval_mode = \"approve\"\n"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			world := newFakeWorld(t)
			world.installCodex(world.pathOnUserPath("codex"))
			userConfig := "model = \"o4\"\n\n" + test.change(world.ourCodexTable())
			world.writeFile(world.codexConfigPath(), userConfig, 0o600)
			service := world.newService("")
			if agent := getAgent(t, service, "codex"); agent.State != StateConflict || agent.Detail != codexConflict {
				t.Fatalf("Codex = %+v; want the conflict", agent)
			}
			if _, err := service.Connect("codex"); err == nil || err.Error() != codexConflict {
				t.Fatalf("Connect() error = %v; want %q", err, codexConflict)
			}
			if agent, err := service.Disconnect("codex"); err != nil || agent.State != StateConflict {
				t.Fatalf("Disconnect() = %+v, %v; want the conflict left alone", agent, err)
			}
			if got := world.readFile(world.codexConfigPath()); got != userConfig {
				t.Fatalf("config.toml =\n%s\nwant it unchanged", got)
			}
			if calls := world.readCalls(); slices.ContainsFunc(calls, func(call string) bool { return !strings.HasSuffix(call, " mcp get hisar --json") }) {
				t.Fatalf("codex calls = %q; want only mcp get", calls)
			}
		})
	}
}

func TestConnectCodexTakesItsTableOutWhenCodexReadsItWrong(t *testing.T) {
	world := newFakeWorld(t)
	world.installCodex(world.pathOnUserPath("codex"))
	userConfig := "model = \"o4\"\n"
	world.writeFile(world.codexConfigPath(), userConfig, 0o600)
	world.setState("broken-hisar", "")
	_, err := world.newService("").Connect("codex")
	if err == nil || !strings.HasPrefix(err.Error(), "Codex did not accept the server Hisar added, so Hisar took it out again") {
		t.Fatalf("Connect() error = %v", err)
	}
	if got := world.readFile(world.codexConfigPath()); got != userConfig {
		t.Fatalf("config.toml = %s; want it as it was", got)
	}
}

func TestConnectCodexRefusesAConfigItCannotRead(t *testing.T) {
	world := newFakeWorld(t)
	world.installCodex(world.pathOnUserPath("codex"))
	// The fake Codex reads the file with Hisar's own parser, so a file it cannot read is also one Codex says is broken.
	world.writeFile(world.codexConfigPath(), "model = [\n", 0o600)
	service := world.newService("")
	if agent := getAgent(t, service, "codex"); agent.State != StateUnknown || agent.Detail != "Codex could not list its MCP servers: failed to load configuration" {
		t.Fatalf("Codex = %+v", agent)
	}
	if _, err := service.Connect("codex"); err == nil {
		t.Fatal("Connect() succeeded with a broken config.toml")
	}
}

func TestReplaceConfigKeepsAChangeMadeMeanwhile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("model = \"o3\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	read, err := readConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("model = \"o4\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := replaceConfig(path, read, []byte("hisar\n")); !errors.Is(err, errCodexChanged) {
		t.Fatalf("replaceConfig() error = %v; want errCodexChanged", err)
	}
	if content, _ := os.ReadFile(path); string(content) != "model = \"o4\"\n" {
		t.Fatalf("config.toml = %s; want the other change", content)
	}
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
		t.Fatalf("folder holds %v; want only config.toml", entries)
	}

	created := filepath.Join(t.TempDir(), "codex", "config.toml")
	missing, err := readConfig(created)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(created), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(created, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := replaceConfig(created, missing, []byte("hisar\n")); !errors.Is(err, errCodexChanged) {
		t.Fatalf("replaceConfig() over a file created meanwhile error = %v; want errCodexChanged", err)
	}
}

func TestDisconnectCodexRemovesHisarsServer(t *testing.T) {
	world := newFakeWorld(t)
	codexPath := world.pathOnUserPath("codex")
	world.installCodex(codexPath)
	world.writeFile(world.codexConfigPath(), "model = \"o4\"\n", 0o600)
	service := world.newService("")
	if _, err := service.Connect("codex"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if agent, err := service.Disconnect("codex"); err != nil || agent.State != StateAvailable {
			t.Fatalf("Disconnect() = %+v, %v; want available", agent, err)
		}
	}
	remove := codexPath + " mcp remove hisar"
	if calls := world.readCalls(); len(slices.DeleteFunc(slices.Clone(calls), func(call string) bool { return call != remove })) != 1 {
		t.Fatalf("codex calls = %q; want one mcp remove hisar", calls)
	}
}

func TestListAgentsNamesEveryAgent(t *testing.T) {
	world := newFakeWorld(t)
	got := world.newService("").ListAgents()
	want := []Agent{
		{ID: "omp", Name: "omp", State: StateNotInstalled},
		{ID: "claude-code", Name: "Claude Code", State: StateNotInstalled},
		{ID: "codex", Name: "Codex", State: StateNotInstalled},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("ListAgents() = %+v; want %+v", got, want)
	}
	if _, err := world.newService("").Connect("cursor"); err == nil {
		t.Fatal("Connect(cursor) succeeded")
	}
}
