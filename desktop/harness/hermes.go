package harness

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
)

// hermesHomeVariable is where Hermes keeps its home when it is not ~/.hermes. A user who moved it sets it in their shell
// profile, so Hisar reads it from the login shell.
const hermesHomeVariable = "HERMES_HOME"

// ListUserVariables names the login shell's variables the connectors read.
func ListUserVariables() []string {
	return []string{hermesHomeVariable}
}

var hermesConfigFile = configFile{name: "Hermes' config.yaml", folderName: "Hermes' folder"}

// hermesConnector writes Hisar's entry into Hermes' config.yaml itself, because Hermes' own `hermes mcp add` asks its
// questions in a terminal and rewrites the whole file.
type hermesConnector struct {
	server        hermesServer
	getHermesHome func() string
}

// configPath asks Hermes which config.yaml it reads: $HERMES_HOME/config.yaml, ~/.hermes/config.yaml, or the one of the
// profile `hermes profile use` made sticky. It runs with the login shell's HERMES_HOME, as hermes does in a terminal.
func (connector hermesConnector) configPath(hermes program) (string, error) {
	output, err := connector.inHome(hermes).run("config", "path")
	if err != nil {
		return "", fmt.Errorf("Hermes could not say where its config.yaml is: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	path := strings.TrimSpace(lines[len(lines)-1])
	if !filepath.IsAbs(path) {
		return "", errors.New("Hermes named its config.yaml in a form Hisar cannot read")
	}
	return path, nil
}

// inHome sets the login shell's HERMES_HOME even when it is empty, so a value this app inherited and the user has since
// removed from their profile does not apply; Hermes reads an empty HERMES_HOME as unset.
func (connector hermesConnector) inHome(hermes program) program {
	hermes.environment = append(slices.Clone(hermes.environment), hermesHomeVariable+"="+connector.getHermesHome())
	return hermes
}

func (connector hermesConnector) getState(hermes program) (State, error) {
	path, err := connector.configPath(hermes)
	if err != nil {
		return "", err
	}
	state, _, err := connector.readState(path)
	return state, err
}

func (connector hermesConnector) readState(path string) (State, configSnapshot, error) {
	config, err := hermesConfigFile.read(path)
	if err != nil {
		return "", configSnapshot{}, err
	}
	document, err := parseHermesDocument(config.content)
	if err != nil {
		return "", configSnapshot{}, err
	}
	entry, err := document.findServer(connector.server)
	switch {
	case err != nil:
		return "", configSnapshot{}, err
	case entry.isOurs:
		return StateConnected, config, nil
	case entry.isNamed:
		return StateConflict, config, nil
	}
	return StateAvailable, config, nil
}

func (connector hermesConnector) connect(hermes program) error {
	path, err := connector.configPath(hermes)
	if err != nil {
		return err
	}
	state, original, err := connector.readState(path)
	switch {
	case err != nil:
		return err
	case state == StateConnected:
		return nil
	case state == StateConflict:
		return errConflict
	}
	if err := requireHisar(connector.server.hisarPath); err != nil {
		return err
	}
	edited, err := setHermesServer(original.content, connector.server)
	if err != nil {
		return err
	}
	written, err := hermesConfigFile.replace(original, edited)
	if err != nil {
		return err
	}
	if err := connector.checkLoaded(hermes); err != nil {
		return hermesConfigFile.rollBack("Hermes", err, original, written)
	}
	return nil
}

func (connector hermesConnector) checkLoaded(hermes program) error {
	output, err := connector.inHome(hermes).run("config", "get", "mcp_servers."+mcpServerName, "--json", "--raw")
	if err != nil {
		return err
	}
	var got any
	if err := json.Unmarshal(output, &got); err != nil {
		return errors.New("Hermes listed Hisar's server in a form Hisar cannot read")
	}
	encoded, _ := json.Marshal(connector.server.value())
	var want any
	_ = json.Unmarshal(encoded, &want)
	if !reflect.DeepEqual(got, want) {
		return errors.New("Hermes reads Hisar's server differently")
	}
	return nil
}

func (connector hermesConnector) disconnect(hermes program) error {
	path, err := connector.configPath(hermes)
	if err != nil {
		return err
	}
	state, original, err := connector.readState(path)
	if err != nil || state != StateConnected {
		return err
	}
	edited, err := removeHermesServer(original.content, connector.server)
	if err != nil {
		return err
	}
	_, err = hermesConfigFile.replace(original, edited)
	return err
}
