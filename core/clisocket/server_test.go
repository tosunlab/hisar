package clisocket

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
)

type fakePeerResolver struct {
	mutex   sync.Mutex
	peer    Peer
	session TerminalSession
}

func (resolver *fakePeerResolver) ResolvePeer(net.Conn) (Peer, error) {
	resolver.mutex.Lock()
	defer resolver.mutex.Unlock()
	return resolver.peer, nil
}

func (resolver *fakePeerResolver) GetTerminalSession(int) (TerminalSession, error) {
	resolver.mutex.Lock()
	defer resolver.mutex.Unlock()
	return resolver.session, nil
}

func (resolver *fakePeerResolver) GetProcessPath(int) (string, error) {
	return "/Applications/Hisar.app/Contents/Helpers/hisar", nil
}

func (resolver *fakePeerResolver) GetParentPID(int) (int, error) { return 4321, nil }

func (resolver *fakePeerResolver) setSession(session TerminalSession) {
	resolver.mutex.Lock()
	defer resolver.mutex.Unlock()
	resolver.session = session
}

type fakeApprover struct {
	mutex          sync.Mutex
	requests       []ApprovalRequest
	result         error
	release        chan struct{}
	requestCount   atomic.Int32
	cancelledCount atomic.Int32
}

func (approver *fakeApprover) ApproveCommandLine(ctx context.Context, request ApprovalRequest) error {
	approver.requestCount.Add(1)
	approver.mutex.Lock()
	approver.requests = append(approver.requests, request)
	release := approver.release
	result := approver.result
	approver.mutex.Unlock()
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			approver.cancelledCount.Add(1)
			return ErrDenied
		}
	}
	return result
}

type fakeHandler struct {
	mutex    sync.Mutex
	state    string
	types    []string
	resultBy map[string]any
}

func (handler *fakeHandler) GetHello() cliprotocol.HelloResult {
	handler.mutex.Lock()
	defer handler.mutex.Unlock()
	return cliprotocol.HelloResult{State: handler.state, Email: "alice@example.com", Server: "https://hisar.example.com"}
}

func (handler *fakeHandler) HandleRequest(_ context.Context, request *cliprotocol.Request) (any, error) {
	handler.mutex.Lock()
	defer handler.mutex.Unlock()
	handler.types = append(handler.types, request.Type)
	if request.Type == cliprotocol.TypeGetItem && request.ItemID == "missing" {
		return nil, cliprotocol.NewError(cliprotocol.ErrorNotFound, "item not found")
	}
	return handler.resultBy[request.Type], nil
}

func (handler *fakeHandler) setState(state string) {
	handler.mutex.Lock()
	defer handler.mutex.Unlock()
	handler.state = state
}

type serverFixture struct {
	server    *Server
	resolver  *fakePeerResolver
	approver  *fakeApprover
	handler   *fakeHandler
	approvals *SessionApprovals
	now       time.Time
	nowMutex  sync.Mutex
	isEnabled atomic.Bool
}

func createServerFixture(t *testing.T) *serverFixture {
	t.Helper()
	fixture := &serverFixture{
		resolver: &fakePeerResolver{peer: Peer{UID: os.Getuid(), PID: 1234}, session: TerminalSession{SessionID: 77, TerminalDevice: 5, LeaderStartTime: 99, TerminalName: "ttys003"}},
		approver: &fakeApprover{},
		handler:  &fakeHandler{state: cliprotocol.StateUnlocked, resultBy: map[string]any{cliprotocol.TypeListVault: cliprotocol.VaultListResult{Vaults: []cliprotocol.Vault{{ID: "v1", Name: "Personal"}}}}},
		now:      time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC),
	}
	fixture.isEnabled.Store(true)
	fixture.approvals = NewSessionApprovals(0, fixture.getNow)
	fixture.server = NewServer(ServerConfig{
		Handler:      fixture.handler,
		Approver:     fixture.approver,
		PeerResolver: fixture.resolver,
		Approvals:    fixture.approvals,
		IsEnabled:    fixture.isEnabled.Load,
	})
	return fixture
}

func (fixture *serverFixture) getNow() time.Time {
	fixture.nowMutex.Lock()
	defer fixture.nowMutex.Unlock()
	return fixture.now
}

func (fixture *serverFixture) advance(duration time.Duration) {
	fixture.nowMutex.Lock()
	defer fixture.nowMutex.Unlock()
	fixture.now = fixture.now.Add(duration)
}

type testClient struct {
	connection net.Conn
	nextID     int
}

func (fixture *serverFixture) connect(t *testing.T) *testClient {
	t.Helper()
	serverSide, clientSide := net.Pipe()
	go fixture.server.ServeConnection(context.Background(), serverSide)
	t.Cleanup(func() { clientSide.Close() })
	return &testClient{connection: clientSide}
}

func (client *testClient) send(t *testing.T, request cliprotocol.Request) cliprotocol.Response {
	t.Helper()
	client.nextID++
	request.ID = string(rune('a' + client.nextID))
	client.connection.SetDeadline(time.Now().Add(5 * time.Second))
	if err := cliprotocol.WriteJSON(client.connection, request); err != nil {
		t.Fatalf("write: %v", err)
	}
	var response cliprotocol.Response
	if err := cliprotocol.ReadJSON(client.connection, &response); err != nil {
		t.Fatalf("read: %v", err)
	}
	if response.ID != request.ID {
		t.Fatalf("response ID %q for request %q", response.ID, request.ID)
	}
	return response
}

func getErrorCode(response cliprotocol.Response) string {
	if response.Error == nil {
		return ""
	}
	return response.Error.Code
}

func TestServerRefusesOtherUser(t *testing.T) {
	fixture := createServerFixture(t)
	fixture.resolver.peer.UID = os.Getuid() + 1
	client := fixture.connect(t)
	client.connection.SetDeadline(time.Now().Add(5 * time.Second))
	cliprotocol.WriteJSON(client.connection, cliprotocol.Request{ID: "1", Type: cliprotocol.TypeHello})
	var response cliprotocol.Response
	if err := cliprotocol.ReadJSON(client.connection, &response); err == nil {
		t.Fatalf("another user got an answer: %+v", response)
	}
	if fixture.approver.requestCount.Load() != 0 {
		t.Error("another user caused an approval prompt")
	}
}

func TestHelloNeedsNoApproval(t *testing.T) {
	fixture := createServerFixture(t)
	client := fixture.connect(t)
	response := client.send(t, cliprotocol.Request{Type: cliprotocol.TypeHello})
	var hello cliprotocol.HelloResult
	json.Unmarshal(response.Result, &hello)
	if !response.OK || hello.ProtocolVersion != cliprotocol.ProtocolVersion || hello.State != cliprotocol.StateUnlocked || hello.Email != "alice@example.com" || hello.IsApproved {
		t.Fatalf("hello: %+v %+v", response, hello)
	}
	if fixture.approver.requestCount.Load() != 0 {
		t.Error("hello asked for approval")
	}
	client.send(t, cliprotocol.Request{Type: cliprotocol.TypeListVault})
	json.Unmarshal(client.send(t, cliprotocol.Request{Type: cliprotocol.TypeHello}).Result, &hello)
	if !hello.IsApproved {
		t.Error("hello does not report the approval")
	}
}

func TestApprovalIsReusedWithinTheSession(t *testing.T) {
	fixture := createServerFixture(t)
	client := fixture.connect(t)
	response := client.send(t, cliprotocol.Request{Type: cliprotocol.TypeListVault})
	var vaultList cliprotocol.VaultListResult
	json.Unmarshal(response.Result, &vaultList)
	if !response.OK || len(vaultList.Vaults) != 1 || vaultList.Vaults[0].Name != "Personal" {
		t.Fatalf("vaults.list: %+v", response)
	}
	secondClient := fixture.connect(t)
	if response := secondClient.send(t, cliprotocol.Request{Type: cliprotocol.TypeListVault}); !response.OK {
		t.Fatalf("second command in the same session: %+v", response.Error)
	}
	if count := fixture.approver.requestCount.Load(); count != 1 {
		t.Errorf("approval asked %d times for one session", count)
	}
	request := fixture.approver.requests[0]
	if request.ClientPID != 1234 || request.ParentPID != 4321 || request.TerminalName != "ttys003" || request.ClientPath != "/Applications/Hisar.app/Contents/Helpers/hisar" {
		t.Errorf("approval request: %+v", request)
	}
}

func TestOtherSessionNeedsItsOwnApproval(t *testing.T) {
	fixture := createServerFixture(t)
	fixture.connect(t).send(t, cliprotocol.Request{Type: cliprotocol.TypeListVault})
	fixture.resolver.setSession(TerminalSession{SessionID: 78, TerminalDevice: 6, LeaderStartTime: 100})
	fixture.connect(t).send(t, cliprotocol.Request{Type: cliprotocol.TypeListVault})
	fixture.resolver.setSession(TerminalSession{SessionID: 77, TerminalDevice: 5, LeaderStartTime: 101})
	fixture.connect(t).send(t, cliprotocol.Request{Type: cliprotocol.TypeListVault})
	if count := fixture.approver.requestCount.Load(); count != 3 {
		t.Errorf("approval asked %d times for three sessions (one with a reused session ID)", count)
	}
}

func TestApprovalExpiresTenMinutesAfterTheLastRequest(t *testing.T) {
	fixture := createServerFixture(t)
	client := fixture.connect(t)
	client.send(t, cliprotocol.Request{Type: cliprotocol.TypeListVault})
	fixture.advance(9 * time.Minute)
	client.send(t, cliprotocol.Request{Type: cliprotocol.TypeListVault})
	fixture.advance(9 * time.Minute)
	client.send(t, cliprotocol.Request{Type: cliprotocol.TypeListVault})
	if count := fixture.approver.requestCount.Load(); count != 1 {
		t.Fatalf("approval asked %d times while requests kept coming", count)
	}
	fixture.advance(10 * time.Minute)
	client.send(t, cliprotocol.Request{Type: cliprotocol.TypeListVault})
	if count := fixture.approver.requestCount.Load(); count != 2 {
		t.Errorf("approval asked %d times after 10 idle minutes", count)
	}
}

func TestClearAllApprovalEndsApprovals(t *testing.T) {
	fixture := createServerFixture(t)
	client := fixture.connect(t)
	client.send(t, cliprotocol.Request{Type: cliprotocol.TypeListVault})
	fixture.server.ClearApprovals()
	client.send(t, cliprotocol.Request{Type: cliprotocol.TypeListVault})
	if count := fixture.approver.requestCount.Load(); count != 2 {
		t.Errorf("approval asked %d times across a lock", count)
	}
}

func TestApprovalDuringLockIsNotRemembered(t *testing.T) {
	fixture := createServerFixture(t)
	fixture.approver.release = make(chan struct{})
	client := fixture.connect(t)
	done := make(chan cliprotocol.Response)
	go func() { done <- client.send(t, cliprotocol.Request{Type: cliprotocol.TypeListVault}) }()
	for fixture.approver.requestCount.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	fixture.server.ClearApprovals()
	close(fixture.approver.release)
	<-done
	if fixture.approvals.IsApproved(fixture.resolver.session) {
		t.Error("an approval that finished after a lock was remembered")
	}
}

func TestDeniedApprovalReturnsDenied(t *testing.T) {
	fixture := createServerFixture(t)
	fixture.approver.result = ErrDenied
	response := fixture.connect(t).send(t, cliprotocol.Request{Type: cliprotocol.TypeListVault})
	if getErrorCode(response) != cliprotocol.ErrorDenied || response.Error.Message != ErrDenied.Error() {
		t.Fatalf("denied: %+v", response)
	}
	if len(fixture.handler.types) != 0 {
		t.Errorf("handler ran after a denial: %v", fixture.handler.types)
	}
	fixture.approver.result = nil
	if response := fixture.connect(t).send(t, cliprotocol.Request{Type: cliprotocol.TypeListVault}); !response.OK {
		t.Errorf("a denial was remembered: %+v", response.Error)
	}
}

func TestConcurrentRequestsShareOnePrompt(t *testing.T) {
	fixture := createServerFixture(t)
	fixture.approver.release = make(chan struct{})
	var waitGroup sync.WaitGroup
	responses := make([]cliprotocol.Response, 3)
	for index := range responses {
		client := fixture.connect(t)
		waitGroup.Go(func() { responses[index] = client.send(t, cliprotocol.Request{Type: cliprotocol.TypeListVault}) })
	}
	for fixture.approver.requestCount.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	close(fixture.approver.release)
	waitGroup.Wait()
	for index, response := range responses {
		if !response.OK {
			t.Errorf("request %d: %+v", index, response.Error)
		}
	}
	if count := fixture.approver.requestCount.Load(); count != 1 {
		t.Errorf("%d prompts for one session", count)
	}
}

func TestSwitchOffRefusesRequests(t *testing.T) {
	fixture := createServerFixture(t)
	fixture.isEnabled.Store(false)
	client := fixture.connect(t)
	for _, requestType := range []string{cliprotocol.TypeHello, cliprotocol.TypeListVault} {
		if response := client.send(t, cliprotocol.Request{Type: requestType}); getErrorCode(response) != cliprotocol.ErrorIntegrationDisabled {
			t.Errorf("%s with the switch off: %+v", requestType, response)
		}
	}
	if fixture.approver.requestCount.Load() != 0 || len(fixture.handler.types) != 0 {
		t.Error("a request went through with the switch off")
	}
}

func TestUpdateRequiredRefusesEverythingButHello(t *testing.T) {
	fixture := createServerFixture(t)
	var checkedVersions []string
	fixture.server.config.CheckUpdateRequired = func(request *cliprotocol.Request) error {
		checkedVersions = append(checkedVersions, request.ClientVersion)
		return cliprotocol.NewError(cliprotocol.ErrorUpdateRequired, "Hisar needs an update")
	}
	client := fixture.connect(t)
	if response := client.send(t, cliprotocol.Request{Type: cliprotocol.TypeHello, ClientVersion: "0.1.0"}); response.Error != nil {
		t.Fatalf("hello: %+v", response)
	}
	for _, request := range []cliprotocol.Request{{Type: cliprotocol.TypeListVault, ClientVersion: "0.1.0"}, {Type: cliprotocol.TypeResolveSecret, References: []string{"hisar://a/b/c"}}} {
		if response := client.send(t, request); getErrorCode(response) != cliprotocol.ErrorUpdateRequired || response.Error.Message != "Hisar needs an update" {
			t.Errorf("%s: %+v", request.Type, response)
		}
	}
	agentClient := fixture.connect(t)
	if response := agentClient.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentHello, AgentName: "claude-code"}); getErrorCode(response) != cliprotocol.ErrorUpdateRequired {
		t.Errorf("agent.hello: %+v", response)
	}
	if fixture.approver.requestCount.Load() != 0 || len(fixture.handler.types) != 0 {
		t.Error("a request went through while an update is required")
	}
	if len(checkedVersions) != 3 || checkedVersions[0] != "0.1.0" {
		t.Errorf("checked %q", checkedVersions)
	}
}

func TestSignedOutFailsWithoutPrompt(t *testing.T) {
	fixture := createServerFixture(t)
	fixture.handler.setState(cliprotocol.StateSignedOut)
	response := fixture.connect(t).send(t, cliprotocol.Request{Type: cliprotocol.TypeListVault})
	if getErrorCode(response) != cliprotocol.ErrorSignedOut {
		t.Fatalf("signed out: %+v", response)
	}
	if fixture.approver.requestCount.Load() != 0 {
		t.Error("signed out asked for approval")
	}
}

func TestLockedAppStillAsksSoTheWindowCanUnlock(t *testing.T) {
	fixture := createServerFixture(t)
	fixture.handler.setState(cliprotocol.StateLocked)
	fixture.connect(t).send(t, cliprotocol.Request{Type: cliprotocol.TypeListVault})
	if fixture.approver.requestCount.Load() != 1 {
		t.Error("a locked app did not open the approval window")
	}
}

func TestHandlerErrorsKeepTheirCode(t *testing.T) {
	fixture := createServerFixture(t)
	client := fixture.connect(t)
	if response := client.send(t, cliprotocol.Request{Type: cliprotocol.TypeGetItem, ItemID: "missing"}); getErrorCode(response) != cliprotocol.ErrorNotFound {
		t.Errorf("not found: %+v", response)
	}
	client.connection.SetDeadline(time.Now().Add(5 * time.Second))
	cliprotocol.WriteMessage(client.connection, []byte(`{"type":"vaults.list"}`))
	var response cliprotocol.Response
	if err := cliprotocol.ReadJSON(client.connection, &response); err != nil || getErrorCode(response) != cliprotocol.ErrorInvalidRequest {
		t.Errorf("request without ID: %+v %v", response, err)
	}
}

func TestSystemPeerResolverReadsOwnCredentials(t *testing.T) {
	socketPath := filepath.Join(shortTempDirectory(t), "peer.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		connection, _ := listener.Accept()
		accepted <- connection
	}()
	clientConnection, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer clientConnection.Close()
	serverConnection := <-accepted
	defer serverConnection.Close()

	resolver := SystemPeerResolver{}
	peer, err := resolver.ResolvePeer(serverConnection)
	if err != nil || peer.UID != os.Getuid() || peer.PID != os.Getpid() {
		t.Fatalf("peer %+v %v; want uid %d pid %d", peer, err, os.Getuid(), os.Getpid())
	}
	session, err := resolver.GetTerminalSession(peer.PID)
	if err != nil || session.SessionID <= 0 {
		t.Fatalf("session %+v %v", session, err)
	}
	if parentPID, err := resolver.GetParentPID(peer.PID); err != nil || parentPID != os.Getppid() {
		t.Errorf("parent %d %v", parentPID, err)
	}
	if path, err := resolver.GetProcessPath(peer.PID); err != nil || path == "" {
		t.Errorf("path %q %v", path, err)
	}
}

func shortTempDirectory(t *testing.T) string {
	t.Helper()
	directory, err := os.MkdirTemp("/tmp", "hisar-cli-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(directory) })
	return directory
}

func TestRunnerServesPrivateSocketAndStops(t *testing.T) {
	fixture := createServerFixture(t)
	socketPath := filepath.Join(shortTempDirectory(t), "hisar", "cli.sock")
	runner := NewRunner(fixture.server, socketPath)
	runner.Start()
	var connection net.Conn
	var err error
	for range 200 {
		if connection, err = net.Dial("unix", socketPath); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer connection.Close()
	socketInfo, _ := os.Stat(socketPath)
	directoryInfo, _ := os.Stat(filepath.Dir(socketPath))
	if socketInfo.Mode().Perm() != 0o600 || directoryInfo.Mode().Perm() != 0o700 {
		t.Errorf("modes: socket %o, directory %o", socketInfo.Mode().Perm(), directoryInfo.Mode().Perm())
	}
	client := &testClient{connection: connection}
	client.send(t, cliprotocol.Request{Type: cliprotocol.TypeListVault})
	if status := runner.GetStatus(); !status.IsRunning || status.SocketPath != socketPath {
		t.Errorf("status %+v", status)
	}

	runner.Stop()
	if _, err := os.Lstat(socketPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("socket left behind: %v", err)
	}
	if fixture.approvals.IsApproved(fixture.resolver.session) {
		t.Error("stopping kept the approvals")
	}
	if status := runner.GetStatus(); status.IsRunning {
		t.Errorf("status after stop %+v", status)
	}
	connection.SetDeadline(time.Now().Add(time.Second))
	if _, err := cliprotocol.ReadMessage(connection); err == nil {
		t.Error("an open connection survived Stop")
	}
}

func TestRunnerReportsServeError(t *testing.T) {
	fixture := createServerFixture(t)
	socketPath := filepath.Join(shortTempDirectory(t), "cli.sock")
	os.WriteFile(socketPath, []byte("not a socket"), 0o600)
	runner := NewRunner(fixture.server, socketPath)
	runner.Start()
	for range 200 {
		if runner.GetStatus().ErrorMessage != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if status := runner.GetStatus(); status.IsRunning || status.ErrorMessage == "" {
		t.Errorf("status %+v", status)
	}
}

func TestClientDisconnectCancelsTheWaitingApproval(t *testing.T) {
	fixture := createServerFixture(t)
	fixture.approver.release = make(chan struct{})
	client := fixture.connect(t)
	if err := cliprotocol.WriteJSON(client.connection, cliprotocol.Request{ID: "1", Type: cliprotocol.TypeListVault}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for fixture.approver.requestCount.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the server never asked for approval")
		}
		time.Sleep(time.Millisecond)
	}
	client.connection.Close()
	deadline = time.Now().Add(3 * time.Second)
	for fixture.approver.cancelledCount.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the approval kept waiting after the client disconnected")
		}
		time.Sleep(time.Millisecond)
	}
}
