package harness

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
)

// ompPluginName is the name in omp/package.json, which omp lists and uninstalls the plugin by.
const ompPluginName = "@tryhisar/omp-plugin"

// ompLocations are where omp's own installer, Tern and Bun put omp.
func ompLocations(homeDirectory string) []string {
	return []string{
		filepath.Join(homeDirectory, "bin", "omp"),
		filepath.Join(homeDirectory, "Library", "Application Support", "Tern", "omp", "bin", "omp"),
		filepath.Join(homeDirectory, ".bun", "bin", "omp"),
	}
}

// ompConnector links the Hisar plugin, which brings the MCP server with it, through omp's plugin manager.
type ompConnector struct {
	// pluginPath is the plugin folder Connect links; empty when this copy of Hisar has none.
	pluginPath string
}

func (ompConnector) getState(omp program) (State, error) {
	output, err := omp.run("plugin", "list", "--json")
	if err != nil {
		return "", fmt.Errorf("omp could not list its plugins: %w", err)
	}
	var list struct {
		NPM []struct {
			Name      string `json:"name"`
			IsEnabled bool   `json:"enabled"`
		} `json:"npm"`
	}
	if err := json.Unmarshal(output, &list); err != nil {
		return "", errors.New("omp listed its plugins in a form Hisar cannot read")
	}
	for _, plugin := range list.NPM {
		if plugin.Name == ompPluginName && plugin.IsEnabled {
			return StateConnected, nil
		}
	}
	return StateAvailable, nil
}

func (connector ompConnector) connect(omp program) error {
	if connector.pluginPath == "" {
		return errors.New("this copy of Hisar does not include the omp plugin")
	}
	if _, err := omp.run("plugin", "link", connector.pluginPath); err != nil {
		return fmt.Errorf("omp could not connect to Hisar: %w", err)
	}
	return nil
}

// disconnect succeeds when the plugin is already gone, as after the user removed it in omp.
func (connector ompConnector) disconnect(omp program) error {
	if _, err := omp.run("plugin", "uninstall", ompPluginName); err != nil {
		if state, stateErr := connector.getState(omp); stateErr == nil && state != StateConnected {
			return nil
		}
		return fmt.Errorf("omp could not disconnect from Hisar: %w", err)
	}
	return nil
}
