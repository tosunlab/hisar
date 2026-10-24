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

// browserFamily decides how a browser names the extensions allowed to start the host, and where it keeps its manifests.
type browserFamily int

const (
	chromiumFamily browserFamily = iota
	firefoxFamily
)

type knownBrowser struct {
	name   string
	family browserFamily
	// userDataDirectory holds Local State once a Chromium browser has run.
	userDataDirectory string
	// hostDirectory is where the browser looks for NativeMessagingHosts; empty means userDataDirectory, as in stock Chromium.
	hostDirectory string
	// For Firefox: manifestDirectory holds the host manifests, and profilesFile exists once Firefox has run.
	manifestDirectory string
	profilesFile      string
}

// getKnownBrowsers lists Chromium browsers by their user data directory, and Firefox.
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
			{
				name: "Firefox", family: firefoxFamily,
				manifestDirectory: filepath.Join(support, "Mozilla", "NativeMessagingHosts"),
				profilesFile:      filepath.Join(support, "Firefox", "profiles.ini"),
			},
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
		{
			name: "Firefox", family: firefoxFamily,
			manifestDirectory: filepath.Join(homeDirectory, ".mozilla", "native-messaging-hosts"),
			profilesFile:      filepath.Join(homeDirectory, ".mozilla", "firefox", "profiles.ini"),
		},
	}
}

func (browser knownBrowser) getManifestPath() string {
	if browser.family == firefoxFamily {
		return filepath.Join(browser.manifestDirectory, browserprotocol.HostName+".json")
	}
	hostDirectory := browser.hostDirectory
	if hostDirectory == "" {
		hostDirectory = browser.userDataDirectory
	}
	return filepath.Join(hostDirectory, "NativeMessagingHosts", browserprotocol.HostName+".json")
}

// hasRun reports whether the browser has run on this account, so Settings lists it.
func (browser knownBrowser) hasRun() bool {
	marker := filepath.Join(browser.userDataDirectory, "Local State")
	if browser.family == firefoxFamily {
		marker = browser.profilesFile
	}
	_, err := os.Stat(marker)
	return err == nil
}

// buildHostManifest names the extension allowed to start the host as the family reads it: Chromium browsers by origin,
// Firefox by add-on ID.
func buildHostManifest(hostPath string, family browserFamily) ([]byte, error) {
	manifest := map[string]any{
		"name":        browserprotocol.HostName,
		"description": "Hisar password manager",
		"path":        hostPath,
		"type":        "stdio",
	}
	if family == firefoxFamily {
		manifest["allowed_extensions"] = []string{browserprotocol.FirefoxExtensionID}
	} else {
		manifest["allowed_origins"] = []string{browserprotocol.AllowedOrigin}
	}
	return json.MarshalIndent(manifest, "", "  ")
}

// InstallHostManifests writes the native host manifest for every known browser, so one installed later finds it at its
// first launch, and reports the browsers that have run. It only writes changed files.
func InstallHostManifests(homeDirectory, hostPath string) []browserprotocol.BrowserInstallation {
	return installHostManifests(homeDirectory, hostPath, runtime.GOOS)
}

func installHostManifests(homeDirectory, hostPath, operatingSystem string) []browserprotocol.BrowserInstallation {
	installations := []browserprotocol.BrowserInstallation{}
	for _, browser := range getKnownBrowsers(homeDirectory, operatingSystem) {
		installation := browserprotocol.BrowserInstallation{Name: browser.name, ManifestPath: browser.getManifestPath()}
		manifest, err := buildHostManifest(hostPath, browser.family)
		if err == nil {
			err = writeFileIfChanged(installation.ManifestPath, manifest)
		}
		if !browser.hasRun() {
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
	partialFile, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	_, err = partialFile.Write(content)
	if err == nil {
		err = partialFile.Chmod(0o644)
	}
	if closeErr := partialFile.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(partialFile.Name(), path)
	}
	if err != nil {
		os.Remove(partialFile.Name())
	}
	return err
}
