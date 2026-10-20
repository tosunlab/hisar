package harness

import (
	"path/filepath"
	"slices"
	"testing"
)

const claudeConflict = "Claude Code already has a server named hisar. Remove it to connect Hisar."

func TestFindClaudeCodeWhereItsInstallersPutIt(t *testing.T) {
	for _, test := range []struct {
		name     string
		location func(*fakeWorld) string
	}{
		{"the login shell's PATH", func(world *fakeWorld) string { return world.pathOnUserPath("claude") }},
		{"the native installer", func(world *fakeWorld) string { return world.pathInHome(".local", "bin", "claude") }},
		{"Homebrew", func(world *fakeWorld) string { return world.pathInRoot("opt", "homebrew", "bin", "claude") }},
		{"npm in /usr/local", func(world *fakeWorld) string { return world.pathInRoot("usr", "local", "bin", "claude") }},
		{"the old local install", func(world *fakeWorld) string { return world.pathInHome(".claude", "local", "claude") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			world := newFakeWorld(t)
			service := world.newService("")
			if agent := getAgent(t, service, "claude-code"); agent.State != StateNotInstalled {
				t.Fatalf("Claude Code before installing = %+v; want not-installed", agent)
			}
			world.installClaude(test.location(world))
			if _, err := service.Connect("claude-code"); err != nil {
				t.Fatal(err)
			}
			if calls := world.readCalls(); len(calls) != 1 || !slices.Contains(calls, test.location(world)+" mcp add --scope user --transport stdio hisar -- "+
				world.hisarPath+" mcp --no-reveal (in "+world.homeDirectory+")") {
				t.Fatalf("claude calls = %q; want one mcp add by %s in the home folder", calls, test.location(world))
			}
		})
	}
}

func TestClaudeCodeStateReadsTheUserSettings(t *testing.T) {
	ours := func(world *fakeWorld) string {
		return `{"mcpServers":{"hisar":{"type":"stdio","command":"` + world.hisarPath + `","args":["mcp","--no-reveal"],"env":{}}}}`
	}
	for _, test := range []struct {
		name       string
		settings   func(*fakeWorld) string
		want       State
		wantDetail string
	}{
		{name: "no settings yet", want: StateAvailable},
		{name: "other servers", settings: func(*fakeWorld) string { return `{"mcpServers":{"github":{"command":"gh"}}}` }, want: StateAvailable},
		{name: "Hisar's server", settings: ours, want: StateConnected},
		{
			name: "a hisar on the PATH",
			settings: func(*fakeWorld) string {
				return `{"mcpServers":{"hisar":{"type":"stdio","command":"hisar","args":["mcp"]}}}`
			},
			want: StateConflict, wantDetail: claudeConflict,
		},
		{
			name: "Hisar's command with get_secret on",
			settings: func(world *fakeWorld) string {
				return `{"mcpServers":{"hisar":{"type":"stdio","command":"` + world.hisarPath + `","args":["mcp"]}}}`
			},
			want: StateConflict, wantDetail: claudeConflict,
		},
		{
			name: "a remote server named hisar",
			settings: func(*fakeWorld) string {
				return `{"mcpServers":{"hisar":{"type":"http","url":"https://example.com/mcp"}}}`
			},
			want: StateConflict, wantDetail: claudeConflict,
		},
		{
			name:     "settings that are not JSON",
			settings: func(*fakeWorld) string { return `{"mcpServers":` },
			want:     StateUnknown,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			world := newFakeWorld(t)
			world.installClaude(world.pathOnUserPath("claude"))
			settingsPath := world.pathInHome(".claude.json")
			if test.settings != nil {
				world.writeFile(settingsPath, test.settings(world), 0o600)
			}
			wantDetail := test.wantDetail
			if test.want == StateUnknown {
				wantDetail = "Claude Code's settings in " + settingsPath + " are not valid JSON"
			}
			want := Agent{ID: "claude-code", Name: "Claude Code", State: test.want, Detail: wantDetail}
			if agent := getAgent(t, world.newService(""), "claude-code"); agent != want {
				t.Fatalf("Claude Code = %+v; want %+v", agent, want)
			}
			if calls := world.readCalls(); len(calls) != 0 {
				t.Fatalf("claude calls = %q; want none", calls)
			}
		})
	}
}

func TestClaudeCodeConfigDirectoryHoldsTheSettings(t *testing.T) {
	world := newFakeWorld(t)
	world.installClaude(world.pathOnUserPath("claude"))
	configDirectory := filepath.Join(t.TempDir(), "claude-config")
	t.Setenv("CLAUDE_CONFIG_DIR", configDirectory)
	world.writeFile(filepath.Join(configDirectory, ".claude.json"), "{}", 0o600)
	service := world.newService("")
	if agent, err := service.Connect("claude-code"); err != nil || agent.State != StateConnected {
		t.Fatalf("Connect() = %+v, %v; want connected", agent, err)
	}
	world.writeFile(world.pathInHome(".claude.json"), `{"mcpServers":{"hisar":{"command":"hisar"}}}`, 0o600)
	if agent := getAgent(t, service, "claude-code"); agent.State != StateConnected {
		t.Fatalf("Claude Code = %+v; want connected from CLAUDE_CONFIG_DIR", agent)
	}
}

func TestConnectClaudeCodeLeavesAnotherHisarServer(t *testing.T) {
	world := newFakeWorld(t)
	world.installClaude(world.pathOnUserPath("claude"))
	settings := `{"mcpServers":{"hisar":{"type":"stdio","command":"hisar","args":["mcp"],"env":{}}}}`
	world.writeFile(world.pathInHome(".claude.json"), settings, 0o600)
	service := world.newService("")
	if _, err := service.Connect("claude-code"); err == nil || err.Error() != claudeConflict {
		t.Fatalf("Connect() error = %v; want %q", err, claudeConflict)
	}
	if agent, err := service.Disconnect("claude-code"); err != nil || agent.State != StateConflict {
		t.Fatalf("Disconnect() = %+v, %v; want the conflict left alone", agent, err)
	}
	if calls := world.readCalls(); len(calls) != 0 {
		t.Fatalf("claude calls = %q; want none", calls)
	}
	if got := world.readFile(world.pathInHome(".claude.json")); got != settings {
		t.Fatalf(".claude.json = %s; want it unchanged", got)
	}
}

func TestConnectClaudeCodeNeedsTheBundledHisar(t *testing.T) {
	world := newFakeWorld(t)
	world.installClaude(world.pathOnUserPath("claude"))
	world.hisarPath = filepath.Join(t.TempDir(), "Helpers", "hisar")
	if _, err := world.newService("").Connect("claude-code"); err == nil || err.Error() != "this copy of Hisar does not include the hisar command-line tool" {
		t.Fatalf("Connect() error = %v", err)
	}
	if calls := world.readCalls(); len(calls) != 0 {
		t.Fatalf("claude calls = %q; want none", calls)
	}
}

func TestDisconnectClaudeCodeRemovesHisarsServer(t *testing.T) {
	world := newFakeWorld(t)
	claudePath := world.pathOnUserPath("claude")
	world.installClaude(claudePath)
	service := world.newService("")
	if _, err := service.Connect("claude-code"); err != nil {
		t.Fatal(err)
	}
	if agent, err := service.Connect("claude-code"); err != nil || agent.State != StateConnected {
		t.Fatalf("second Connect() = %+v, %v; want connected", agent, err)
	}
	for range 2 {
		if agent, err := service.Disconnect("claude-code"); err != nil || agent.State != StateAvailable {
			t.Fatalf("Disconnect() = %+v, %v; want available", agent, err)
		}
	}
	want := []string{
		claudePath + " mcp add --scope user --transport stdio hisar -- " + world.hisarPath + " mcp --no-reveal (in " + world.homeDirectory + ")",
		claudePath + " mcp remove hisar --scope user (in " + world.homeDirectory + ")",
	}
	if calls := world.readCalls(); !slices.Equal(calls, want) {
		t.Fatalf("claude calls = %q; want %q", calls, want)
	}
}
