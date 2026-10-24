package browser

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tryhisar/hisar/core/browserprotocol"
)

func TestInstallHostManifestForFirefox(t *testing.T) {
	for _, test := range []struct {
		operatingSystem, manifestDirectory, profilesFile string
	}{
		{"darwin", "Library/Application Support/Mozilla/NativeMessagingHosts", "Library/Application Support/Firefox/profiles.ini"},
		{"linux", ".mozilla/native-messaging-hosts", ".mozilla/firefox/profiles.ini"},
	} {
		t.Run(test.operatingSystem, func(t *testing.T) {
			homeDirectory := t.TempDir()
			manifestPath := filepath.Join(homeDirectory, test.manifestDirectory, "com.tryhisar.hisar.json")
			// A Firefox that has never run gets the manifest for its first launch, but Settings does not list it.
			if installations := installHostManifests(homeDirectory, "/usr/lib/hisar/hisar-browser-host", test.operatingSystem); len(installations) != 0 {
				t.Fatalf("installations before Firefox ran: %+v", installations)
			}
			readFirefoxHostManifest(t, manifestPath, "/usr/lib/hisar/hisar-browser-host")

			profilesFile := filepath.Join(homeDirectory, test.profilesFile)
			if err := os.MkdirAll(filepath.Dir(profilesFile), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(profilesFile, []byte("[General]\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			installations := installHostManifests(homeDirectory, "/usr/lib/hisar/hisar-browser-host", test.operatingSystem)
			if len(installations) != 1 || installations[0].Name != "Firefox" || !installations[0].IsInstalled || installations[0].ManifestPath != manifestPath {
				t.Fatalf("installations: %+v", installations)
			}
		})
	}
}

func readFirefoxHostManifest(t *testing.T, path, hostPath string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(content, &manifest); err != nil {
		t.Fatal(err)
	}
	expected := map[string]any{
		"name": browserprotocol.HostName, "description": "Hisar password manager", "path": hostPath, "type": "stdio",
		"allowed_extensions": []any{"extension@tryhisar.com"},
	}
	if string(mustMarshal(t, manifest)) != string(mustMarshal(t, expected)) {
		t.Errorf("%s: manifest = %s", path, content)
	}
}

func TestReadCaller(t *testing.T) {
	for _, test := range []struct {
		arguments []string
		caller    string
	}{
		{[]string{"hisar-browser-host", "chrome-extension://mmipiffoogblblocokimpemdopmdjghf/"}, browserprotocol.AllowedOrigin},
		{[]string{"hisar-browser-host", "chrome-extension://mmipiffoogblblocokimpemdopmdjghf/", "--parent-window=0"}, browserprotocol.AllowedOrigin},
		{[]string{"hisar-browser-host", "/Users/alex/Library/Application Support/Mozilla/NativeMessagingHosts/com.tryhisar.hisar.json", "extension@tryhisar.com"}, browserprotocol.FirefoxExtensionID},
		{[]string{"hisar-browser-host"}, ""},
	} {
		if caller := ReadCaller(test.arguments); caller != test.caller {
			t.Errorf("ReadCaller(%q) = %q, want %q", test.arguments, caller, test.caller)
		}
	}
}

func TestServerAnswersFirefoxAndRefusesOtherAddOns(t *testing.T) {
	handler := &recordingHandler{}
	socketPath := startTestServer(t, handler, nil, "")
	request := encodeFrames(t, `{"id":"1","type":"status"}`)
	var output bytes.Buffer
	if err := RunHost(bytes.NewReader(request), &output, HostConfig{SocketPath: socketPath, ExtensionOrigin: browserprotocol.FirefoxExtensionID}); err != nil {
		t.Fatal(err)
	}
	if responses := decodeResponses(t, output.Bytes()); len(responses) != 1 || !responses[0].OK {
		t.Fatalf("Firefox got %q", output.String())
	}
	// Another add-on, or the manifest path a host started with Chrome's arguments would forward, gets nothing.
	for _, caller := range []string{"someone-else@example.com", "/Users/alex/Library/Application Support/Mozilla/NativeMessagingHosts/com.tryhisar.hisar.json"} {
		output.Reset()
		RunHost(bytes.NewReader(request), &output, HostConfig{SocketPath: socketPath, ExtensionOrigin: caller})
		if len(decodeResponses(t, output.Bytes())) != 0 {
			t.Errorf("%s got answers: %q", caller, output.String())
		}
	}
	if len(handler.requests) != 1 {
		t.Errorf("handled requests: %+v", handler.requests)
	}
}
