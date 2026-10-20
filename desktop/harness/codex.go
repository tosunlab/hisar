package harness

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
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

var errCodexChanged = errors.New("Codex's config.toml changed while Hisar was connecting. Try again")

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
	config, err := readConfig(connector.configPath())
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
	path := connector.configPath()
	// A config.toml linked from a dotfiles folder stays a link: the file it points to is the one that changes.
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
	original, err := readConfig(path)
	if err != nil {
		return err
	}
	edited, err := setCodexServer(original.content, connector.hisarPath)
	if err != nil {
		return err
	}
	if err := replaceConfig(path, original, edited); err != nil {
		return err
	}
	if state, err := connector.getState(codex); err != nil || state != StateConnected {
		restoreErr := restoreConfig(path, original, edited)
		if err == nil {
			err = errors.New("Codex did not load Hisar's server")
		}
		return errors.Join(fmt.Errorf("Codex did not accept the server Hisar added, so Hisar took it out again: %w", err), restoreErr)
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

// codexConfig is config.toml as Hisar read it; a file that does not exist yet is created owner-only, as Codex does.
type codexConfig struct {
	content  []byte
	mode     fs.FileMode
	isExists bool
}

func readConfig(path string) (codexConfig, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return codexConfig{mode: 0o600}, nil
	}
	if err != nil {
		return codexConfig{}, fmt.Errorf("Hisar cannot read Codex's config.toml: %w", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return codexConfig{}, fmt.Errorf("Hisar cannot read Codex's config.toml: %w", err)
	}
	return codexConfig{content: content, mode: info.Mode().Perm(), isExists: true}, nil
}

// replaceConfig writes content to a temporary file beside path and renames it over path, so Codex never reads half a
// file, unless path no longer holds what Hisar read: then whoever changed it keeps their change.
func replaceConfig(path string, expected codexConfig, content []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("Hisar cannot create Codex's folder: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".config.toml.hisar-*")
	if err != nil {
		return fmt.Errorf("Hisar cannot write Codex's config.toml: %w", err)
	}
	defer os.Remove(temporary.Name())
	_, err = temporary.Write(content)
	if err == nil {
		err = temporary.Chmod(expected.mode)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("Hisar cannot write Codex's config.toml: %w", err)
	}
	current, err := readConfig(path)
	if err != nil {
		return err
	}
	if current.isExists != expected.isExists || !bytes.Equal(current.content, expected.content) {
		return errCodexChanged
	}
	if err := os.Rename(temporary.Name(), path); err != nil {
		return fmt.Errorf("Hisar cannot write Codex's config.toml: %w", err)
	}
	return nil
}

// restoreConfig puts back what Hisar read, as long as the file still holds what Hisar wrote.
func restoreConfig(path string, original codexConfig, edited []byte) error {
	if original.isExists {
		return replaceConfig(path, codexConfig{content: edited, mode: original.mode, isExists: true}, original.content)
	}
	current, err := readConfig(path)
	if err != nil || !bytes.Equal(current.content, edited) {
		return err
	}
	return os.Remove(path)
}
