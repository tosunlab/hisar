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
	name string
	// userDataDirectory holds Local State once the browser has run.
	userDataDirectory string
	// hostDirectory is where the browser looks for NativeMessagingHosts; empty means userDataDirectory, as in stock Chromium.
	hostDirectory string
}

// getKnownBrowsers lists Chromium browsers by their user data directory.
func getKnownBrowsers(homeDirectory, operatingSystem string) []knownBrowser {
	if operatingSystem == "darwin" {
		support := filepath.Join(homeDirectory, "Library", "Application Support")
		chrome := filepath.Join(support, "Google", "Chrome")
		return []knownBrowser{
			{name: "Google Chrome", userDataDirectory: chrome},
			{name: "Google Chrome Beta", userDataDirectory: filepath.Join(support, "Google", "Chrome Beta")},
			{name: "Google Chrome Dev", userDataDirectory: filepath.Join(support, "Google", "Chrome Dev")},
			{name: "Google Chrome Canary", userDataDirectory: filepath.Join(support, "Google", "Chrome Canary")},
			{name: "Chromium", userDataDirectory: filepath.Join(support, "Chromium")},
			{name: "Brave", userDataDirectory: filepath.Join(support, "BraveSoftware", "Brave-Browser")},
			{name: "Brave Beta", userDataDirectory: filepath.Join(support, "BraveSoftware", "Brave-Browser-Beta")},
			{name: "Brave Nightly", userDataDirectory: filepath.Join(support, "BraveSoftware", "Brave-Browser-Nightly")},
			{name: "Microsoft Edge", userDataDirectory: filepath.Join(support, "Microsoft Edge")},
			{name: "Microsoft Edge Beta", userDataDirectory: filepath.Join(support, "Microsoft Edge Beta")},
			{name: "Microsoft Edge Dev", userDataDirectory: filepath.Join(support, "Microsoft Edge Dev")},
			{name: "Microsoft Edge Canary", userDataDirectory: filepath.Join(support, "Microsoft Edge Canary")},
			{name: "Arc", userDataDirectory: filepath.Join(support, "Arc", "User Data")},
			// Dia (ArcCore 1.53) ignores NativeMessagingHosts in its own user data directory and reads Chrome's.
			{name: "Dia", userDataDirectory: filepath.Join(support, "Dia", "User Data"), hostDirectory: chrome},
			{name: "Vivaldi", userDataDirectory: filepath.Join(support, "Vivaldi")},
			{name: "Vivaldi Snapshot", userDataDirectory: filepath.Join(support, "Vivaldi Snapshot")},
			{name: "Opera", userDataDirectory: filepath.Join(support, "com.operasoftware.Opera")},
			{name: "Opera GX", userDataDirectory: filepath.Join(support, "com.operasoftware.OperaGX")},
		}
	}
	config := filepath.Join(homeDirectory, ".config")
	return []knownBrowser{
		{name: "Google Chrome", userDataDirectory: filepath.Join(config, "google-chrome")},
		{name: "Google Chrome Beta", userDataDirectory: filepath.Join(config, "google-chrome-beta")},
		{name: "Google Chrome Dev", userDataDirectory: filepath.Join(config, "google-chrome-unstable")},
		{name: "Chromium", userDataDirectory: filepath.Join(config, "chromium")},
		{name: "Brave", userDataDirectory: filepath.Join(config, "BraveSoftware", "Brave-Browser")},
		{name: "Brave Beta", userDataDirectory: filepath.Join(config, "BraveSoftware", "Brave-Browser-Beta")},
		{name: "Brave Nightly", userDataDirectory: filepath.Join(config, "BraveSoftware", "Brave-Browser-Nightly")},
		{name: "Microsoft Edge", userDataDirectory: filepath.Join(config, "microsoft-edge")},
		{name: "Microsoft Edge Beta", userDataDirectory: filepath.Join(config, "microsoft-edge-beta")},
		{name: "Microsoft Edge Dev", userDataDirectory: filepath.Join(config, "microsoft-edge-dev")},
		{name: "Vivaldi", userDataDirectory: filepath.Join(config, "vivaldi")},
		{name: "Vivaldi Snapshot", userDataDirectory: filepath.Join(config, "vivaldi-snapshot")},
		{name: "Opera", userDataDirectory: filepath.Join(config, "opera")},
	}
}

func (browser knownBrowser) getManifestPath() string {
	hostDirectory := browser.hostDirectory
	if hostDirectory == "" {
		hostDirectory = browser.userDataDirectory
	}
	return filepath.Join(hostDirectory, "NativeMessagingHosts", browserprotocol.HostName+".json")
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
		installation := browserprotocol.BrowserInstallation{Name: browser.name, ManifestPath: browser.getManifestPath()}
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
