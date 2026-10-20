package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// OmpState is what Settings shows on the omp card.
type OmpState string

const (
	OmpNotInstalled OmpState = "not-installed"
	OmpAvailable    OmpState = "available"
	OmpConnected    OmpState = "connected"
)

// ompPluginName is the name in omp/package.json, which omp lists and uninstalls the plugin by.
const ompPluginName = "@tryhisar/omp-plugin"

const ompTimeout = 30 * time.Second

// The plugin manager also runs Bun as a child of omp; past this delay a child holding the output open is abandoned.
const ompWaitDelay = 2 * time.Second

// findOmp looks on the login shell's PATH, then where omp's own installer, Tern and Bun put it.
func findOmp(homeDirectory string, userPath []string) (string, bool) {
	var candidates []string
	for _, directory := range userPath {
		if filepath.IsAbs(directory) {
			candidates = append(candidates, filepath.Join(directory, "omp"))
		}
	}
	candidates = append(candidates,
		filepath.Join(homeDirectory, "bin", "omp"),
		filepath.Join(homeDirectory, "Library", "Application Support", "Tern", "omp", "bin", "omp"),
		filepath.Join(homeDirectory, ".bun", "bin", "omp"),
	)
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return candidate, true
		}
	}
	return "", false
}

type ompCommand struct {
	path     string
	userPath []string
	timeout  time.Duration
}

// run starts omp outside any project, so project plugin folders do not change what it lists, with the folder omp is
// in on the PATH, so an omp that Bun installed finds bun.
func (command ompCommand) run(arguments ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), command.timeout)
	defer cancel()
	process := exec.CommandContext(ctx, command.path, arguments...)
	process.Dir = "/"
	searchPath := append([]string{filepath.Dir(command.path)}, command.userPath...)
	process.Env = append(os.Environ(), "PATH="+strings.Join(searchPath, string(os.PathListSeparator)), "NO_COLOR=1")
	process.WaitDelay = ompWaitDelay
	var stderr bytes.Buffer
	process.Stderr = &stderr
	output, err := process.Output()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("omp did not answer within %s", command.timeout)
	}
	if err != nil {
		return nil, errors.New(describeOmpFailure(stderr.String(), err))
	}
	return output, nil
}

// describeOmpFailure keeps omp's last line, without its cross mark and the "Error:" of the JavaScript error inside it.
func describeOmpFailure(stderr string, err error) string {
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	detail := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[len(lines)-1]), "✘"))
	detail = strings.ReplaceAll(detail, "Error: ", "")
	if detail == "" {
		return err.Error()
	}
	return detail
}

func (command ompCommand) getState() (OmpState, error) {
	output, err := command.run("plugin", "list", "--json")
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
			return OmpConnected, nil
		}
	}
	return OmpAvailable, nil
}

func (command ompCommand) link(pluginPath string) error {
	if _, err := command.run("plugin", "link", pluginPath); err != nil {
		return fmt.Errorf("omp could not connect to Hisar: %w", err)
	}
	return nil
}

func (command ompCommand) unlink() error {
	if _, err := command.run("plugin", "uninstall", ompPluginName); err != nil {
		return fmt.Errorf("omp could not disconnect from Hisar: %w", err)
	}
	return nil
}
