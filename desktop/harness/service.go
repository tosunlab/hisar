// Package harness connects agent harnesses to Hisar from Settings → Coding agents: it finds omp and links or unlinks
// the Hisar plugin that ships inside the app.
package harness

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Config struct {
	HomeDirectory string
	// GetUserPath returns the login shell's PATH, which an app started from Finder does not get.
	GetUserPath func() []string
	// OmpPluginPath is the plugin folder Connect links; empty when this copy of Hisar has none.
	OmpPluginPath string
}

// Service is bound to the UI. The mutex keeps two clicks from running omp's plugin manager at once: omp writes its
// plugin list without a lock.
type Service struct {
	config  Config
	mutex   sync.Mutex
	timeout time.Duration
}

func NewService(config Config) *Service {
	return &Service{config: config, timeout: ompTimeout}
}

func (service *Service) findOmp() (ompCommand, bool) {
	userPath := service.config.GetUserPath()
	path, isFound := findOmp(service.config.HomeDirectory, userPath)
	return ompCommand{path: path, userPath: userPath, timeout: service.timeout}, isFound
}

func (service *Service) GetState() (OmpState, error) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	omp, isFound := service.findOmp()
	if !isFound {
		return OmpNotInstalled, nil
	}
	return omp.getState()
}

// Connect links the bundled plugin, so every omp session started afterwards loads Hisar.
func (service *Service) Connect() (OmpState, error) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if service.config.OmpPluginPath == "" {
		return "", errors.New("this copy of Hisar does not include the omp plugin")
	}
	omp, isFound := service.findOmp()
	if !isFound {
		return "", errors.New("omp is not installed")
	}
	if err := omp.link(service.config.OmpPluginPath); err != nil {
		return "", err
	}
	return omp.getState()
}

// Disconnect succeeds when the plugin is already gone, as after the user removed it in omp.
func (service *Service) Disconnect() (OmpState, error) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	omp, isFound := service.findOmp()
	if !isFound {
		return OmpNotInstalled, nil
	}
	if err := omp.unlink(); err != nil {
		if state, stateErr := omp.getState(); stateErr == nil && state != OmpConnected {
			return state, nil
		}
		return "", err
	}
	return omp.getState()
}

// FindOmpPlugin returns Hisar.app's Contents/Resources/omp-plugin, or for a development build the repository's omp
// folder above the executable.
func FindOmpPlugin(executableDirectory string) string {
	bundled := filepath.Join(filepath.Dir(executableDirectory), "Resources", "omp-plugin")
	if isFile(filepath.Join(bundled, "package.json")) {
		return bundled
	}
	for directory := executableDirectory; ; directory = filepath.Dir(directory) {
		if candidate := filepath.Join(directory, "omp"); isFile(filepath.Join(candidate, "package.json")) {
			return candidate
		}
		if filepath.Dir(directory) == directory {
			return ""
		}
	}
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
