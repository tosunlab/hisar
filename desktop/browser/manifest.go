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
	name             string
	profileDirectory string
}

// getKnownBrowsers lists Chromium browsers by the directory that holds their NativeMessagingHosts.
func getKnownBrowsers(homeDirectory, operatingSystem string) []knownBrowser {
	if operatingSystem == "darwin" {
		support := filepath.Join(homeDirectory, "Library", "Application Support")
		return []knownBrowser{
			{"Google Chrome", filepath.Join(support, "Google", "Chrome")},
			{"Google Chrome Beta", filepath.Join(support, "Google", "Chrome Beta")},
			{"Google Chrome Canary", filepath.Join(support, "Google", "Chrome Canary")},
			{"Chromium", filepath.Join(support, "Chromium")},
			{"Brave", filepath.Join(support, "BraveSoftware", "Brave-Browser")},
			{"Microsoft Edge", filepath.Join(support, "Microsoft Edge")},
			{"Arc", filepath.Join(support, "Arc", "User Data")},
			{"Vivaldi", filepath.Join(support, "Vivaldi")},
		}
	}
	config := filepath.Join(homeDirectory, ".config")
	return []knownBrowser{
		{"Google Chrome", filepath.Join(config, "google-chrome")},
		{"Chromium", filepath.Join(config, "chromium")},
		{"Brave", filepath.Join(config, "BraveSoftware", "Brave-Browser")},
		{"Microsoft Edge", filepath.Join(config, "microsoft-edge")},
		{"Vivaldi", filepath.Join(config, "vivaldi")},
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

// InstallHostManifests registers the native host with every installed Chromium browser; it only writes changed files.
func InstallHostManifests(homeDirectory, hostPath string) []browserprotocol.BrowserInstallation {
	return installHostManifests(homeDirectory, hostPath, runtime.GOOS)
}

func installHostManifests(homeDirectory, hostPath, operatingSystem string) []browserprotocol.BrowserInstallation {
	manifest, manifestErr := buildHostManifest(hostPath)
	installations := []browserprotocol.BrowserInstallation{}
	for _, browser := range getKnownBrowsers(homeDirectory, operatingSystem) {
		if _, err := os.Stat(browser.profileDirectory); err != nil {
			continue
		}
		installation := browserprotocol.BrowserInstallation{Name: browser.name, ManifestPath: filepath.Join(browser.profileDirectory, "NativeMessagingHosts", browserprotocol.HostName+".json")}
		err := manifestErr
		if err == nil {
			err = writeFileIfChanged(installation.ManifestPath, manifest)
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
