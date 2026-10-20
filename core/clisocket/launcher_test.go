package clisocket

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/agentharness"
	"github.com/tryhisar/hisar/core/cliprotocol"
)

type fakeLauncherApprover struct {
	mutex        sync.Mutex
	requests     []LauncherApprovalRequest
	result       error
	release      chan struct{}
	requestCount atomic.Int32
}

func (approver *fakeLauncherApprover) ApproveLauncher(ctx context.Context, request LauncherApprovalRequest) error {
	approver.requestCount.Add(1)
	approver.mutex.Lock()
	approver.requests = append(approver.requests, request)
	release, result := approver.release, approver.result
	approver.mutex.Unlock()
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return ErrDenied
		}
	}
	return result
}

func (approver *fakeLauncherApprover) setResult(result error) {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	approver.result = result
}

// connectLauncher connects from app outside a terminal, sends launcher.hello and returns its isApproved.
func (fixture *serverFixture) connectLauncher(t *testing.T, app AppIdentity) (*testClient, bool) {
	t.Helper()
	fixture.resolver.setCaller(launchdSession, app)
	client := fixture.connect(t)
	var hello cliprotocol.LauncherHelloResult
	readResult(t, client.send(t, cliprotocol.Request{Type: cliprotocol.TypeLauncherHello}), &hello)
	return client, hello.IsApproved
}

func (client *testClient) mustSend(t *testing.T, request cliprotocol.Request) {
	t.Helper()
	if response := client.send(t, request); response.Error != nil {
		t.Fatalf("%s: %+v", request.Type, response.Error)
	}
}

var listLauncherItems = cliprotocol.Request{Type: cliprotocol.TypeLauncherListItem}

func TestLauncherIsApprovedPerAppUntilHisarLocks(t *testing.T) {
	fixture := createServerFixture(t)
	launcher, _ := fixture.connectLauncher(t, raycastApp)
	launcher.mustSend(t, listLauncherItems)
	launcher.mustSend(t, listLauncherItems)
	if count := fixture.launcherApprover.requestCount.Load(); count != 1 {
		t.Fatalf("%d launcher approvals; want 1", count)
	}
	request := fixture.launcherApprover.requests[0]
	if request.App != raycastApp || request.ClientPID != 1234 || request.ParentPID != 4321 || request.ClientPath != "/Applications/Hisar.app/Contents/Helpers/hisar" {
		t.Errorf("launcher approval request: %+v", request)
	}

	fixture.advance(DefaultApprovalLifetime + time.Minute)
	launcher.mustSend(t, listLauncherItems)
	fixture.advance(8 * time.Hour)
	other, isApproved := fixture.connectLauncher(t, raycastApp)
	other.mustSend(t, listLauncherItems)
	if count := fixture.launcherApprover.requestCount.Load(); count != 1 || !isApproved {
		t.Errorf("the approval ended while Hisar stayed unlocked: %d approvals, launcher.hello isApproved %v", count, isApproved)
	}

	fixture.approvals.ClearAll()
	launcher.mustSend(t, listLauncherItems)
	if count := fixture.launcherApprover.requestCount.Load(); count != 2 {
		t.Errorf("the approval outlived the lock: %d approvals", count)
	}
	if fixture.approver.requestCount.Load() != 0 {
		t.Error("a launcher opened the command-line dialog")
	}
}

func TestRevokedLauncherIsAskedAgain(t *testing.T) {
	fixture := createServerFixture(t)
	raycast, _ := fixture.connectLauncher(t, raycastApp)
	raycast.mustSend(t, listLauncherItems)
	approvedAt := fixture.getNow()
	fixture.advance(time.Minute)
	tower, _ := fixture.connectLauncher(t, towerApp)
	tower.mustSend(t, listLauncherItems)
	expected := []LauncherApproval{{App: towerApp, LastUsedAt: approvedAt.Add(time.Minute)}, {App: raycastApp, LastUsedAt: approvedAt}}
	if launchers := fixture.approvals.ListLaunchers(); !slices.Equal(launchers, expected) {
		t.Fatalf("launchers %+v", launchers)
	}

	fixture.approvals.RevokeLauncher(raycastApp.Path)
	fixture.approvals.RevokeLauncher(raycastApp.Path)
	if launchers := fixture.approvals.ListLaunchers(); !slices.Equal(launchers, expected[:1]) {
		t.Errorf("launchers after revoking Raycast %+v", launchers)
	}
	raycast.mustSend(t, listLauncherItems)
	tower.mustSend(t, listLauncherItems)
	if count := fixture.launcherApprover.requestCount.Load(); count != 3 {
		t.Errorf("%d launcher approvals; want Raycast asked again and Tower not", count)
	}
}

func TestLauncherApprovalAnsweredAfterLockIsNotRemembered(t *testing.T) {
	fixture := createServerFixture(t)
	fixture.launcherApprover.release = make(chan struct{})
	launcher, _ := fixture.connectLauncher(t, raycastApp)
	done := make(chan cliprotocol.Response)
	go func() { done <- launcher.send(t, listLauncherItems) }()
	for fixture.launcherApprover.requestCount.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	fixture.approvals.ClearAll()
	close(fixture.launcherApprover.release)
	<-done
	if _, isApproved := fixture.connectLauncher(t, raycastApp); isApproved || len(fixture.approvals.ListLaunchers()) != 0 {
		t.Error("an approval that finished after a lock was remembered")
	}
}

func TestLauncherAndCommandLineApprovalsOfOneAppNeverCoverEachOther(t *testing.T) {
	listVaults := cliprotocol.Request{Type: cliprotocol.TypeListVault}

	launcherFirst := createServerFixture(t)
	launcher, _ := launcherFirst.connectLauncher(t, raycastApp)
	launcher.mustSend(t, listLauncherItems)
	launcherFirst.resolver.setCaller(launchdSession, raycastApp)
	commandLine := launcherFirst.connect(t)
	if commandLine.isApproved(t) {
		t.Error("hello says a command line in Raycast is approved by its launcher approval")
	}
	commandLine.mustSend(t, listVaults)
	if count := launcherFirst.approver.requestCount.Load(); count != 1 {
		t.Errorf("the launcher approval covered a command line in the same app (%d command-line approvals)", count)
	}

	commandLineFirst := createServerFixture(t)
	commandLineFirst.resolver.setCaller(launchdSession, raycastApp)
	commandLineFirst.connect(t).mustSend(t, listVaults)
	launcher, isApproved := commandLineFirst.connectLauncher(t, raycastApp)
	launcher.mustSend(t, listLauncherItems)
	if count := commandLineFirst.launcherApprover.requestCount.Load(); count != 1 || isApproved {
		t.Errorf("the command-line approval covered the launcher in the same app (%d launcher approvals, isApproved %v)", count, isApproved)
	}
}

func TestLauncherHelloNeedsAVerifiedAppOutsideATerminal(t *testing.T) {
	fixture := createServerFixture(t)
	expectRefused := func(caller string, client *testClient) {
		t.Helper()
		response := client.send(t, cliprotocol.Request{Type: cliprotocol.TypeLauncherHello})
		if getErrorCode(response) != cliprotocol.ErrorDenied || response.Error.Message != "Hisar answers launchers only in an app macOS can verify, outside a terminal" {
			t.Errorf("%s: launcher.hello %+v", caller, response.Error)
		}
		if code := getErrorCode(client.send(t, listLauncherItems)); code != cliprotocol.ErrorInvalidRequest {
			t.Errorf("%s: a refused launcher sent launcher.items.list: %q", caller, code)
		}
	}
	expectRefused("a terminal in a verified app", fixture.connect(t))
	fixture.resolver.setCaller(launchdSession, AppIdentity{})
	expectRefused("no terminal and no verified app", fixture.connect(t))

	launcher, isApproved := fixture.connectLauncher(t, raycastApp)
	if isApproved {
		t.Error("launcher.hello says an app nobody approved is approved")
	}
	launcher.mustSend(t, listLauncherItems)
	if _, isApproved := fixture.connectLauncher(t, raycastApp); !isApproved {
		t.Error("launcher.hello does not report the approval")
	}
	fixture.handler.setState(cliprotocol.StateLocked)
	if _, isApproved := fixture.connectLauncher(t, raycastApp); isApproved {
		t.Error("launcher.hello says approved while Hisar is locked")
	}
	if count := fixture.launcherApprover.requestCount.Load(); count != 1 {
		t.Errorf("%d launcher approvals; launcher.hello must never ask", count)
	}
}

func TestLauncherRefusedFromAgentSessionAndAgentHost(t *testing.T) {
	fixture := createServerFixture(t)
	raycast, _ := fixture.connectLauncher(t, raycastApp)
	raycast.mustSend(t, listLauncherItems)

	harness, _ := startHarness(t)
	fixture.resolver.setCaller(launchdSession, raycastApp)
	fixture.resolver.setAgentSession(harness)
	response := fixture.connect(t).send(t, cliprotocol.Request{Type: cliprotocol.TypeLauncherHello})
	if getErrorCode(response) != cliprotocol.ErrorDenied || response.Error.Message != "Hisar answers no launcher inside omp" {
		t.Errorf("inside omp: %+v", response.Error)
	}

	fixture.resolver.setAgentSession(agentharness.Session{})
	response = fixture.connect(t).send(t, cliprotocol.Request{Type: cliprotocol.TypeLauncherHello})
	if getErrorCode(response) != cliprotocol.ErrorDenied || response.Error.Message != "Hisar answers no launcher in an app that ran a coding agent" {
		t.Errorf("in an agent host: %+v", response.Error)
	}
	if response := raycast.send(t, listLauncherItems); getErrorCode(response) != cliprotocol.ErrorDenied || response.Error.Message != "Hisar answers no launcher in an app that ran a coding agent" {
		t.Errorf("the launcher kept working after omp ran in its app: %+v", response.Error)
	}
	if count := fixture.launcherApprover.requestCount.Load(); count != 1 {
		t.Errorf("%d launcher approvals; want 1", count)
	}
}

func TestLauncherConnectionSendsOnlyLauncherRequests(t *testing.T) {
	fixture := createServerFixture(t)
	launcher, _ := fixture.connectLauncher(t, raycastApp)
	for _, request := range []cliprotocol.Request{
		{Type: cliprotocol.TypeListVault},
		{Type: cliprotocol.TypeGetItem, VaultID: "v1", ItemID: "i1"},
		{Type: cliprotocol.TypeAgentListVault},
		{Type: cliprotocol.TypeAgentHello, AgentName: "claude-code"},
		{Type: cliprotocol.TypeLauncherHello},
		{Type: "launcher.items.delete", VaultID: "v1", ItemID: "i1"},
	} {
		if code := getErrorCode(launcher.send(t, request)); code != cliprotocol.ErrorInvalidRequest {
			t.Errorf("a launcher sent %s: %q", request.Type, code)
		}
	}

	commandLine := fixture.connect(t)
	agent := fixture.connect(t)
	agent.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeAgentHello, AgentName: "claude-code"})
	// The command line runs in Raycast too, so launcher.hello would make it a launcher; an agent refuses it.
	for _, crossed := range []struct {
		name         string
		client       *testClient
		requestTypes []string
	}{
		{"command line", commandLine, []string{cliprotocol.TypeLauncherListItem, cliprotocol.TypeLauncherLock, "launcher.items.delete"}},
		{"agent", agent, []string{cliprotocol.TypeLauncherListItem, cliprotocol.TypeLauncherLock, cliprotocol.TypeLauncherHello}},
	} {
		for _, requestType := range crossed.requestTypes {
			if code := getErrorCode(crossed.client.send(t, cliprotocol.Request{Type: requestType})); code != cliprotocol.ErrorInvalidRequest {
				t.Errorf("a %s connection sent %s: %q", crossed.name, requestType, code)
			}
		}
	}
	if len(fixture.handler.types) != 0 || fixture.launcherApprover.requestCount.Load() != 0 || fixture.approver.requestCount.Load() != 0 {
		t.Errorf("a crossed request asked or reached the handler: %q", fixture.handler.types)
	}
}

func TestGenerateAndLockNeedNeitherApprovalNorSignIn(t *testing.T) {
	fixture := createServerFixture(t)
	fixture.handler.setState(cliprotocol.StateSignedOut)
	launcher, _ := fixture.connectLauncher(t, raycastApp)
	launcher.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeLauncherGeneratePassword, Generator: &cliprotocol.GeneratorChange{Length: 20, Digits: true}})
	launcher.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeLauncherLock})
	if code := getErrorCode(launcher.send(t, listLauncherItems)); code != cliprotocol.ErrorSignedOut {
		t.Errorf("launcher.items.list signed out: %q", code)
	}
	if fixture.launcherApprover.requestCount.Load() != 0 {
		t.Error("generating, locking or a signed-out list asked for an approval")
	}
	if !slices.Equal(fixture.handler.types, []string{cliprotocol.TypeLauncherGeneratePassword, cliprotocol.TypeLauncherLock}) {
		t.Errorf("handled %q", fixture.handler.types)
	}

	fixture.handler.setState(cliprotocol.StateLocked)
	launcher.mustSend(t, listLauncherItems)
	if fixture.launcherApprover.requestCount.Load() != 1 {
		t.Error("a locked Hisar did not open the approval window, which unlocks it")
	}
	fixture.isEnabled.Store(false)
	if code := getErrorCode(launcher.send(t, cliprotocol.Request{Type: cliprotocol.TypeLauncherLock})); code != cliprotocol.ErrorIntegrationDisabled {
		t.Errorf("launcher.lock with the switch off: %q", code)
	}
}

func TestDeniedLauncherIsNotRemembered(t *testing.T) {
	fixture := createServerFixture(t)
	fixture.launcherApprover.setResult(TimeoutError{Timeout: time.Minute})
	launcher, _ := fixture.connectLauncher(t, raycastApp)
	response := launcher.send(t, listLauncherItems)
	if getErrorCode(response) != cliprotocol.ErrorDenied || response.Error.Message != "Hisar denied the request (it was not answered within a minute)" {
		t.Fatalf("unanswered: %+v", response.Error)
	}
	if len(fixture.handler.types) != 0 {
		t.Errorf("the handler ran after a denial: %q", fixture.handler.types)
	}
	fixture.launcherApprover.setResult(nil)
	launcher.mustSend(t, listLauncherItems)
	if count := fixture.launcherApprover.requestCount.Load(); count != 2 {
		t.Errorf("%d launcher approvals; a denial must not be remembered", count)
	}
}

func TestLauncherRequestsAreCheckedBeforeAsking(t *testing.T) {
	fixture := createServerFixture(t)
	launcher, _ := fixture.connectLauncher(t, raycastApp)
	shareLink := &cliprotocol.ShareLinkChange{ExpiresInSeconds: 3600, MaxViews: 1}
	for _, request := range []cliprotocol.Request{
		{Type: cliprotocol.TypeLauncherGetCode, VaultID: "v1"},
		{Type: cliprotocol.TypeLauncherReadPassword, ItemID: "i1"},
		{Type: cliprotocol.TypeLauncherCopyField, VaultID: "v1", ItemID: "i1", Field: "notes"},
		{Type: cliprotocol.TypeLauncherCopyField, VaultID: "v1", ItemID: "i1"},
		{Type: cliprotocol.TypeLauncherOpenItem, VaultID: "v1", IsEditing: true},
		{Type: cliprotocol.TypeLauncherShareItem, ItemID: "i1", ShareLink: shareLink},
		{Type: cliprotocol.TypeLauncherShareItem, VaultID: "v1", ItemID: "i1"},
		{Type: cliprotocol.TypeLauncherShareItem, VaultID: "v1", ItemID: "i1", ShareLink: &cliprotocol.ShareLinkChange{Text: "hello", ExpiresInSeconds: 3600}},
	} {
		if code := getErrorCode(launcher.send(t, request)); code != cliprotocol.ErrorInvalidRequest {
			t.Errorf("%s %+v: %q", request.Type, request, code)
		}
	}
	if fixture.launcherApprover.requestCount.Load() != 0 || len(fixture.handler.types) != 0 {
		t.Error("an invalid launcher request asked or reached the handler")
	}
}

func TestEveryLauncherRequestReachesTheHandler(t *testing.T) {
	fixture := createServerFixture(t)
	launcher, _ := fixture.connectLauncher(t, raycastApp)
	requestTypes := cliprotocol.ListLauncherRequestTypes()[1:]
	for _, requestType := range requestTypes {
		launcher.mustSend(t, cliprotocol.Request{
			Type: requestType, VaultID: "v1", ItemID: "i1", Field: cliprotocol.LauncherFieldOneTimeCode,
			ShareLink: &cliprotocol.ShareLinkChange{ExpiresInSeconds: 604800, MaxViews: 1},
		})
	}
	if !slices.Equal(fixture.handler.types, requestTypes) {
		t.Errorf("handled %q", fixture.handler.types)
	}
}

func shareLauncherItem() cliprotocol.Request {
	return cliprotocol.Request{
		Type: cliprotocol.TypeLauncherShareItem, VaultID: "v-personal", ItemID: "i-github",
		ShareLink: &cliprotocol.ShareLinkChange{Fields: []string{"password", "recovery code"}, ExpiresInSeconds: 604800, MaxViews: 1},
	}
}

func TestLauncherShareAsksEveryTimeAndNamesTheItem(t *testing.T) {
	fixture := createServerFixture(t)
	fixture.server.config.Handler = shareLinkDescribingHandler{fixture.handler}
	launcher, _ := fixture.connectLauncher(t, raycastApp)
	launcher.mustSend(t, listLauncherItems)
	launcher.mustSend(t, shareLauncherItem())
	launcher.mustSend(t, shareLauncherItem())
	launcher.mustSend(t, listLauncherItems)
	if count := fixture.launcherApprover.requestCount.Load(); count != 3 {
		t.Fatalf("%d launcher approvals; want the launcher's and one per share link", count)
	}
	if request := fixture.launcherApprover.requests[0]; request.ShareLink != nil {
		t.Errorf("the launcher approval described a share link: %+v", request.ShareLink)
	}
	request := fixture.launcherApprover.requests[1]
	if request.App != raycastApp || request.ShareLink == nil || request.ShareLink.Title != "GitHub" ||
		!slices.Equal(request.ShareLink.FieldLabels, []string{"password", "Recovery code"}) || request.ShareLink.Detail != "Expires in 7 days · 1 view" {
		t.Errorf("share approval request %+v %+v", request, request.ShareLink)
	}
	if fixture.approver.requestCount.Load() != 0 {
		t.Error("a launcher's share link opened the command-line dialog")
	}
}

func TestLauncherShareDoesNotApproveLauncher(t *testing.T) {
	fixture := createServerFixture(t)
	launcher, _ := fixture.connectLauncher(t, raycastApp)
	launcher.mustSend(t, shareLauncherItem())
	if launcher.isApproved(t) || len(fixture.approvals.ListLaunchers()) != 0 {
		t.Error("approving one share link approved the launcher")
	}
	launcher.mustSend(t, shareLauncherItem())
	if count := fixture.launcherApprover.requestCount.Load(); count != 2 {
		t.Errorf("%d approvals; each share link must ask", count)
	}
	launcher.mustSend(t, listLauncherItems)
	if count := fixture.launcherApprover.requestCount.Load(); count != 3 {
		t.Errorf("%d approvals; listing items must ask after approving only share links", count)
	}
	if !slices.Equal(fixture.handler.types, []string{cliprotocol.TypeLauncherShareItem, cliprotocol.TypeLauncherShareItem, cliprotocol.TypeLauncherListItem}) {
		t.Errorf("handled %q", fixture.handler.types)
	}
}

func TestLauncherShareDoesNotRefreshLauncherApproval(t *testing.T) {
	fixture := createServerFixture(t)
	launcher, _ := fixture.connectLauncher(t, raycastApp)
	launcher.mustSend(t, listLauncherItems)
	approved := fixture.approvals.ListLaunchers()
	fixture.advance(time.Minute)
	launcher.mustSend(t, shareLauncherItem())
	if launchers := fixture.approvals.ListLaunchers(); !slices.Equal(launchers, approved) {
		t.Errorf("approving a share link refreshed the launcher approval: %+v; want %+v", launchers, approved)
	}
}

func TestDeniedLauncherShareCreatesNothing(t *testing.T) {
	fixture := createServerFixture(t)
	launcher, _ := fixture.connectLauncher(t, raycastApp)
	launcher.mustSend(t, listLauncherItems)
	fixture.launcherApprover.setResult(ErrDenied)
	response := launcher.send(t, shareLauncherItem())
	if getErrorCode(response) != cliprotocol.ErrorDenied || response.Error.Message != "Hisar denied the request (it was declined)" {
		t.Fatalf("denied share link: %+v", response.Error)
	}
	if slices.Contains(fixture.handler.types, cliprotocol.TypeLauncherShareItem) {
		t.Errorf("a denied share link reached the handler: %q", fixture.handler.types)
	}
	launcher.mustSend(t, listLauncherItems)
	if count := fixture.launcherApprover.requestCount.Load(); count != 2 {
		t.Errorf("%d launcher approvals; denying a share link must not end the launcher's approval", count)
	}

	unapproved, _ := fixture.connectLauncher(t, towerApp)
	unapproved.send(t, shareLauncherItem())
	if _, isApproved := fixture.connectLauncher(t, towerApp); isApproved {
		t.Error("a denied share link approved the launcher")
	}
}

func TestConcurrentLauncherRequestsShareOnePrompt(t *testing.T) {
	fixture := createServerFixture(t)
	fixture.launcherApprover.release = make(chan struct{})
	first, _ := fixture.connectLauncher(t, raycastApp)
	second, _ := fixture.connectLauncher(t, raycastApp)
	var waitGroup sync.WaitGroup
	responses := make([]cliprotocol.Response, 2)
	for index, client := range []*testClient{first, second} {
		waitGroup.Go(func() { responses[index] = client.send(t, listLauncherItems) })
	}
	for fixture.launcherApprover.requestCount.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	close(fixture.launcherApprover.release)
	waitGroup.Wait()
	for index, response := range responses {
		if !response.OK {
			t.Errorf("request %d: %+v", index, response.Error)
		}
	}
	if count := fixture.launcherApprover.requestCount.Load(); count != 1 {
		t.Errorf("%d prompts for one app", count)
	}
}
