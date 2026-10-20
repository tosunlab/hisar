package app

import (
	"context"
	"errors"
	"net"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/tryhisar/hisar/core/agentharness"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/localstate"
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

type approvingAgentApprover struct {
	mutex    sync.Mutex
	requests []clisocket.AgentApprovalRequest
}

func (approver *approvingAgentApprover) ApproveAgent(_ context.Context, request clisocket.AgentApprovalRequest) (clisocket.AgentApprovalResult, error) {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	approver.requests = append(approver.requests, request)
	return clisocket.AgentApprovalResult{}, nil
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
	approvals := clisocket.NewSessionApprovals(0, nil)
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
	bridge := NewCommandLineBridge(testService.service, clisocket.NewSessionApprovals(0, nil))
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
