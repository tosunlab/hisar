package harness

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/cliprotocol"
)

const hermesConflict = "Hermes already has a server named hisar. Remove it to connect Hisar."

func (world *fakeWorld) hermesConfigPath() string { return world.pathInHome(".hermes", "config.yaml") }

func (world *fakeWorld) ourHermesEntry() string {
	return "mcp_servers:\n" + strings.Replace(hermesEntryText, testHisarPath, world.hisarPath, 1)
}

func TestFindHermesWhereItsInstallerPutsIt(t *testing.T) {
	for _, test := range []struct {
		name     string
		location func(*fakeWorld) string
	}{
		{"the login shell's PATH", func(world *fakeWorld) string { return world.pathOnUserPath("hermes") }},
		{"~/.local/bin", func(world *fakeWorld) string { return world.pathInHome(".local", "bin", "hermes") }},
		{"the launcher in its checkout", func(world *fakeWorld) string {
			return world.pathInHome(".hermes", "hermes-agent", ".hermes", "bin", "hermes")
		}},
		{"Homebrew", func(world *fakeWorld) string { return world.pathInRoot("opt", "homebrew", "bin", "hermes") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			world := newFakeWorld(t)
			world.installHermes(test.location(world))
			if agent := getAgent(t, world.newService(""), "hermes"); agent != (Agent{ID: "hermes", Name: "Hermes", State: StateAvailable}) {
				t.Fatalf("Hermes = %+v; want available", agent)
			}
			if calls := world.readCalls(); !slices.Equal(calls, []string{test.location(world) + " config path (HERMES_HOME=)"}) {
				t.Fatalf("hermes calls = %q; want one config path by %s", calls, test.location(world))
			}
		})
	}
}

func TestConnectHermesAddsItsEntryAndKeepsTheRest(t *testing.T) {
	world := newFakeWorld(t)
	hermesPath := world.pathOnUserPath("hermes")
	world.installHermes(hermesPath)
	userConfig, err := os.ReadFile("testdata/hermes/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	world.writeFile(world.hermesConfigPath(), string(userConfig), 0o640)
	service := world.newService("")
	if agent, err := service.Connect("hermes"); err != nil || agent.State != StateConnected {
		t.Fatalf("Connect() = %+v, %v; want connected", agent, err)
	}
	if got := world.readFile(world.hermesConfigPath()); got != string(userConfig)+"\n"+world.ourHermesEntry() {
		t.Fatalf("config.yaml ends with %q", got[len(userConfig)-20:])
	}
	if info, err := os.Stat(world.hermesConfigPath()); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("config.yaml mode = %v, %v; want 0640", info.Mode(), err)
	}
	if entries, _ := os.ReadDir(filepath.Dir(world.hermesConfigPath())); len(entries) != 1 {
		t.Fatalf("Hermes' folder holds %v; want only config.yaml", entries)
	}
	if agent, err := service.Connect("hermes"); err != nil || agent.State != StateConnected {
		t.Fatalf("second Connect() = %+v, %v", agent, err)
	}
	path, get := hermesPath+" config path (HERMES_HOME=)", hermesPath+" config get mcp_servers.hisar --json --raw (HERMES_HOME=)"
	if calls := world.readCalls(); !slices.Equal(calls, []string{path, get, path, path, path}) {
		t.Fatalf("hermes calls = %q; want Hermes asked once for the entry", calls)
	}
	for range 2 {
		if agent, err := service.Disconnect("hermes"); err != nil || agent.State != StateAvailable {
			t.Fatalf("Disconnect() = %+v, %v; want available", agent, err)
		}
	}
	if got := world.readFile(world.hermesConfigPath()); got != string(userConfig) {
		t.Fatal("config.yaml after Disconnect differs from the file before Connect")
	}
}

func TestConnectHermesCreatesItsConfigInTheLoginShellsHome(t *testing.T) {
	world := newFakeWorld(t)
	hermesPath := world.pathOnUserPath("hermes")
	world.installHermes(hermesPath)
	hermesHome := filepath.Join(t.TempDir(), "hermes home")
	world.userVariables = map[string]string{"HERMES_HOME": hermesHome}
	if agent, err := world.newService("").Connect("hermes"); err != nil || agent.State != StateConnected {
		t.Fatalf("Connect() = %+v, %v; want connected", agent, err)
	}
	path := filepath.Join(hermesHome, "config.yaml")
	if got := world.readFile(path); got != world.ourHermesEntry() {
		t.Fatalf("config.yaml =\n%s", got)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("config.yaml mode = %v, %v; want 0600", info.Mode(), err)
	}
	if _, err := os.Stat(world.hermesConfigPath()); !os.IsNotExist(err) {
		t.Fatalf("~/.hermes/config.yaml: %v; want none", err)
	}
	if calls := world.readCalls(); !strings.HasSuffix(calls[0], "config path (HERMES_HOME="+hermesHome+")") {
		t.Fatalf("hermes calls = %q; want the login shell's HERMES_HOME", calls)
	}
}

func TestConnectHermesIgnoresTheAppsHermesHome(t *testing.T) {
	for _, test := range []struct {
		name, shellsHome string
	}{
		{"changed in the login shell", filepath.Join(t.TempDir(), "hermes home")},
		{"unset in the login shell", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			world := newFakeWorld(t)
			world.installHermes(world.pathOnUserPath("hermes"))
			staleHome := t.TempDir()
			t.Setenv("HERMES_HOME", staleHome)
			shellsHome := test.shellsHome
			world.userVariables = map[string]string{"HERMES_HOME": shellsHome}
			if _, err := world.newService("").Connect("hermes"); err != nil {
				t.Fatal(err)
			}
			path := world.hermesConfigPath()
			if shellsHome != "" {
				path = filepath.Join(shellsHome, "config.yaml")
			}
			if got := world.readFile(path); got != world.ourHermesEntry() {
				t.Fatalf("%s =\n%s", path, got)
			}
			if entries, _ := os.ReadDir(staleHome); len(entries) != 0 {
				t.Fatalf("the app's HERMES_HOME holds %v; want nothing", entries)
			}
		})
	}
}

func TestConnectHermesKeepsALinkedConfig(t *testing.T) {
	world := newFakeWorld(t)
	world.installHermes(world.pathOnUserPath("hermes"))
	target := filepath.Join(t.TempDir(), "dotfiles", "hermes.yaml")
	world.writeFile(target, "model:\n  default: x\n", 0o600)
	if err := os.MkdirAll(filepath.Dir(world.hermesConfigPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, world.hermesConfigPath()); err != nil {
		t.Fatal(err)
	}
	service := world.newService("")
	if _, err := service.Connect("hermes"); err != nil {
		t.Fatal(err)
	}
	if link, err := os.Readlink(world.hermesConfigPath()); err != nil || link != target {
		t.Fatalf("config.yaml links to %q, %v; want %q", link, err, target)
	}
	if got := world.readFile(target); got != "model:\n  default: x\n\n"+world.ourHermesEntry() {
		t.Fatalf("linked config =\n%s", got)
	}
	if _, err := service.Disconnect("hermes"); err != nil {
		t.Fatal(err)
	}
	if link, err := os.Readlink(world.hermesConfigPath()); err != nil || link != target || world.readFile(target) != "model:\n  default: x\n" {
		t.Fatalf("after Disconnect config.yaml links to %q, %v, and holds %q", link, err, world.readFile(target))
	}
}

func TestConnectHermesLeavesAnotherHisarServer(t *testing.T) {
	for _, test := range []struct {
		name, userConfig string
	}{
		{"written in mcp_servers", "mcp_servers:\n  hisar:\n    command: hisar\n    args: [mcp]\n"},
		{"merged into mcp_servers", "defaults: &servers\n  hisar:\n    command: user-owned\n    args: [mcp]\nmcp_servers:\n  <<: *servers\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			world := newFakeWorld(t)
			world.installHermes(world.pathOnUserPath("hermes"))
			world.writeFile(world.hermesConfigPath(), test.userConfig, 0o600)
			service := world.newService("")
			if agent := getAgent(t, service, "hermes"); agent.State != StateConflict || agent.Detail != hermesConflict {
				t.Fatalf("Hermes = %+v; want the conflict", agent)
			}
			if _, err := service.Connect("hermes"); err == nil || err.Error() != hermesConflict {
				t.Fatalf("Connect() error = %v; want %q", err, hermesConflict)
			}
			if agent, err := service.Disconnect("hermes"); err != nil || agent.State != StateConflict {
				t.Fatalf("Disconnect() = %+v, %v; want the conflict left alone", agent, err)
			}
			if got := world.readFile(world.hermesConfigPath()); got != test.userConfig {
				t.Fatalf("config.yaml = %s; want it as it was", got)
			}
		})
	}
}

func TestConnectHermesRefusesAConfigItCannotRead(t *testing.T) {
	for _, test := range []struct {
		name       string
		userConfig func(world *fakeWorld) string
		wantErr    error
	}{
		{"servers in flow style", func(*fakeWorld) string { return "mcp_servers: {time: {command: uvx}}\n" }, errHermesLayout},
		{"mcp_servers twice, Hisar's entry in the first", func(world *fakeWorld) string {
			return world.ourHermesEntry() + "mcp_servers:\n  hisar:\n    command: user-owned\n"
		}, errHermesDuplicateKey},
		{"a key written as an alias of an earlier key, Hisar's entry after it", func(world *fakeWorld) string {
			return "&setting model: first\n*setting: second\n" + world.ourHermesEntry()
		}, errHermesAliasKey},
		{"an alias as a key in a nested mapping", func(world *fakeWorld) string {
			return "model:\n  &setting default: a\n  fallback:\n    *setting: b\n" + world.ourHermesEntry()
		}, errHermesAliasKey},
		{"a top level that merges another mapping", func(world *fakeWorld) string {
			return "base: &base\n  mcp_servers:\n    hisar:\n      command: user-owned\n<<: *base\n" + world.ourHermesEntry()
		}, errHermesLayout},
	} {
		t.Run(test.name, func(t *testing.T) {
			world := newFakeWorld(t)
			world.installHermes(world.pathOnUserPath("hermes"))
			userConfig := test.userConfig(world)
			world.writeFile(world.hermesConfigPath(), userConfig, 0o600)
			service := world.newService("")
			if agent := getAgent(t, service, "hermes"); agent.State != StateUnknown || agent.Detail != test.wantErr.Error() {
				t.Fatalf("Hermes = %+v; want unknown", agent)
			}
			if _, err := service.Connect("hermes"); err == nil || err.Error() != test.wantErr.Error() {
				t.Fatalf("Connect() error = %v; want %v", err, test.wantErr)
			}
			if _, err := service.Disconnect("hermes"); err == nil || err.Error() != test.wantErr.Error() {
				t.Fatalf("Disconnect() error = %v; want %v", err, test.wantErr)
			}
			if got := world.readFile(world.hermesConfigPath()); got != userConfig {
				t.Fatalf("config.yaml = %s; want it as it was", got)
			}
		})
	}
}

func TestConnectHermesTakesItsEntryOutWhenHermesReadsItWrong(t *testing.T) {
	world := newFakeWorld(t)
	world.installHermes(world.pathOnUserPath("hermes"))
	userConfig := "model:\n  default: x\n"
	world.writeFile(world.hermesConfigPath(), userConfig, 0o600)
	world.setState("broken-hisar", "")
	_, err := world.newService("").Connect("hermes")
	if err == nil || !strings.HasPrefix(err.Error(), "Hermes did not accept the server Hisar added, so Hisar took it out again") {
		t.Fatalf("Connect() error = %v", err)
	}
	if got := world.readFile(world.hermesConfigPath()); got != userConfig {
		t.Fatalf("config.yaml = %s; want it as it was", got)
	}
}

func TestConnectHermesSaysWhenItCouldNotTakeItsEntryOut(t *testing.T) {
	for _, test := range []struct {
		name, userConfig string
	}{
		{"a file Hisar changed", "model:\n  default: x\n"},
		{"a file Hisar created", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			world := newFakeWorld(t)
			world.installHermes(world.pathOnUserPath("hermes"))
			if test.userConfig != "" {
				world.writeFile(world.hermesConfigPath(), test.userConfig, 0o600)
			}
			world.setState("broken-hisar", "")
			world.setState("edits-config", "")
			_, err := world.newService("").Connect("hermes")
			want := "Hermes did not accept the server Hisar added (Hermes reads Hisar's server differently), and Hermes' config.yaml changed after Hisar wrote it, so Hisar left it as it is. Remove the server named hisar from it if it is still there"
			if err == nil || err.Error() != want {
				t.Fatalf("Connect() error = %v; want %q", err, want)
			}
			written := test.userConfig
			if written != "" {
				written += "\n"
			}
			if got, want := world.readFile(world.hermesConfigPath()), written+world.ourHermesEntry()+"telemetry: false\n"; got != want {
				t.Fatalf("config.yaml =\n%s\nwant Hermes' change kept:\n%s", got, want)
			}
		})
	}
}

func TestConnectHermesNeedsTheBundledHisar(t *testing.T) {
	world := newFakeWorld(t)
	world.installHermes(world.pathOnUserPath("hermes"))
	world.hisarPath = filepath.Join(t.TempDir(), "Helpers", "hisar")
	if _, err := world.newService("").Connect("hermes"); err == nil || err.Error() != "this copy of Hisar does not include the hisar command-line tool" {
		t.Fatalf("Connect() error = %v", err)
	}
	if _, err := os.Stat(world.hermesConfigPath()); !os.IsNotExist(err) {
		t.Fatalf("config.yaml: %v; want none", err)
	}
}

func TestConnectHermesNamesTheDevelopmentSocket(t *testing.T) {
	for _, test := range []struct {
		name, socketPath string
		wantEnv          bool
	}{
		{"a development socket", "/tmp/hisar-dev/cli.sock", true},
		{"the default socket named", "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			world := newFakeWorld(t)
			world.installHermes(world.pathOnUserPath("hermes"))
			socketPath := test.socketPath
			if socketPath == "" {
				socketPath = cliprotocol.GetDefaultSocketPath(world.homeDirectory)
			}
			t.Setenv(cliprotocol.SocketEnvironment, socketPath)
			if _, err := world.newService("").Connect("hermes"); err != nil {
				t.Fatal(err)
			}
			want := world.ourHermesEntry()
			if test.wantEnv {
				want += "    env:\n      HISAR_CLI_SOCKET: \"" + socketPath + "\"\n"
			}
			if got := world.readFile(world.hermesConfigPath()); got != want {
				t.Fatalf("config.yaml =\n%s", got)
			}
		})
	}
}
