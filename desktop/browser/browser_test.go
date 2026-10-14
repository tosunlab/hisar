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
	chromeDirectory := filepath.Join(homeDirectory, "Library", "Application Support", "Google", "Chrome")
	braveDirectory := filepath.Join(homeDirectory, "Library", "Application Support", "BraveSoftware", "Brave-Browser")
	for _, directory := range []string{chromeDirectory, braveDirectory} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	allInstallation := installAllHostManifest(homeDirectory, "/Applications/Hisar.app/Contents/MacOS/hisar-browser-host", "darwin")
	if len(allInstallation) != 2 || !allInstallation[0].IsInstalled || allInstallation[0].Name != "Google Chrome" || allInstallation[1].Name != "Brave" {
		t.Fatalf("installations: %+v", allInstallation)
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
	manifestPath := allInstallation[0].ManifestPath
	before, _ := os.Stat(manifestPath)
	time.Sleep(20 * time.Millisecond)
	installAllHostManifest(homeDirectory, "/Applications/Hisar.app/Contents/MacOS/hisar-browser-host", "darwin")
	if after, _ := os.Stat(manifestPath); !after.ModTime().Equal(before.ModTime()) {
		t.Error("an unchanged manifest was rewritten")
	}
	if allLinux := installAllHostManifest(homeDirectory, "/usr/bin/hisar-browser-host", "linux"); len(allLinux) != 0 {
		t.Errorf("linux browsers found in a macOS home: %+v", allLinux)
	}
}

type recordingHandler struct {
	allRequest []browserprotocol.Request
}

func (handler *recordingHandler) HandleRequest(ctx context.Context, request *browserprotocol.Request) (any, error) {
	handler.allRequest = append(handler.allRequest, *request)
	switch request.Type {
	case "status":
		return browserprotocol.StatusResult{State: "unlocked", Email: "alice@example.com"}, nil
	case "unlock":
		return nil, nil
	case "logins.fill":
		return nil, browserprotocol.CreateError(browserprotocol.ErrorNoMatch, "this login is not for this site")
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
	server := CreateServer(handler, peerResolver, allowedPeerPath)
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

func encodeAllFrame(t *testing.T, allMessage ...string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	for _, message := range allMessage {
		if err := browserprotocol.WriteMessage(&buffer, []byte(message)); err != nil {
			t.Fatal(err)
		}
	}
	return buffer.Bytes()
}

func decodeAllResponse(t *testing.T, output []byte) []browserprotocol.Response {
	t.Helper()
	reader := bytes.NewReader(output)
	allResponse := []browserprotocol.Response{}
	for {
		message, err := browserprotocol.ReadMessage(reader)
		if errors.Is(err, io.EOF) {
			return allResponse
		}
		if err != nil {
			t.Fatal(err)
		}
		var response browserprotocol.Response
		if err := json.Unmarshal(message, &response); err != nil {
			t.Fatal(err)
		}
		allResponse = append(allResponse, response)
	}
}

func TestHostRelaysThroughServer(t *testing.T) {
	handler := &recordingHandler{}
	socketPath := startTestServer(t, handler, nil, "")
	input := encodeAllFrame(t,
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
	allResponse := decodeAllResponse(t, output.Bytes())
	if len(allResponse) != 5 {
		t.Fatalf("responses: %+v", allResponse)
	}
	if !allResponse[0].OK || allResponse[0].ID != "1" {
		t.Errorf("status response: %+v", allResponse[0])
	}
	if allResponse[1].OK || allResponse[1].Error.Code != browserprotocol.ErrorNoMatch {
		t.Errorf("fill response: %+v", allResponse[1])
	}
	if allResponse[2].Error == nil || allResponse[2].Error.Code != browserprotocol.ErrorInternal {
		t.Errorf("handler error: %+v", allResponse[2])
	}
	if allResponse[3].Error == nil || allResponse[3].Error.Code != browserprotocol.ErrorInvalidRequest {
		t.Errorf("invalid JSON: %+v", allResponse[3])
	}
	if !allResponse[4].OK || string(mustMarshal(t, allResponse[4].Result)) != "{}" {
		t.Errorf("empty result: %+v", allResponse[4])
	}
	if len(handler.allRequest) != 4 || handler.allRequest[0].Token != "secret" {
		t.Errorf("handled requests: %+v", handler.allRequest)
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
	input := encodeAllFrame(t, `{"id":"1","type":"status"}`)
	RunHost(bytes.NewReader(input), &output, HostConfig{SocketPath: socketPath, ExtensionOrigin: "chrome-extension://someoneelse/"})
	if len(decodeAllResponse(t, output.Bytes())) != 0 || len(handler.allRequest) != 0 {
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
	RunHost(bytes.NewReader(encodeAllFrame(t, `{"id":"1","type":"status"}`)), &output, HostConfig{SocketPath: socketPath, ExtensionOrigin: browserprotocol.AllowedOrigin})
	if len(handler.allRequest) != 0 {
		t.Error("an unknown process was served")
	}

	allowedSocketPath := startTestServer(t, handler, fakePeerResolver{path: hostPath}, hostPath)
	output.Reset()
	RunHost(bytes.NewReader(encodeAllFrame(t, `{"id":"1","type":"status"}`)), &output, HostConfig{SocketPath: allowedSocketPath, ExtensionOrigin: browserprotocol.AllowedOrigin})
	if allResponse := decodeAllResponse(t, output.Bytes()); len(allResponse) != 1 || !allResponse[0].OK {
		t.Errorf("the packaged host was refused: %+v", allResponse)
	}
}

func TestHostAnswersUnavailableAndLaunchesApp(t *testing.T) {
	missingSocket := filepath.Join(t.TempDir(), "missing.sock")
	launchCount := 0
	var output bytes.Buffer
	input := encodeAllFrame(t, `{"id":"a","type":"hello"}`, `{"id":"b","type":"status"}`)
	err := RunHost(bytes.NewReader(input), &output, HostConfig{
		SocketPath: missingSocket, ExtensionOrigin: browserprotocol.AllowedOrigin,
		LaunchApp: func() error { launchCount++; return nil }, RetryFor: 50 * time.Millisecond, RetryEvery: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	allResponse := decodeAllResponse(t, output.Bytes())
	if launchCount != 1 || len(allResponse) != 2 || allResponse[1].ID != "b" || allResponse[1].Error.Code != browserprotocol.ErrorDesktopUnavailable {
		t.Errorf("launches %d, responses %+v", launchCount, allResponse)
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
		go CreateServer(handler, nil, "").Serve(ctx, socketPath)
		return nil
	}
	var output bytes.Buffer
	err = RunHost(bytes.NewReader(encodeAllFrame(t, `{"id":"1","type":"status"}`)), &output, HostConfig{
		SocketPath: socketPath, ExtensionOrigin: browserprotocol.AllowedOrigin, LaunchApp: launchApp, RetryFor: 2 * time.Second, RetryEvery: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if allResponse := decodeAllResponse(t, output.Bytes()); len(allResponse) != 1 || !allResponse[0].OK || !strings.Contains(string(mustMarshal(t, allResponse[0].Result)), "alice@example.com") {
		t.Errorf("responses after launch: %+v", allResponse)
	}
}
