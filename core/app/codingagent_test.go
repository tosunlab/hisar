package app

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	sshprotocol "golang.org/x/crypto/ssh/agent"

	"github.com/tryhisar/hisar/core/agentharness"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/localsocket"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/sshagent"
)

func TestVaultAvailableToAgentsIsSavedAndOffByDefault(t *testing.T) {
	testService := createUnlockedTestService(t)
	service := testService.service
	if vaultIDs := service.ListAgentVaultIDs(); len(vaultIDs) != 0 {
		t.Fatalf("available by default: %q", vaultIDs)
	}
	if vaultIDs, err := service.SetVaultAvailableToAgents(testService.personalID, true); err != nil || !slices.Equal(vaultIDs, []string{testService.personalID}) {
		t.Fatalf("turn on: %q %v", vaultIDs, err)
	}
	service.SetVaultAvailableToAgents(testService.personalID, true)
	service.SetVaultAvailableToAgents(testService.familyID, true)
	if vaultIDs, _ := service.SetVaultAvailableToAgents(testService.familyID, false); !slices.Equal(vaultIDs, []string{testService.personalID}) {
		t.Errorf("turn off: %q", vaultIDs)
	}
	state, err := localstate.Load(service.config.StatePath)
	if err != nil || !slices.Equal(state.AgentVaultIDs, []string{testService.personalID}) {
		t.Errorf("saved %q %v", state.AgentVaultIDs, err)
	}
	if _, err := service.SetVaultAvailableToAgents("no-such-vault", true); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown vault: %v", err)
	}
	service.Lock()
	if _, err := service.SetVaultAvailableToAgents(testService.personalID, false); !errors.Is(err, ErrLocked) {
		t.Errorf("locked: %v", err)
	}
	if vaultIDs := service.ListAgentVaultIDs(); !slices.Equal(vaultIDs, []string{testService.personalID}) {
		t.Errorf("after lock: %q", vaultIDs)
	}
}

// insideOmpResolver places every caller inside an omp that is this test process, so the harness stays running.
type insideOmpResolver struct {
	staticPeerResolver
	session agentharness.Session
}

func (resolver insideOmpResolver) GetAgentSession(int) (agentharness.Session, bool, error) {
	return resolver.session, true, nil
}

// approvingAgentApprover approves every agent request, remembering its secrets when isRemembered.
type approvingAgentApprover struct {
	mutex        sync.Mutex
	requests     []clisocket.AgentApprovalRequest
	isRemembered bool
}

func (approver *approvingAgentApprover) ApproveAgent(_ context.Context, request clisocket.AgentApprovalRequest) (clisocket.AgentApprovalResult, error) {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	approver.requests = append(approver.requests, request)
	return clisocket.AgentApprovalResult{IsRemembered: approver.isRemembered}, nil
}

func (approver *approvingAgentApprover) getCount() int {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	return len(approver.requests)
}

func TestCommandLineInsideAnAgentSeesOnlyAgentVaults(t *testing.T) {
	testService := createUnlockedTestService(t)
	service := testService.service
	deployToken, err := service.SaveItem(testService.personalID, ItemDetail{Title: "Deploy token", Password: "deploy-token-value"})
	if err != nil {
		t.Fatal(err)
	}
	bank, err := service.SaveItem(testService.familyID, ItemDetail{Title: "Bank", Password: "bank-password"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetVaultAvailableToAgents(testService.personalID, true); err != nil {
		t.Fatal(err)
	}
	instance, err := agentharness.ReadInstance(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	approvals := clisocket.NewSessionApprovals(0, nil, nil)
	agentApprover := &approvingAgentApprover{}
	commandLineApprover := &countingApprover{}
	server := clisocket.NewServer(clisocket.ServerConfig{
		Handler: NewCommandLineBridge(service, approvals), Approver: commandLineApprover, AgentApprover: agentApprover, Approvals: approvals,
		PeerResolver: insideOmpResolver{session: agentharness.Session{Harness: agentharness.Harness{ID: "omp", Name: "omp"}, PID: os.Getpid(), StartTime: instance.StartTime}},
	})
	serverSide, clientSide := net.Pipe()
	go server.ServeConnection(context.Background(), serverSide)
	t.Cleanup(func() { clientSide.Close() })
	test := &commandLineTest{connection: clientSide}

	var vaultList cliprotocol.VaultListResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeListVault}, &vaultList)
	if len(vaultList.Vaults) != 1 || vaultList.Vaults[0].ID != testService.personalID {
		t.Errorf("vaults %+v", vaultList.Vaults)
	}
	var itemList cliprotocol.ItemListResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeListItem, VaultID: testService.personalID}, &itemList)
	if len(itemList.Items) != 1 || itemList.Items[0].Title != "Deploy token" {
		t.Errorf("items %+v", itemList.Items)
	}
	var itemResult cliprotocol.ItemResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeGetItem, VaultID: testService.personalID, ItemID: deployToken.ID}, &itemResult)
	if itemResult.Item.Password != "deploy-token-value" {
		t.Errorf("item %+v", itemResult.Item)
	}
	var values cliprotocol.ResolveSecretResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeResolveSecret, References: []string{"hisar://Personal/Deploy%20token/password"}, CommandLine: []string{"true"}}, &values)
	if !slices.Equal(values.Values, []string{"deploy-token-value"}) {
		t.Errorf("values %q", values.Values)
	}

	approvalCount := agentApprover.getCount()
	missingVault := test.send(t, cliprotocol.Request{Type: cliprotocol.TypeListItem, VaultID: "no-such-vault"}, nil)
	for _, request := range []cliprotocol.Request{
		{Type: cliprotocol.TypeListItem, VaultID: testService.familyID},
		{Type: cliprotocol.TypeGetItem, VaultID: testService.familyID, ItemID: bank.ID},
	} {
		if failure := test.send(t, request, nil); failure != missingVault {
			t.Errorf("%s in the hidden vault: %q; a vault that does not exist: %q", request.Type, failure, missingVault)
		}
	}
	for _, reference := range []string{"hisar://Family/Bank/password", "hisar://Nowhere/Bank/password"} {
		if failure := test.send(t, cliprotocol.Request{Type: cliprotocol.TypeResolveSecret, References: []string{reference}, CommandLine: []string{"true"}}, nil); !strings.HasPrefix(failure, "not-found: "+reference+": ") {
			t.Errorf("%s: %q", reference, failure)
		}
	}
	if failure := test.send(t, cliprotocol.Request{Type: cliprotocol.TypeSaveItem, VaultID: testService.personalID, Item: &cliprotocol.ItemChange{Title: "New"}}, nil); failure != "denied: omp can use secrets through hisar run. Change items in the Hisar app." {
		t.Errorf("items.save: %q", failure)
	}
	if agentApprover.getCount() != approvalCount || commandLineApprover.getCount() != 0 {
		t.Errorf("a hidden vault or a refused request asked: %d agent approvals, %d command-line approvals", agentApprover.getCount()-approvalCount, commandLineApprover.getCount())
	}
}

func TestAgentHostsAreSavedInDesktopState(t *testing.T) {
	testService := createUnlockedTestService(t)
	bridge := NewCommandLineBridge(testService.service, clisocket.NewSessionApprovals(0, nil, nil))
	tern := clisocket.AppIdentity{Path: "/Applications/Tern.app", TeamID: "VTLHGQC72S"}
	if bridge.IsAgentHost(tern) {
		t.Fatal("an agent host before any agent ran")
	}
	for range 2 {
		if err := bridge.AddAgentHost(tern); err != nil {
			t.Fatal(err)
		}
	}
	if !bridge.IsAgentHost(tern) || bridge.IsAgentHost(clisocket.AppIdentity{Path: tern.Path, TeamID: "OTHERTEAM1"}) {
		t.Error("IsAgentHost does not match the app's identity")
	}
	state, err := localstate.Load(testService.service.config.StatePath)
	if err != nil || !slices.Equal(state.AgentHosts, []localstate.AgentHost{{Path: tern.Path, TeamID: tern.TeamID}}) {
		t.Errorf("saved %+v, %v", state.AgentHosts, err)
	}
}

var ternApp = clisocket.AppIdentity{Path: "/Applications/Tern.app", TeamID: "VTLHGQC72S"}

// ternResolver places every caller in Tern without a terminal, inside omp while isInsideOmp.
type ternResolver struct {
	staticPeerResolver
	mutex       sync.Mutex
	omp         agentharness.Session
	isInsideOmp bool
}

func (*ternResolver) GetTerminalSession(int) (clisocket.TerminalSession, error) {
	return clisocket.TerminalSession{SessionID: 4200}, nil
}

func (resolver *ternResolver) GetAgentSession(int) (agentharness.Session, bool, error) {
	resolver.mutex.Lock()
	defer resolver.mutex.Unlock()
	if !resolver.isInsideOmp {
		return agentharness.Session{}, false, nil
	}
	return resolver.omp, true, nil
}

func (resolver *ternResolver) setInsideOmp(isInsideOmp bool) {
	resolver.mutex.Lock()
	defer resolver.mutex.Unlock()
	resolver.isInsideOmp = isInsideOmp
}

// agentSessionTest is the app's CLI socket for callers in Tern, inside an omp that is this test process, so it keeps
// running. Personal holds "Deploy token" and is available to agents. sessions are the live agent sessions the CLI
// socket shares with an SSH agent.
type agentSessionTest struct {
	testService         unlockedTestService
	server              *clisocket.Server
	resolver            *ternResolver
	agentApprover       *approvingAgentApprover
	commandLineApprover *countingApprover
	sessions            *agentharness.Sessions
}

const deployTokenReference = "hisar://Personal/Deploy%20token/password"

func createAgentSessionTest(t *testing.T, isRemembered bool) *agentSessionTest {
	t.Helper()
	testService := createUnlockedTestService(t)
	if _, err := testService.service.SaveItem(testService.personalID, ItemDetail{Title: "Deploy token", Password: "deploy-token-value"}); err != nil {
		t.Fatal(err)
	}
	if _, err := testService.service.SetVaultAvailableToAgents(testService.personalID, true); err != nil {
		t.Fatal(err)
	}
	instance, err := agentharness.ReadInstance(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	omp := agentharness.Session{
		Harness: agentharness.Harness{ID: "omp", Name: "omp"}, PID: os.Getpid(), StartTime: instance.StartTime,
		Directory: "/Users/alice/src/hisarapp-core", IsSigned: true,
	}
	test := &agentSessionTest{
		testService: testService, resolver: &ternResolver{omp: omp, isInsideOmp: true},
		agentApprover: &approvingAgentApprover{isRemembered: isRemembered}, commandLineApprover: &countingApprover{},
		sessions: agentharness.NewSessions(nil),
	}
	approvals := clisocket.NewSessionApprovals(0, nil, test.sessions)
	bridge := NewCommandLineBridge(testService.service, approvals)
	test.server = clisocket.NewServer(clisocket.ServerConfig{
		Handler: bridge, Approver: test.commandLineApprover, AgentApprover: test.agentApprover, Approvals: approvals, AgentHosts: bridge,
		PeerResolver: test.resolver, IdentifyApp: func(int) (clisocket.AppIdentity, bool) { return ternApp, true },
	})
	return test
}

func (test *agentSessionTest) connect(t *testing.T) *commandLineTest {
	t.Helper()
	serverSide, clientSide := net.Pipe()
	go test.server.ServeConnection(context.Background(), serverSide)
	t.Cleanup(func() { clientSide.Close() })
	return &commandLineTest{connection: clientSide}
}

func (test *agentSessionTest) runDeploy(t *testing.T, connection *commandLineTest) {
	t.Helper()
	var values cliprotocol.ResolveSecretResult
	connection.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeResolveSecret, References: []string{deployTokenReference}, CommandLine: []string{"make", "deploy"}}, &values)
	if !slices.Equal(values.Values, []string{"deploy-token-value"}) {
		t.Fatalf("values %q", values.Values)
	}
}

func TestListAgentSessionsShowsRememberedSecrets(t *testing.T) {
	test := createAgentSessionTest(t, true)
	service := test.testService.service
	if sessions := service.ListAgentSessions(); len(sessions) != 0 {
		t.Fatalf("sessions before any request: %+v", sessions)
	}
	approvedAt := time.Now()
	test.runDeploy(t, test.connect(t))

	sessions := service.ListAgentSessions()
	if len(sessions) != 1 {
		t.Fatalf("sessions %+v", sessions)
	}
	omp := test.resolver.omp
	listed := sessions[0]
	if listed.PID != omp.PID || listed.StartTime != omp.StartTime || listed.HarnessName != "omp" || !listed.IsSigned || listed.HostName != "Tern" ||
		listed.ProjectName != "hisarapp-core" || listed.ProjectPath != "/Users/alice/src/hisarapp-core" {
		t.Errorf("session %+v", listed)
	}
	if firstRequestAt, err := time.Parse(time.RFC3339, listed.FirstRequestAt); err != nil || firstRequestAt.Before(approvedAt.Truncate(time.Second)) {
		t.Errorf("first request at %q, %v", listed.FirstRequestAt, err)
	}
	if len(listed.RememberedSecrets) != 1 || listed.RememberedSecrets[0].Name != "Personal › Deploy token › password" || listed.RememberedSecrets[0].IsReveal {
		t.Fatalf("remembered %+v", listed.RememberedSecrets)
	}
	if expiresAt, err := time.Parse(time.RFC3339, listed.RememberedSecrets[0].ExpiresAt); err != nil || expiresAt.Sub(approvedAt) < 59*time.Minute {
		t.Errorf("expires at %q, %v", listed.RememberedSecrets[0].ExpiresAt, err)
	}

	service.Lock()
	if sessions := service.ListAgentSessions(); len(sessions) != 1 || len(sessions[0].RememberedSecrets) != 0 {
		t.Errorf("after lock: %+v", sessions)
	}
}

func TestRevokeAgentSessionAsksAgain(t *testing.T) {
	test := createAgentSessionTest(t, true)
	service := test.testService.service
	revoked := test.connect(t)
	test.runDeploy(t, revoked)
	test.runDeploy(t, revoked)
	if count := test.agentApprover.getCount(); count != 1 {
		t.Fatalf("the remembered secret asked again: %d approvals", count)
	}

	omp := test.resolver.omp
	if left := service.RevokeAgentSession(omp.PID, omp.StartTime); len(left) != 0 {
		t.Errorf("sessions after revoking: %+v", left)
	}
	revoked.connection.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := revoked.connection.Read(make([]byte, 1)); err == nil {
		t.Error("the revoked session's connection stayed open")
	}
	test.runDeploy(t, test.connect(t))
	if count := test.agentApprover.getCount(); count != 2 {
		t.Errorf("after revoking, %d approvals; want a second one", count)
	}
	if sessions := service.ListAgentSessions(); len(sessions) != 1 || sessions[0].PID != omp.PID {
		t.Errorf("the next request did not start a session: %+v", sessions)
	}
}

// ompSSHResolver resolves every SSH agent connection to ssh inside omp, joining omp's live session as the desktop's
// resolver does.
type ompSSHResolver struct {
	test *agentSessionTest
}

func (resolver ompSSHResolver) ResolvePeer(net.Conn) (sshagent.Peer, error) {
	omp := resolver.test.resolver.omp
	return sshagent.Peer{
		PeerInfo:     localsocket.PeerInfo{PID: omp.PID, Name: "ssh", Path: "/usr/bin/ssh", ApplicationPath: ternApp.Path},
		AgentSession: resolver.test.sessions.Join(omp, ternApp.Path),
	}, nil
}

// startSSHAgent serves an SSH agent that shares the CLI socket's live agent sessions, and returns a function that signs
// with the key over a new connection.
func (test *agentSessionTest) startSSHAgent(t *testing.T, approver sshagent.Approver, resolver sshagent.PeerResolver, key ssh.PublicKey) func() error {
	t.Helper()
	server := sshagent.NewServer(NewSSHKeySource(test.testService.service), approver, resolver)
	directory, err := os.MkdirTemp("/tmp", "hisar-app-ssh-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(directory) })
	listener, err := localsocket.ListenUnixSocket(filepath.Join(directory, "agent.sock"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go server.Serve(ctx, listener)
	return func() error {
		connection, err := net.Dial("unix", listener.Addr().String())
		if err != nil {
			return err
		}
		defer connection.Close()
		_, err = sshprotocol.NewClient(connection).Sign(key, []byte("auth"))
		return err
	}
}

func (test *agentSessionTest) generateDeployKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	generated, err := test.testService.service.GenerateSSHKey(test.testService.personalID, "Deploy key", "")
	if err != nil {
		t.Fatal(err)
	}
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(generated.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// TestRevokeAgentSessionEndsItsSSHApprovals: an omp that only used SSH is listed in Settings, and revoking it there
// ends the SSH approval it remembered, as for the command line.
func TestRevokeAgentSessionEndsItsSSHApprovals(t *testing.T) {
	test := createAgentSessionTest(t, true)
	service := test.testService.service
	approver := &recordingApprover{isAllowed: true, isRemembered: true}
	sign := test.startSSHAgent(t, approver, ompSSHResolver{test}, test.generateDeployKey(t))
	for range 2 {
		if err := sign(); err != nil {
			t.Fatal(err)
		}
	}
	if count := approver.countRequest(); count != 1 {
		t.Fatalf("%d SSH approvals; the remembered one asked again", count)
	}
	omp := test.resolver.omp
	if sessions := service.ListAgentSessions(); len(sessions) != 1 || sessions[0].PID != omp.PID || sessions[0].HostName != "Tern" {
		t.Fatalf("Settings lists %+v for an omp that used SSH", sessions)
	}

	service.RevokeAgentSession(omp.PID, omp.StartTime)
	if err := sign(); err != nil {
		t.Fatal(err)
	}
	if count := approver.countRequest(); count != 2 {
		t.Errorf("SSH reused a grant after Settings revoked its session; %d approvals, want 2", count)
	}
}

// ternSSHResolver resolves every SSH agent connection to ssh without a terminal in Tern, outside any harness: work omp
// may have handed off once Tern is an agent host, as the desktop's resolver decides on every request.
type ternSSHResolver struct {
	server *clisocket.Server
}

func (resolver ternSSHResolver) ResolvePeer(net.Conn) (sshagent.Peer, error) {
	return sshagent.Peer{
		PeerInfo:          localsocket.PeerInfo{PID: 4242, Name: "ssh", Path: "/usr/bin/ssh", ApplicationPath: ternApp.Path},
		IsNeverRemembered: func() bool { return resolver.server.IsAgentHost(ternApp) },
	}, nil
}

// TestOpenSSHConnectionLosesAppApprovalWhenHostRecorded: an SSH connection from Tern stays open while omp's first
// hisar request makes Tern an agent host. Its next signature asks again rather than reusing Tern's app approval.
func TestOpenSSHConnectionLosesAppApprovalWhenHostRecorded(t *testing.T) {
	test := createAgentSessionTest(t, true)
	key := test.generateDeployKey(t)
	approver := &recordingApprover{isAllowed: true, isRemembered: true}
	resolver := ternSSHResolver{test.server}
	peer, _ := resolver.ResolvePeer(nil)
	client := sshagent.NewServer(NewSSHKeySource(test.testService.service), approver, resolver).CreateAgent(context.Background(), peer)
	for range 2 {
		if _, err := client.Sign(key, []byte("before")); err != nil {
			t.Fatal(err)
		}
	}
	if count := approver.countRequest(); count != 1 {
		t.Fatalf("%d approvals before Tern hosted omp; want the remembered one", count)
	}
	test.runDeploy(t, test.connect(t))
	if !test.server.IsAgentHost(ternApp) {
		t.Fatal("Tern was not recorded as an agent host")
	}
	if _, err := client.Sign(key, []byte("after")); err != nil {
		t.Fatal(err)
	}
	if count := approver.countRequest(); count != 2 {
		t.Errorf("the open connection reused Tern's app approval after Tern hosted omp; %d approvals, want 2", count)
	}
}

func TestForgetAgentHostRestoresAppScope(t *testing.T) {
	test := createAgentSessionTest(t, false)
	service := test.testService.service
	test.connect(t).mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeListVault}, nil)
	if hosts := service.ListAgentHosts(); !slices.Equal(hosts, []AgentHost{{Path: ternApp.Path, TeamID: ternApp.TeamID, Name: "Tern"}}) {
		t.Fatalf("hosts %+v", hosts)
	}

	test.resolver.setInsideOmp(false)
	for range 2 {
		test.connect(t).mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeListVault}, nil)
	}
	if count := test.commandLineApprover.getCount(); count != 2 {
		t.Fatalf("an agent host's callers: %d approvals; want one per connection", count)
	}

	if hosts, err := service.ForgetAgentHost(ternApp.Path, ternApp.TeamID); err != nil || len(hosts) != 0 {
		t.Fatalf("forget: %+v, %v", hosts, err)
	}
	for range 2 {
		test.connect(t).mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeListVault}, nil)
	}
	if count := test.commandLineApprover.getCount(); count != 3 {
		t.Errorf("after forgetting Tern: %d approvals; want one for the app", count-2)
	}
	if state, err := localstate.Load(service.config.StatePath); err != nil || len(state.AgentHosts) != 0 {
		t.Errorf("saved %+v, %v", state.AgentHosts, err)
	}

	service.Lock()
	if _, err := service.ForgetAgentHost(ternApp.Path, ternApp.TeamID); !errors.Is(err, ErrLocked) {
		t.Errorf("forget while locked: %v", err)
	}
}
