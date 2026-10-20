package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// claudeCodeConnector adds Hisar's MCP server to Claude Code's user settings with `claude mcp add --scope user`.
type claudeCodeConnector struct {
	hisarPath     string
	homeDirectory string
}

// configPath is the file `claude mcp add --scope user` writes: .claude.json in $CLAUDE_CONFIG_DIR, or in the home folder.
func (connector claudeCodeConnector) configPath() string {
	if directory := os.Getenv("CLAUDE_CONFIG_DIR"); directory != "" {
		return filepath.Join(directory, ".claude.json")
	}
	return filepath.Join(connector.homeDirectory, ".claude.json")
}

// getState reads the user settings: Claude Code's `mcp get` has no JSON and checks the server's health by starting it.
func (connector claudeCodeConnector) getState(program) (State, error) {
	path := connector.configPath()
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return StateAvailable, nil
	}
	if err != nil {
		return "", fmt.Errorf("Hisar cannot read Claude Code's settings in %s: %w", path, err)
	}
	var settings struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(content, &settings); err != nil {
		return "", fmt.Errorf("Claude Code's settings in %s are not valid JSON", path)
	}
	server, isPresent := settings.MCPServers[mcpServerName]
	if !isPresent {
		return StateAvailable, nil
	}
	var stdio struct {
		Type    string            `json:"type"`
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		Env     map[string]string `json:"env"`
	}
	if json.Unmarshal(server, &stdio) == nil && (stdio.Type == "" || stdio.Type == "stdio") && stdio.Command == connector.hisarPath &&
		connector.hisarPath != "" && slices.Equal(stdio.Args, mcpServerArguments) && len(stdio.Env) == 0 {
		return StateConnected, nil
	}
	return StateConflict, nil
}

func (connector claudeCodeConnector) connect(claude program) error {
	state, err := connector.getState(claude)
	if err != nil {
		return err
	}
	switch state {
	case StateConnected:
		return nil
	case StateConflict:
		return errConflict
	}
	if err := requireHisar(connector.hisarPath); err != nil {
		return err
	}
	arguments := append([]string{"mcp", "add", "--scope", "user", "--transport", "stdio", mcpServerName, "--", connector.hisarPath}, mcpServerArguments...)
	if _, err := claude.run(arguments...); err != nil {
		return fmt.Errorf("Claude Code could not add Hisar's server: %w", err)
	}
	return nil
}

func (connector claudeCodeConnector) disconnect(claude program) error {
	if state, err := connector.getState(claude); err != nil || state != StateConnected {
		return err
	}
	if _, err := claude.run("mcp", "remove", mcpServerName, "--scope", "user"); err != nil {
		return fmt.Errorf("Claude Code could not remove Hisar's server: %w", err)
	}
	return nil
}
