package browser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/browserprotocol"
	"github.com/tryhisar/hisar/core/localsocket"
)

func TestInstallAllHostManifest(t *testing.T) {
	homeDirectory := t.TempDir()
	support := filepath.Join(homeDirectory, "Library", "Application Support")
	chromeDirectory := filepath.Join(support, "Google", "Chrome")
	braveDirectory := filepath.Join(support, "BraveSoftware", "Brave-Browser")
	diaDirectory := filepath.Join(support, "Dia", "User Data")
	for _, directory := range []string{chromeDirectory, braveDirectory, diaDirectory} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "Local State"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Another password manager's registration leaves a browser directory without Local State: that browser has not run.
	if err := os.MkdirAll(filepath.Join(support, "Microsoft Edge", "NativeMessagingHosts"), 0o755); err != nil {
		t.Fatal(err)
	}
	installations := installHostManifests(homeDirectory, "/Applications/Hisar.app/Contents/MacOS/hisar-browser-host", "darwin")
	if len(installations) != 3 || !installations[0].IsInstalled || installations[0].Name != "Google Chrome" || installations[1].Name != "Brave" || installations[2].Name != "Dia" ||
		installations[2].ManifestPath != filepath.Join(diaDirectory, "NativeMessagingHosts", "com.tryhisar.hisar.json") {
		t.Fatalf("installations: %+v", installations)
	}
	content, err := os.ReadFile(filepath.Join(chromeDirectory, "NativeMessagingHosts", "com.tryhisar.hisar.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Name           string   `json:"name"`
		Path           string   `json:"path"`
		Type           string   `json:"type"`
		AllowedOrigins []string `json:"allowed_origins"`
	}
	if err := json.Unmarshal(content, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Name != browserprotocol.HostName || manifest.Type != "stdio" || manifest.Path != "/Applications/Hisar.app/Contents/MacOS/hisar-browser-host" ||
		len(manifest.AllowedOrigins) != 1 || manifest.AllowedOrigins[0] != "chrome-extension://mmipiffoogblblocokimpemdopmdjghf/" {
		t.Errorf("manifest = %+v", manifest)
	}
	// Browsers that have not run, or are not installed at all, get the manifest too, so they find it at their first launch.
	for _, directory := range []string{filepath.Join(support, "Microsoft Edge"), filepath.Join(support, "Arc", "User Data"), filepath.Join(support, "com.operasoftware.Opera")} {
		if prepared, err := os.ReadFile(filepath.Join(directory, "NativeMessagingHosts", "com.tryhisar.hisar.json")); err != nil || !bytes.Equal(prepared, content) {
			t.Errorf("%s: manifest not prepared: %v", directory, err)
		}
	}
	manifestPath := installations[0].ManifestPath
	before, _ := os.Stat(manifestPath)
	time.Sleep(20 * time.Millisecond)
	installHostManifests(homeDirectory, "/Applications/Hisar.app/Contents/MacOS/hisar-browser-host", "darwin")
	if after, _ := os.Stat(manifestPath); !after.ModTime().Equal(before.ModTime()) {
		t.Error("an unchanged manifest was rewritten")
	}
	if linuxes := installHostManifests(homeDirectory, "/usr/bin/hisar-browser-host", "linux"); len(linuxes) != 0 {
		t.Errorf("linux browsers found in a macOS home: %+v", linuxes)
	}
}

type recordingHandler struct {
	requests []browserprotocol.Request
}

func (handler *recordingHandler) HandleRequest(ctx context.Context, request *browserprotocol.Request) (any, error) {
	handler.requests = append(handler.requests, *request)
	switch request.Type {
	case "status":
		return browserprotocol.StatusResult{State: "unlocked", Email: "alice@example.com"}, nil
	case "unlock":
		return nil, nil
	case "logins.fill":
		return nil, browserprotocol.NewError(browserprotocol.ErrorNoMatch, "this login is not for this site")
	}
	return nil, errors.New("boom")
}

type fakePeerResolver struct {
	path string
}

func (resolver fakePeerResolver) ResolvePeer(net.Conn) (localsocket.PeerInfo, error) {
	return localsocket.PeerInfo{Path: resolver.path}, nil
}

func startTestServer(t *testing.T, handler browserprotocol.Handler, peerResolver localsocket.PeerResolver, allowedPeerPath string) string {
	t.Helper()
	socketDirectory, err := os.MkdirTemp("/tmp", "hisar-browser-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socketDirectory) })
	socketPath := filepath.Join(socketDirectory, "browser.sock")
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	server := NewServer(handler, peerResolver, allowedPeerPath)
	go func() {
		server.Serve(ctx, socketPath)
		close(stopped)
	}()
	t.Cleanup(func() {
		cancel()
		<-stopped
	})
	for range 100 {
		if _, err := os.Stat(socketPath); err == nil {
			return socketPath
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("socket did not appear")
	return ""
}

func encodeFrames(t *testing.T, messages ...string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	for _, message := range messages {
		if err := browserprotocol.WriteMessage(&buffer, []byte(message)); err != nil {
			t.Fatal(err)
		}
	}
	return buffer.Bytes()
}

func decodeResponses(t *testing.T, output []byte) []browserprotocol.Response {
	t.Helper()
	reader := bytes.NewReader(output)
	responses := []browserprotocol.Response{}
	for {
		message, err := browserprotocol.ReadMessage(reader)
		if errors.Is(err, io.EOF) {
			return responses
		}
		if err != nil {
			t.Fatal(err)
		}
		var response browserprotocol.Response
		if err := json.Unmarshal(message, &response); err != nil {
			t.Fatal(err)
		}
		responses = append(responses, response)
	}
}

func TestHostRelaysThroughServer(t *testing.T) {
	handler := &recordingHandler{}
	socketPath := startTestServer(t, handler, nil, "")
	input := encodeFrames(t,
		`{"id":"1","type":"status","token":"secret"}`,
		`{"id":"2","type":"logins.fill","url":"https://evil.example"}`,
		`{"id":"3","type":"explode"}`,
		`not json`,
		`{"id":"5","type":"unlock"}`,
	)
	var output bytes.Buffer
	if err := RunHost(bytes.NewReader(input), &output, HostConfig{SocketPath: socketPath, ExtensionOrigin: browserprotocol.AllowedOrigin}); err != nil {
		t.Fatal(err)
	}
	responses := decodeResponses(t, output.Bytes())
	if len(responses) != 5 {
		t.Fatalf("responses: %+v", responses)
	}
	if !responses[0].OK || responses[0].ID != "1" {
		t.Errorf("status response: %+v", responses[0])
	}
	if responses[1].OK || responses[1].Error.Code != browserprotocol.ErrorNoMatch {
		t.Errorf("fill response: %+v", responses[1])
	}
	if responses[2].Error == nil || responses[2].Error.Code != browserprotocol.ErrorInternal {
		t.Errorf("handler error: %+v", responses[2])
	}
	if responses[3].Error == nil || responses[3].Error.Code != browserprotocol.ErrorInvalidRequest {
		t.Errorf("invalid JSON: %+v", responses[3])
	}
	if !responses[4].OK || string(mustMarshal(t, responses[4].Result)) != "{}" {
		t.Errorf("empty result: %+v", responses[4])
	}
	if len(handler.requests) != 4 || handler.requests[0].Token != "secret" {
		t.Errorf("handled requests: %+v", handler.requests)
	}
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestServerRefusesOtherOrigins(t *testing.T) {
	handler := &recordingHandler{}
	socketPath := startTestServer(t, handler, nil, "")
	var output bytes.Buffer
	input := encodeFrames(t, `{"id":"1","type":"status"}`)
	RunHost(bytes.NewReader(input), &output, HostConfig{SocketPath: socketPath, ExtensionOrigin: "chrome-extension://someoneelse/"})
	if len(decodeResponses(t, output.Bytes())) != 0 || len(handler.requests) != 0 {
		t.Errorf("another extension got answers: %q", output.String())
	}
}

func TestServerRefusesUnknownPeer(t *testing.T) {
	hostPath := filepath.Join(t.TempDir(), "hisar-browser-host")
	if err := os.WriteFile(hostPath, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	handler := &recordingHandler{}
	socketPath := startTestServer(t, handler, fakePeerResolver{path: "/usr/bin/python3"}, hostPath)
	var output bytes.Buffer
	RunHost(bytes.NewReader(encodeFrames(t, `{"id":"1","type":"status"}`)), &output, HostConfig{SocketPath: socketPath, ExtensionOrigin: browserprotocol.AllowedOrigin})
	if len(handler.requests) != 0 {
		t.Error("an unknown process was served")
	}

	allowedSocketPath := startTestServer(t, handler, fakePeerResolver{path: hostPath}, hostPath)
	output.Reset()
	RunHost(bytes.NewReader(encodeFrames(t, `{"id":"1","type":"status"}`)), &output, HostConfig{SocketPath: allowedSocketPath, ExtensionOrigin: browserprotocol.AllowedOrigin})
	if responses := decodeResponses(t, output.Bytes()); len(responses) != 1 || !responses[0].OK {
		t.Errorf("the packaged host was refused: %+v", responses)
	}
}

func TestHostAnswersUnavailableAndLaunchesApp(t *testing.T) {
	missingSocket := filepath.Join(t.TempDir(), "missing.sock")
	launchCount := 0
	var output bytes.Buffer
	input := encodeFrames(t, `{"id":"a","type":"hello"}`, `{"id":"b","type":"status"}`)
	err := RunHost(bytes.NewReader(input), &output, HostConfig{
		SocketPath: missingSocket, ExtensionOrigin: browserprotocol.AllowedOrigin,
		LaunchApp: func() error { launchCount++; return nil }, RetryFor: 50 * time.Millisecond, RetryEvery: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	responses := decodeResponses(t, output.Bytes())
	if launchCount != 1 || len(responses) != 2 || responses[1].ID != "b" || responses[1].Error.Code != browserprotocol.ErrorDesktopUnavailable {
		t.Errorf("launches %d, responses %+v", launchCount, responses)
	}
}

func TestHostConnectsOnceTheLaunchedAppListens(t *testing.T) {
	handler := &recordingHandler{}
	socketDirectory, err := os.MkdirTemp("/tmp", "hisar-browser-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(socketDirectory)
	socketPath := filepath.Join(socketDirectory, "browser.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	launchApp := func() error {
		go NewServer(handler, nil, "").Serve(ctx, socketPath)
		return nil
	}
	var output bytes.Buffer
	err = RunHost(bytes.NewReader(encodeFrames(t, `{"id":"1","type":"status"}`)), &output, HostConfig{
		SocketPath: socketPath, ExtensionOrigin: browserprotocol.AllowedOrigin, LaunchApp: launchApp, RetryFor: 2 * time.Second, RetryEvery: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if responses := decodeResponses(t, output.Bytes()); len(responses) != 1 || !responses[0].OK || !strings.Contains(string(mustMarshal(t, responses[0].Result)), "alice@example.com") {
		t.Errorf("responses after launch: %+v", responses)
	}
}
