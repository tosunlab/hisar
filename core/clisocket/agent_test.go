package clisocket

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
)

type fakeAgentHandler struct {
	fakeHandler
	secretsByReference map[string]AgentSecret
}

func (handler *fakeAgentHandler) LocateAgentSecrets(request *cliprotocol.Request) ([]AgentSecret, error) {
	handler.mutex.Lock()
	defer handler.mutex.Unlock()
	if handler.state == cliprotocol.StateLocked {
		return nil, cliprotocol.NewError(cliprotocol.ErrorLocked, "locked")
	}
	var secrets []AgentSecret
	for _, reference := range request.References {
		secret, isFound := handler.secretsByReference[reference]
		if !isFound {
			return nil, cliprotocol.NewError(cliprotocol.ErrorNotFound, reference+": not found")
		}
		secrets = append(secrets, secret)
	}
	return secrets, nil
}

type fakeAgentApprover struct {
	mutex        sync.Mutex
	requests     []AgentApprovalRequest
	result       error
	isRemembered bool
	onApprove    func()
	// isWaitingForCancel holds every approval until its request is cancelled.
	isWaitingForCancel bool
	cancelledCount     atomic.Int32
}

func (approver *fakeAgentApprover) ApproveAgent(ctx context.Context, request AgentApprovalRequest) (AgentApprovalResult, error) {
	approver.mutex.Lock()
	approver.requests = append(approver.requests, request)
	if approver.onApprove != nil {
		approver.onApprove()
	}
	isWaitingForCancel := approver.isWaitingForCancel
	result := AgentApprovalResult{IsRemembered: approver.isRemembered}
	resultError := approver.result
	approver.mutex.Unlock()
	if isWaitingForCancel {
		<-ctx.Done()
		approver.cancelledCount.Add(1)
		return AgentApprovalResult{}, ErrDenied
	}
	return result, resultError
}

func (approver *fakeAgentApprover) getRequestCount() int {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	return len(approver.requests)
}

func (approver *fakeAgentApprover) getLastRequest() AgentApprovalRequest {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	return approver.requests[len(approver.requests)-1]
}

func (approver *fakeAgentApprover) set(result error, isRemembered bool) {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	approver.result, approver.isRemembered = result, isRemembered
}

const (
	stripeReference = "hisar://Work/Stripe/api%20key"
	githubReference = "hisar://Work/GitHub/password"
)

type agentFixture struct {
	*serverFixture
	agentHandler  *fakeAgentHandler
	agentApprover *fakeAgentApprover
}

func createAgentFixture(t *testing.T) *agentFixture {
	t.Helper()
	fixture := &agentFixture{serverFixture: createServerFixture(t), agentApprover: &fakeAgentApprover{}}
	fixture.agentHandler = &fakeAgentHandler{
		fakeHandler: fakeHandler{state: cliprotocol.StateUnlocked, resultBy: map[string]any{
			cliprotocol.TypeAgentListVault: cliprotocol.AgentVaultListResult{Vaults: []cliprotocol.AgentVault{{ID: "v2", Name: "Work", ItemCount: 2}}},
		}},
		secretsByReference: map[string]AgentSecret{
			stripeReference: {VaultID: "v2", ItemID: "stripe", VaultName: "Work", ItemTitle: "Stripe", FieldLabel: "api key"},
			githubReference: {VaultID: "v2", ItemID: "github", VaultName: "Work", ItemTitle: "GitHub", FieldLabel: "password"},
		},
	}
	fixture.server = NewServer(ServerConfig{
		Handler: fixture.agentHandler, Approver: fixture.approver, AgentApprover: fixture.agentApprover,
		PeerResolver: fixture.resolver, Approvals: fixture.approvals, IsEnabled: fixture.isEnabled.Load,
	})
	return fixture
}

func (fixture *agentFixture) connectAgent(t *testing.T) *testClient {
	t.Helper()
	client := fixture.connect(t)
	if response := client.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentHello, AgentName: "claude-code", AgentVersion: "2.1.0"}); response.Error != nil {
		t.Fatalf("agent.hello: %+v", response.Error)
	}
	return client
}

func TestAgentConnectionIsApprovedOnceAndNamesTheAgent(t *testing.T) {
	fixture := createAgentFixture(t)
	client := fixture.connectAgent(t)
	if fixture.agentApprover.getRequestCount() != 0 {
		t.Fatal("agent.hello asked for an approval")
	}
	for range 3 {
		if response := client.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentListVault}); response.Error != nil {
			t.Fatalf("list: %+v", response.Error)
		}
		client.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentSearchItem, Query: "stripe"})
	}
	if count := fixture.agentApprover.getRequestCount(); count != 1 {
		t.Fatalf("approvals = %d; want one for the connection", count)
	}
	request := fixture.agentApprover.getLastRequest()
	if request.Kind != AgentRequestConnect || request.Agent.Name != "claude-code" || request.Agent.Version != "2.1.0" || !request.IsConnectionApproval ||
		request.ClientPath != "/Applications/Hisar.app/Contents/Helpers/hisar" || request.ParentPID != 4321 || request.TerminalName != "ttys003" || len(request.SecretNames) != 0 {
		t.Errorf("connection approval: %+v", request)
	}
	if fixture.approver.requestCount.Load() != 0 {
		t.Error("an agent connection opened the terminal session dialog")
	}

	other := fixture.connectAgent(t)
	other.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentListVault})
	if count := fixture.agentApprover.getRequestCount(); count != 2 {
		t.Errorf("a second agent connection shared the first one's approval (%d approvals)", count)
	}

	fixture.advance(DefaultApprovalLifetime)
	client.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentListVault})
	if count := fixture.agentApprover.getRequestCount(); count != 3 {
		t.Errorf("the approval outlived 10 idle minutes (%d approvals)", count)
	}
}

func TestAgentAndTerminalApprovalsNeverCoverEachOther(t *testing.T) {
	fixture := createAgentFixture(t)
	terminal := fixture.connect(t)
	if response := terminal.send(t, cliprotocol.Request{Type: cliprotocol.TypeListVault}); response.Error != nil {
		t.Fatal(response.Error)
	}
	agent := fixture.connectAgent(t)
	agent.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentListVault})
	if fixture.agentApprover.getRequestCount() != 1 {
		t.Error("the approved terminal session covered the agent")
	}
	if code := getErrorCode(agent.send(t, cliprotocol.Request{Type: cliprotocol.TypeGetItem, VaultID: "v1", ItemID: "i1"})); code != cliprotocol.ErrorInvalidRequest {
		t.Errorf("an agent connection sent items.get: %q", code)
	}
	if code := getErrorCode(terminal.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentRevealSecret, References: []string{stripeReference}})); code != cliprotocol.ErrorInvalidRequest {
		t.Errorf("a terminal connection sent agent.secrets.reveal: %q", code)
	}
	if code := getErrorCode(agent.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentHello, AgentName: "cursor"})); code != cliprotocol.ErrorInvalidRequest {
		t.Errorf("a second agent.hello: %q", code)
	}
}

func TestAgentRunAsksPerCallWithTheCommandAndSecrets(t *testing.T) {
	fixture := createAgentFixture(t)
	client := fixture.connectAgent(t)
	run := cliprotocol.Request{Type: cliprotocol.TypeAgentResolveSecret, References: []string{stripeReference, githubReference}, CommandLine: []string{"npm", "test"}, WorkingDirectory: "/Users/alice/src/app"}
	for range 2 {
		if response := client.send(t, run); response.Error != nil {
			t.Fatal(response.Error)
		}
	}
	if count := fixture.agentApprover.getRequestCount(); count != 2 {
		t.Fatalf("approvals = %d; want one per call", count)
	}
	first, second := fixture.agentApprover.requests[0], fixture.agentApprover.requests[1]
	if first.Kind != AgentRequestRun || !slices.Equal(first.CommandLine, []string{"npm", "test"}) || first.WorkingDirectory != "/Users/alice/src/app" ||
		!slices.Equal(first.SecretNames, []string{"Work › Stripe › api key", "Work › GitHub › password"}) || !first.IsConnectionApproval {
		t.Errorf("first run: %+v", first)
	}
	if second.IsConnectionApproval {
		t.Error("the second run asked to approve the connection again")
	}
	client.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentListVault})
	if count := fixture.agentApprover.getRequestCount(); count != 2 {
		t.Errorf("the run's approval did not approve the connection (%d approvals)", count)
	}
	handler := fixture.agentHandler
	if !slices.Equal(handler.types, []string{cliprotocol.TypeAgentResolveSecret, cliprotocol.TypeAgentResolveSecret, cliprotocol.TypeAgentListVault}) {
		t.Errorf("handled %q", handler.types)
	}
}

func TestAgentRememberedSecretsLastAnHourForThoseSecretsOnly(t *testing.T) {
	fixture := createAgentFixture(t)
	client := fixture.connectAgent(t)
	fixture.agentApprover.set(nil, true)
	client.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentResolveSecret, References: []string{stripeReference}, CommandLine: []string{"make", "deploy"}})
	fixture.agentApprover.set(nil, false)
	client.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentResolveSecret, References: []string{stripeReference}, CommandLine: []string{"make", "test"}})
	if count := fixture.agentApprover.getRequestCount(); count != 1 {
		t.Fatalf("a remembered secret asked again (%d approvals)", count)
	}
	client.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentResolveSecret, References: []string{stripeReference, githubReference}, CommandLine: []string{"make"}})
	if count := fixture.agentApprover.getRequestCount(); count != 2 {
		t.Errorf("a secret that was not remembered did not ask (%d approvals)", count)
	}
	client.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentRevealSecret, References: []string{stripeReference}})
	if count := fixture.agentApprover.getRequestCount(); count != 3 {
		t.Errorf("remembering a secret for commands let the agent read it (%d approvals)", count)
	}
	other := fixture.connectAgent(t)
	other.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentResolveSecret, References: []string{stripeReference}})
	if count := fixture.agentApprover.getRequestCount(); count != 4 {
		t.Errorf("another agent connection used the remembered secret (%d approvals)", count)
	}
	for range 5 {
		fixture.advance(9 * time.Minute)
		client.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentListVault})
	}
	fixture.advance(16 * time.Minute)
	client.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentResolveSecret, References: []string{stripeReference}})
	if count := fixture.agentApprover.getRequestCount(); count != 5 {
		t.Errorf("the remembered secret outlived an hour (%d approvals)", count)
	}
}

func TestAgentRevealAsksPerItemAndRemembersTheItem(t *testing.T) {
	fixture := createAgentFixture(t)
	client := fixture.connectAgent(t)
	fixture.agentHandler.secretsByReference["hisar://Work/Stripe/username"] = AgentSecret{VaultID: "v2", ItemID: "stripe", VaultName: "Work", ItemTitle: "Stripe", FieldLabel: "username"}
	reveal := cliprotocol.Request{Type: cliprotocol.TypeAgentRevealSecret, References: []string{stripeReference}}

	fixture.agentApprover.set(ErrDenied, false)
	response := client.send(t, reveal)
	if getErrorCode(response) != cliprotocol.ErrorDenied || response.Error.Message != ErrDenied.Error() {
		t.Fatalf("denied: %+v", response.Error)
	}
	if request := fixture.agentApprover.getLastRequest(); request.Kind != AgentRequestReveal || request.RevealedName != "Stripe › api key" ||
		!slices.Equal(request.SecretNames, []string{"Work › Stripe › api key"}) || !request.IsConnectionApproval {
		t.Errorf("reveal request: %+v", request)
	}
	if code := getErrorCode(client.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentListVault})); code != cliprotocol.ErrorDenied {
		t.Errorf("a denied reveal approved the connection: %q", code)
	}

	fixture.agentApprover.set(TimeoutError{Timeout: time.Minute}, false)
	if response := client.send(t, reveal); getErrorCode(response) != cliprotocol.ErrorDenied || !strings.Contains(response.Error.Message, "not answered within a minute") {
		t.Errorf("timeout: %+v", response.Error)
	}

	fixture.agentApprover.set(nil, false)
	client.send(t, reveal)
	client.send(t, reveal)
	if count := fixture.agentApprover.getRequestCount(); count != 5 {
		t.Fatalf("a reveal without remembering did not ask every time (%d approvals)", count)
	}
	fixture.agentApprover.set(nil, true)
	client.send(t, reveal)
	fixture.agentApprover.set(ErrDenied, false)
	for _, reference := range []string{stripeReference, "hisar://Work/Stripe/username"} {
		if response := client.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentRevealSecret, References: []string{reference}}); response.Error != nil {
			t.Errorf("remembered item, %s: %+v", reference, response.Error)
		}
	}
	if code := getErrorCode(client.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentRevealSecret, References: []string{githubReference}})); code != cliprotocol.ErrorDenied {
		t.Errorf("another item was covered: %q", code)
	}
	fixture.advance(AgentGrantLifetime)
	if code := getErrorCode(client.send(t, reveal)); code != cliprotocol.ErrorDenied {
		t.Errorf("the remembered item outlived an hour: %q", code)
	}
}

func TestAgentReferencesAreCheckedBeforeAsking(t *testing.T) {
	fixture := createAgentFixture(t)
	client := fixture.connectAgent(t)
	response := client.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentRevealSecret, References: []string{"hisar://Hidden/Bank/password"}})
	if getErrorCode(response) != cliprotocol.ErrorNotFound || fixture.agentApprover.getRequestCount() != 0 {
		t.Errorf("unknown reference: %+v, %d approvals", response.Error, fixture.agentApprover.getRequestCount())
	}
	for _, request := range []cliprotocol.Request{
		{Type: cliprotocol.TypeAgentResolveSecret},
		{Type: cliprotocol.TypeAgentRevealSecret, References: []string{stripeReference, githubReference}},
		{Type: cliprotocol.TypeAgentGetCode},
		{Type: cliprotocol.TypeAgentGetCode, References: []string{stripeReference}, ItemID: "stripe"},
		{Type: "agent.items.delete"},
	} {
		if code := getErrorCode(client.send(t, request)); code != cliprotocol.ErrorInvalidRequest {
			t.Errorf("%s %q: %q", request.Type, request.References, code)
		}
	}
	references := make([]string, cliprotocol.MaxAgentReferenceCount+1)
	for index := range references {
		references[index] = stripeReference
	}
	if code := getErrorCode(client.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentResolveSecret, References: references})); code != cliprotocol.ErrorInvalidRequest {
		t.Errorf("too many references: %q", code)
	}
	if fixture.agentApprover.getRequestCount() != 0 {
		t.Error("an invalid request asked for an approval")
	}
}

func TestAgentRequestWhileLockedUnlocksInTheDialog(t *testing.T) {
	fixture := createAgentFixture(t)
	client := fixture.connectAgent(t)
	fixture.agentHandler.setState(cliprotocol.StateLocked)
	fixture.agentApprover.set(nil, true)
	fixture.agentApprover.onApprove = func() { fixture.agentHandler.setState(cliprotocol.StateUnlocked) }
	if response := client.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentGetCode, References: []string{githubReference}}); response.Error != nil {
		t.Fatal(response.Error)
	}
	if request := fixture.agentApprover.getLastRequest(); request.Kind != AgentRequestCode || !slices.Equal(request.SecretNames, []string{"Work › GitHub › password"}) || request.RevealedName != "GitHub" {
		t.Errorf("locked request: %+v", request)
	}
	fixture.agentApprover.onApprove = nil
	fixture.agentApprover.set(ErrDenied, false)
	if response := client.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentGetCode, References: []string{githubReference}}); response.Error != nil {
		t.Errorf("the secret remembered while locked asked again: %+v", response.Error)
	}
}

func TestLockEndsAgentApprovalsAndRememberedSecrets(t *testing.T) {
	fixture := createAgentFixture(t)
	client := fixture.connectAgent(t)
	fixture.agentApprover.set(nil, true)
	client.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentRevealSecret, References: []string{stripeReference}})
	fixture.approvals.ClearAll()
	fixture.agentApprover.set(ErrDenied, false)
	if code := getErrorCode(client.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentRevealSecret, References: []string{stripeReference}})); code != cliprotocol.ErrorDenied {
		t.Errorf("the remembered item survived the lock: %q", code)
	}
	if code := getErrorCode(client.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentListVault})); code != cliprotocol.ErrorDenied {
		t.Errorf("the connection approval survived the lock: %q", code)
	}
}

func TestAgentHelloReportsTheAgentApproval(t *testing.T) {
	fixture := createAgentFixture(t)
	client := fixture.connectAgent(t)
	var hello cliprotocol.HelloResult
	readResult(t, client.send(t, cliprotocol.Request{Type: cliprotocol.TypeHello}), &hello)
	if hello.IsApproved {
		t.Error("approved before any approval")
	}
	client.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentListVault})
	readResult(t, client.send(t, cliprotocol.Request{Type: cliprotocol.TypeHello}), &hello)
	if !hello.IsApproved {
		t.Error("not approved after the approval")
	}
	fixture.agentHandler.setState(cliprotocol.StateSignedOut)
	if code := getErrorCode(client.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentListVault})); code != cliprotocol.ErrorSignedOut {
		t.Errorf("signed out: %q", code)
	}
}

func TestDescribeAgentSecretsWhileLocked(t *testing.T) {
	names, revealedName := describeAgentSecrets(&cliprotocol.Request{Type: cliprotocol.TypeAgentRevealSecret, References: []string{stripeReference}}, nil)
	if !slices.Equal(names, []string{"Work › Stripe › api key"}) || revealedName != "Stripe › api key" {
		t.Errorf("%q %q", names, revealedName)
	}
	names, revealedName = describeAgentSecrets(&cliprotocol.Request{Type: cliprotocol.TypeAgentGetCode, ItemID: "item-1"}, nil)
	if !slices.Equal(names, []string{"item item-1"}) || revealedName != "item item-1" {
		t.Errorf("%q %q", names, revealedName)
	}
}

func readResult(t *testing.T, response cliprotocol.Response, result any) {
	t.Helper()
	if response.Error != nil {
		t.Fatalf("%+v", response.Error)
	}
	if err := json.Unmarshal(response.Result, result); err != nil {
		t.Fatal(err)
	}
}

func TestAgentDisconnectCancelsTheWaitingApproval(t *testing.T) {
	fixture := createAgentFixture(t)
	fixture.agentApprover.isWaitingForCancel = true
	client := fixture.connectAgent(t)
	if err := cliprotocol.WriteJSON(client.connection, cliprotocol.Request{ID: "1", Type: cliprotocol.TypeAgentListVault}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for fixture.agentApprover.getRequestCount() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the server never asked for approval")
		}
		time.Sleep(time.Millisecond)
	}
	client.connection.Close()
	deadline = time.Now().Add(3 * time.Second)
	for fixture.agentApprover.cancelledCount.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the agent approval kept waiting after hisar mcp disconnected")
		}
		time.Sleep(time.Millisecond)
	}
}
