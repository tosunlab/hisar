package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/appversion"
	"github.com/tryhisar/hisar/core/browserprotocol"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/localstate"
)

type fakeVersionServer struct {
	mutex                  sync.Mutex
	minimumVersionByClient map[string]string
	checkedVersions        []string
}

func (server *fakeVersionServer) setMinimumVersion(clientName, minimumVersion string) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	server.minimumVersionByClient[clientName] = minimumVersion
}

func (server *fakeVersionServer) countCheck() int {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	return len(server.checkedVersions)
}

func (server *fakeVersionServer) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	server.mutex.Lock()
	clientName := apitypes.GetClient(request.Header.Get(apitypes.PlatformHeader))
	version := request.Header.Get(apitypes.VersionHeader)
	minimumVersion := server.minimumVersionByClient[clientName]
	if request.URL.Path == "/api/v1/version-check" {
		server.checkedVersions = append(server.checkedVersions, clientName+" "+version)
	}
	server.mutex.Unlock()
	if isOlder, err := appversion.IsOlder(version, minimumVersion); err == nil && isOlder {
		writer.WriteHeader(http.StatusUpgradeRequired)
		json.NewEncoder(writer).Encode(apitypes.UpgradeRequired{Error: "update required", MinimumVersion: minimumVersion, UpdateURL: "https://tryhisar.com/download"})
		return
	}
	if request.URL.Path == "/api/v1/version-check" {
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	writer.WriteHeader(http.StatusUnauthorized)
}

func createVersionTestService(t *testing.T, appVersion string, minimumVersionByClient map[string]string) (*Service, *fakeVersionServer, *httptest.Server, chan UpdateRequirement) {
	t.Helper()
	versionServer := &fakeVersionServer{minimumVersionByClient: minimumVersionByClient}
	httpServer := httptest.NewServer(versionServer)
	t.Cleanup(httpServer.Close)
	statePath := filepath.Join(t.TempDir(), "desktop.json")
	state, _ := localstate.Load(statePath)
	state.Server, state.Email, state.AccountID, state.Token = httpServer.URL, "a@example.com", "ABC123", "session-token"
	if err := state.Save(); err != nil {
		t.Fatal(err)
	}
	service, _, _ := createTestService(t, statePath, createMemorySecretKeyStore())
	service.config.Platform, service.config.AppVersion = apitypes.PlatformDesktopMacOS, appVersion
	heards := make(chan UpdateRequirement, 10)
	service.config.OnUpdateRequired = func(requirement UpdateRequirement) { heards <- requirement }
	return service, versionServer, httpServer, heards
}

func waitForUpdateRequired(t *testing.T, heards chan UpdateRequirement) UpdateRequirement {
	t.Helper()
	select {
	case requirement := <-heards:
		return requirement
	case <-time.After(5 * time.Second):
		t.Fatal("OnUpdateRequired was not called")
		return UpdateRequirement{}
	}
}

func TestAnyServerAnswerCanRequireAnUpdate(t *testing.T) {
	service, _, _, heards := createVersionTestService(t, "0.1.5", map[string]string{apitypes.ClientDesktopMacOS: "0.1.6"})
	if status := service.GetStatus(); status.UpdateRequired != nil {
		t.Fatalf("required before any server contact: %+v", status.UpdateRequired)
	}
	if _, err := service.ListSharesAwaitingUnlock(); err == nil {
		t.Error("the refused request succeeded")
	}
	expected := UpdateRequirement{CurrentVersion: "0.1.5", MinimumVersion: "0.1.6", UpdateURL: "https://tryhisar.com/download"}
	if heard := waitForUpdateRequired(t, heards); heard != expected {
		t.Errorf("OnUpdateRequired heard %+v", heard)
	}
	if status := service.GetStatus(); status.UpdateRequired == nil || *status.UpdateRequired != expected {
		t.Errorf("status: %+v", status.UpdateRequired)
	}
	service.ListSharesAwaitingUnlock()
	select {
	case heard := <-heards:
		t.Errorf("the same requirement was announced twice: %+v", heard)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestCheckMinimumVersion(t *testing.T) {
	service, versionServer, httpServer, heards := createVersionTestService(t, "0.1.5", map[string]string{apitypes.ClientDesktopMacOS: "0.1.6"})
	if status := service.CheckMinimumVersion(); status.UpdateRequired == nil || status.UpdateRequired.MinimumVersion != "0.1.6" {
		t.Fatalf("an old version passed: %+v", status.UpdateRequired)
	}
	waitForUpdateRequired(t, heards)
	if versionServer.checkedVersions[0] != "desktop-macos 0.1.5" {
		t.Errorf("checked %q", versionServer.checkedVersions)
	}

	versionServer.setMinimumVersion(apitypes.ClientDesktopMacOS, "0.1.0")
	if status := service.CheckMinimumVersion(); status.UpdateRequired != nil {
		t.Errorf("a lowered minimum still refuses: %+v", status.UpdateRequired)
	}

	versionServer.setMinimumVersion(apitypes.ClientDesktopMacOS, "0.1.6")
	service.CheckMinimumVersion()
	httpServer.Close()
	if status := service.CheckMinimumVersion(); status.UpdateRequired == nil {
		t.Error("going offline forgot the requirement; the check applies on the next contact")
	}
}

func TestOfflineKeepsWorkingWithoutARequirement(t *testing.T) {
	service, _, httpServer, _ := createVersionTestService(t, "0.1.5", map[string]string{apitypes.ClientDesktopMacOS: "0.1.6"})
	httpServer.Close()
	if status := service.CheckMinimumVersion(); status.UpdateRequired != nil {
		t.Errorf("unreachable server required an update: %+v", status.UpdateRequired)
	}
	if err := service.requireCurrentVersion(); err != nil {
		t.Errorf("refused offline: %v", err)
	}
}

func TestDevelopmentBuildSendsNoVersion(t *testing.T) {
	service, versionServer, _, _ := createVersionTestService(t, "", map[string]string{apitypes.ClientDesktopMacOS: "9.0.0"})
	if status := service.CheckMinimumVersion(); status.UpdateRequired != nil {
		t.Errorf("development build refused: %+v", status.UpdateRequired)
	}
	if versionServer.checkedVersions[0] != " " {
		t.Errorf("development build sent %q", versionServer.checkedVersions[0])
	}
}

func TestUpdateRequiredRefusesLocalRequests(t *testing.T) {
	service, _, _, heards := createVersionTestService(t, "0.1.5", map[string]string{apitypes.ClientDesktopMacOS: "0.1.6"})
	service.CheckMinimumVersion()
	waitForUpdateRequired(t, heards)

	_, err := NewSSHKeySource(service).ListKeys(context.Background())
	var updateRequired *UpdateRequiredError
	if !errors.As(err, &updateRequired) || err.Error() != "Hisar needs an update: the server requires version 0.1.6 or later. Open Hisar to update." {
		t.Errorf("SSH agent: %v", err)
	}
	if heard := waitForUpdateRequired(t, heards); heard.MinimumVersion != "0.1.6" {
		t.Errorf("the SSH request did not bring up the update screen: %+v", heard)
	}

	err = NewCommandLineBridge(service, clisocket.NewSessionApprovals(0, nil, nil)).CheckUpdateRequired(&cliprotocol.Request{Type: cliprotocol.TypeListVault})
	var responseError *cliprotocol.ResponseError
	if !errors.As(err, &responseError) || responseError.Code != cliprotocol.ErrorUpdateRequired ||
		responseError.Message != "Hisar needs an update: the server requires version 0.1.6 or later and this Mac has 0.1.5. Open Hisar and choose Update now." {
		t.Errorf("command-line tool: %v", err)
	}

	bridge := NewBrowserBridge(service, BrowserBridgeConfig{})
	hello := bridge.answerHello(&browserprotocol.Request{Type: "hello", ExtensionVersion: "0.2.0"})
	if hello.UpdateRequired == nil || *hello.UpdateRequired != (browserprotocol.UpdateRequirement{Target: browserprotocol.UpdateTargetApp, CurrentVersion: "0.1.5", MinimumVersion: "0.1.6", UpdateURL: "https://tryhisar.com/download"}) {
		t.Errorf("browser hello: %+v", hello.UpdateRequired)
	}
	service.mutex.Lock()
	browserTokenHash := sha256.Sum256([]byte("browser-token"))
	service.state.PairedBrowsers = append(service.state.PairedBrowsers, localstate.PairedBrowser{InstanceID: "i", TokenHash: browserTokenHash[:]})
	service.mutex.Unlock()
	_, err = bridge.handleRequest(context.Background(), &browserprotocol.Request{Type: "logins.match", Token: "browser-token", URL: "https://github.com"})
	var browserError *browserprotocol.ResponseError
	if !errors.As(err, &browserError) || browserError.Code != browserprotocol.ErrorUpdateRequired {
		t.Errorf("browser fill: %v", err)
	}
}

func TestOtherAppsAreCheckedThroughHisar(t *testing.T) {
	service, versionServer, _, _ := createVersionTestService(t, "0.1.6", map[string]string{apitypes.ClientDesktopMacOS: "0.1.6", apitypes.ClientExtension: "0.3.0", apitypes.ClientCLI: "1.0.0"})
	bridge := NewBrowserBridge(service, BrowserBridgeConfig{})
	hello := bridge.answerHello(&browserprotocol.Request{Type: "hello", ExtensionVersion: "0.2.0"})
	if hello.UpdateRequired == nil || hello.UpdateRequired.Target != browserprotocol.UpdateTargetExtension || hello.UpdateRequired.CurrentVersion != "0.2.0" || hello.UpdateRequired.MinimumVersion != "0.3.0" {
		t.Errorf("old extension: %+v", hello.UpdateRequired)
	}
	bridge.answerHello(&browserprotocol.Request{Type: "hello", ExtensionVersion: "0.2.0"})
	if versionServer.countCheck() != 1 {
		t.Errorf("the extension's version was checked %d times; the answer is kept for an hour", versionServer.countCheck())
	}
	if hello := bridge.answerHello(&browserprotocol.Request{Type: "hello", ExtensionVersion: "0.3.0"}); hello.UpdateRequired != nil {
		t.Errorf("current extension: %+v", hello.UpdateRequired)
	}
	if hello := bridge.answerHello(&browserprotocol.Request{Type: "hello"}); hello.UpdateRequired != nil {
		t.Errorf("extension without a version: %+v", hello.UpdateRequired)
	}

	commandLineBridge := NewCommandLineBridge(service, clisocket.NewSessionApprovals(0, nil, nil))
	err := commandLineBridge.CheckUpdateRequired(&cliprotocol.Request{Type: cliprotocol.TypeListVault, ClientPlatform: "darwin/cli", ClientVersion: "0.9.0"})
	var responseError *cliprotocol.ResponseError
	if !errors.As(err, &responseError) || responseError.Code != cliprotocol.ErrorUpdateRequired ||
		responseError.Message != "hisar needs an update: the server requires version 1.0.0 or later and this is 0.9.0. Install the latest hisar from https://tryhisar.com/download, or use the one that comes with the Hisar app." {
		t.Errorf("old hisar: %v", err)
	}
	for _, request := range []cliprotocol.Request{{ClientPlatform: "darwin/cli", ClientVersion: "1.0.0"}, {ClientPlatform: "darwin/cli"}, {}} {
		if err := commandLineBridge.CheckUpdateRequired(&request); err != nil {
			t.Errorf("%+v refused: %v", request, err)
		}
	}
	if service.GetStatus().UpdateRequired != nil {
		t.Error("an old extension or hisar made the app require an update")
	}
}

func TestParseInviteDeepLink(t *testing.T) {
	for link, expected := range map[string]string{
		"hisar://invite/abc_DEF-123":                                             "abc_DEF-123",
		" hisar://invite/abc_DEF-123?server=https%3A%2F%2Fapi.tryhisar.com ":     "abc_DEF-123",
		"HISAR://INVITE/abc?server=https%3A%2F%2Fhisar.example.com":              "https://hisar.example.com/invite/abc",
		"hisar://invite/abc?server=http%3A%2F%2F127.0.0.1%3A19400":               "http://127.0.0.1:19400/invite/abc",
		"hisar://invite/abc?server=hisar.example.com":                            "https://hisar.example.com/invite/abc",
		"hisar://invite/abc?server=https%3A%2F%2Fhisar.example.com%2F&other=yes": "https://hisar.example.com/invite/abc",
	} {
		if invite, err := ParseInviteDeepLink(link); err != nil || invite != expected {
			t.Errorf("ParseInviteDeepLink(%q) = %q, %v; want %q", link, invite, err, expected)
		}
	}
	for _, link := range []string{"", "hisar://invite/", "hisar://invite/a/b", "hisar://sign-in?server=x", "https://api.tryhisar.com/invite/abc", "hisar://invite/a%20b", "hisar://invite/abc?server=ftp%3A%2F%2Fx"} {
		if invite, err := ParseInviteDeepLink(link); err == nil {
			t.Errorf("ParseInviteDeepLink(%q) = %q", link, invite)
		}
	}
	service := NewInviteLinkService()
	if invite, err := OpenInviteLink(service, "hisar://invite/abc"); err != nil || invite != "abc" {
		t.Fatalf("OpenInviteLink: %q %v", invite, err)
	}
	if first, second := service.TakePendingInvite(), service.TakePendingInvite(); first != "abc" || second != "" {
		t.Errorf("TakePendingInvite: %q then %q", first, second)
	}
}

func TestUpgradeRequiredStopsTheEventStream(t *testing.T) {
	service, _, httpServer, _ := createVersionTestService(t, "0.1.5", map[string]string{apitypes.ClientDesktopMacOS: "0.1.6"})
	finished := make(chan struct{})
	go func() {
		service.watchEvents(context.Background(), service.createClient(httpServer.URL), make(chan struct{}))
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("the event stream kept reconnecting to a server that refuses this version")
	}
	if _, isUpgradeRequired := apiclient.AsUpgradeRequired(service.requireCurrentVersion()); isUpgradeRequired {
		t.Error("requireCurrentVersion returned the client error instead of UpdateRequiredError")
	}
}
