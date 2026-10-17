package browser

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/tryhisar/hisar/core/browserprotocol"
)

type knownBrowser struct {
	name              string
	userDataDirectory string
}

// getKnownBrowsers lists Chromium browsers by their user data directory: it holds NativeMessagingHosts, and Local State once the browser has run.
func getKnownBrowsers(homeDirectory, operatingSystem string) []knownBrowser {
	if operatingSystem == "darwin" {
		support := filepath.Join(homeDirectory, "Library", "Application Support")
		return []knownBrowser{
			{"Google Chrome", filepath.Join(support, "Google", "Chrome")},
			{"Google Chrome Beta", filepath.Join(support, "Google", "Chrome Beta")},
			{"Google Chrome Dev", filepath.Join(support, "Google", "Chrome Dev")},
			{"Google Chrome Canary", filepath.Join(support, "Google", "Chrome Canary")},
			{"Chromium", filepath.Join(support, "Chromium")},
			{"Brave", filepath.Join(support, "BraveSoftware", "Brave-Browser")},
			{"Brave Beta", filepath.Join(support, "BraveSoftware", "Brave-Browser-Beta")},
			{"Brave Nightly", filepath.Join(support, "BraveSoftware", "Brave-Browser-Nightly")},
			{"Microsoft Edge", filepath.Join(support, "Microsoft Edge")},
			{"Microsoft Edge Beta", filepath.Join(support, "Microsoft Edge Beta")},
			{"Microsoft Edge Dev", filepath.Join(support, "Microsoft Edge Dev")},
			{"Microsoft Edge Canary", filepath.Join(support, "Microsoft Edge Canary")},
			{"Arc", filepath.Join(support, "Arc", "User Data")},
			{"Dia", filepath.Join(support, "Dia", "User Data")},
			{"Vivaldi", filepath.Join(support, "Vivaldi")},
			{"Vivaldi Snapshot", filepath.Join(support, "Vivaldi Snapshot")},
			{"Opera", filepath.Join(support, "com.operasoftware.Opera")},
			{"Opera GX", filepath.Join(support, "com.operasoftware.OperaGX")},
		}
	}
	config := filepath.Join(homeDirectory, ".config")
	return []knownBrowser{
		{"Google Chrome", filepath.Join(config, "google-chrome")},
		{"Google Chrome Beta", filepath.Join(config, "google-chrome-beta")},
		{"Google Chrome Dev", filepath.Join(config, "google-chrome-unstable")},
		{"Chromium", filepath.Join(config, "chromium")},
		{"Brave", filepath.Join(config, "BraveSoftware", "Brave-Browser")},
		{"Brave Beta", filepath.Join(config, "BraveSoftware", "Brave-Browser-Beta")},
		{"Brave Nightly", filepath.Join(config, "BraveSoftware", "Brave-Browser-Nightly")},
		{"Microsoft Edge", filepath.Join(config, "microsoft-edge")},
		{"Microsoft Edge Beta", filepath.Join(config, "microsoft-edge-beta")},
		{"Microsoft Edge Dev", filepath.Join(config, "microsoft-edge-dev")},
		{"Vivaldi", filepath.Join(config, "vivaldi")},
		{"Vivaldi Snapshot", filepath.Join(config, "vivaldi-snapshot")},
		{"Opera", filepath.Join(config, "opera")},
	}
}

func buildHostManifest(hostPath string) ([]byte, error) {
	return json.MarshalIndent(map[string]any{
		"name":            browserprotocol.HostName,
		"description":     "Hisar password manager",
		"path":            hostPath,
		"type":            "stdio",
		"allowed_origins": []string{browserprotocol.AllowedOrigin},
	}, "", "  ")
}

// InstallHostManifests writes the native host manifest for every known Chromium browser, so one installed later finds it
// at its first launch, and reports the browsers that have run. It only writes changed files.
func InstallHostManifests(homeDirectory, hostPath string) []browserprotocol.BrowserInstallation {
	return installHostManifests(homeDirectory, hostPath, runtime.GOOS)
}

func installHostManifests(homeDirectory, hostPath, operatingSystem string) []browserprotocol.BrowserInstallation {
	manifest, manifestErr := buildHostManifest(hostPath)
	installations := []browserprotocol.BrowserInstallation{}
	for _, browser := range getKnownBrowsers(homeDirectory, operatingSystem) {
		installation := browserprotocol.BrowserInstallation{Name: browser.name, ManifestPath: filepath.Join(browser.userDataDirectory, "NativeMessagingHosts", browserprotocol.HostName+".json")}
		err := manifestErr
		if err == nil {
			err = writeFileIfChanged(installation.ManifestPath, manifest)
		}
		if _, statErr := os.Stat(filepath.Join(browser.userDataDirectory, "Local State")); statErr != nil {
			continue
		}
		installation.IsInstalled = err == nil
		if err != nil {
			installation.ErrorMessage = err.Error()
		}
		installations = append(installations, installation)
	}
	return installations
}

func writeFileIfChanged(path string, content []byte) error {
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, content) {
		return nil
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	partialPath := path + ".partial"
	if err := os.WriteFile(partialPath, content, 0o644); err != nil {
		return err
	}
	if err := os.Rename(partialPath, path); err != nil {
		os.Remove(partialPath)
		return err
	}
	return nil
}
