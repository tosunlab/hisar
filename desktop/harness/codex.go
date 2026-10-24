package harness

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// codexConnector writes Hisar's table into Codex's config.toml itself, because `codex mcp add` cannot set the
// timeouts, and removes it with `codex mcp remove`.
type codexConnector struct {
	hisarPath     string
	homeDirectory string
}

var codexConfigFile = configFile{name: "Codex's config.toml", folderName: "Codex's folder"}

// configPath is $CODEX_HOME/config.toml, ~/.codex/config.toml by default.
func (connector codexConnector) configPath() string {
	directory := os.Getenv("CODEX_HOME")
	if directory == "" {
		directory = filepath.Join(connector.homeDirectory, ".codex")
	}
	return filepath.Join(directory, "config.toml")
}

// getState asks Codex, which reads its whole configuration, profiles and all, how it runs the server named hisar. The
// server is Hisar's only when Codex runs it as Hisar's table says and config.toml holds that table exactly as Connect
// writes it, so Disconnect never removes a server the user changed.
func (connector codexConnector) getState(codex program) (State, error) {
	output, err := codex.run("mcp", "get", mcpServerName, "--json")
	if err != nil {
		if strings.Contains(err.Error(), "No MCP server named") {
			return StateAvailable, nil
		}
		return "", fmt.Errorf("Codex could not list its MCP servers: %w", err)
	}
	var server struct {
		IsEnabled bool `json:"enabled"`
		Transport struct {
			Type    string            `json:"type"`
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
			EnvVars []string          `json:"env_vars"`
			Cwd     *string           `json:"cwd"`
		} `json:"transport"`
		EnabledTools   []string `json:"enabled_tools"`
		DisabledTools  []string `json:"disabled_tools"`
		StartupTimeout *float64 `json:"startup_timeout_sec"`
		ToolTimeout    *float64 `json:"tool_timeout_sec"`
	}
	if err := json.Unmarshal(output, &server); err != nil {
		return "", errors.New("Codex listed its MCP servers in a form Hisar cannot read")
	}
	transport := server.Transport
	if !server.IsEnabled || transport.Type != "stdio" || connector.hisarPath == "" || transport.Command != connector.hisarPath ||
		!slices.Equal(transport.Args, mcpServerArguments) || len(transport.Env) != 0 || len(transport.EnvVars) != 0 ||
		transport.Cwd != nil || server.EnabledTools != nil || server.DisabledTools != nil ||
		!isNilOr(server.StartupTimeout, codexStartupTimeoutSeconds) || !isNilOr(server.ToolTimeout, codexToolTimeoutSeconds) {
		return StateConflict, nil
	}
	config, err := codexConfigFile.read(connector.configPath())
	if err != nil {
		return "", err
	}
	found, err := findCodexServer(config.content, connector.hisarPath)
	if err != nil {
		return "", err
	}
	if !found.isOurs {
		return StateConflict, nil
	}
	return StateConnected, nil
}

// isNilOr is true for want and for no value: a Codex that does not report the timeouts, or one left unset, which the
// check of config.toml's table catches.
func isNilOr(value *float64, want float64) bool {
	return value == nil || *value == want
}

func (connector codexConnector) connect(codex program) error {
	state, err := connector.getState(codex)
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
	original, err := codexConfigFile.read(connector.configPath())
	if err != nil {
		return err
	}
	edited, err := setCodexServer(original.content, connector.hisarPath)
	if err != nil {
		return err
	}
	written, err := codexConfigFile.replace(original, edited)
	if err != nil {
		return err
	}
	if state, err := connector.getState(codex); err != nil || state != StateConnected {
		if err == nil {
			err = errors.New("Codex did not load Hisar's server")
		}
		return codexConfigFile.rollBack("Codex", err, original, written)
	}
	return nil
}

func (connector codexConnector) disconnect(codex program) error {
	if state, err := connector.getState(codex); err != nil || state != StateConnected {
		return err
	}
	if _, err := codex.run("mcp", "remove", mcpServerName); err != nil {
		return fmt.Errorf("Codex could not remove Hisar's server: %w", err)
	}
	return nil
}
