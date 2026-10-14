package agent

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/approval"
	"github.com/tryhisar/hisar/core/clisocket"
)

type agentApproveResult struct {
	result clisocket.AgentApprovalResult
	err    error
}

func startAgentApproval(approver *Approver, request clisocket.AgentApprovalRequest) chan agentApproveResult {
	resultChannel := make(chan agentApproveResult, 1)
	go func() {
		result, err := approver.ApproveAgent(context.Background(), request)
		resultChannel <- agentApproveResult{result: result, err: err}
	}()
	return resultChannel
}

func waitForAgentResult(t *testing.T, resultChannel chan agentApproveResult) agentApproveResult {
	t.Helper()
	select {
	case result := <-resultChannel:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("approval did not finish")
		return agentApproveResult{}
	}
}

func createAgentRequest(kind string) clisocket.AgentApprovalRequest {
	return clisocket.AgentApprovalRequest{
		ClientPID: 77, ParentPID: 42, ClientPath: "/opt/homebrew/bin/hisar", TerminalName: "ttys003",
		Agent: clisocket.AgentIdentity{Name: "claude-code", Version: "2.1.0"}, Kind: kind, IsConnectionApproval: true,
	}
}

func TestDescribeAgentName(t *testing.T) {
	allCase := []struct {
		identity clisocket.AgentIdentity
		expected string
	}{
		{clisocket.AgentIdentity{Name: "claude-code"}, "Claude Code"},
		{clisocket.AgentIdentity{Name: "cursor-vscode"}, "Cursor"},
		{clisocket.AgentIdentity{Name: "Codex-MCP-Client"}, "Codex"},
		{clisocket.AgentIdentity{Name: "my-agent", Title: "My Agent"}, "My Agent"},
		{clisocket.AgentIdentity{Name: "evil‮\nagent\x00 "}, "evil agent"},
		{clisocket.AgentIdentity{Name: "   "}, "A coding agent"},
		{clisocket.AgentIdentity{Name: "an agent with a very long name that goes on and on and on and on forever"}, "an agent with a very long name that goes on and on and on a…"},
	}
	for _, testCase := range allCase {
		if name := DescribeAgentName(testCase.identity); name != testCase.expected {
			t.Errorf("%+v = %q; want %q", testCase.identity, name, testCase.expected)
		}
	}
}

func TestAgentConnectionDialogNamesTheAgentAndItsApp(t *testing.T) {
	biometric := &fakeBiometric{isAvailable: true}
	emitter := createFakeEmitter()
	approver := createCommandLineApprover(biometric, emitter, &fakeUnlocker{}, 0)
	resultChannel := startAgentApproval(approver, createAgentRequest(clisocket.AgentRequestConnect))
	event := waitForApprovalEvent(t, emitter)
	if event.Kind != ApprovalKindAgent || event.AgentName != "Claude Code" || event.AppName != "Terminal" || event.ClientName != "hisar mcp" ||
		event.Purpose != "connect" || event.Action != "use Hisar" || event.IsRememberable || !event.IsConnectionApproval || event.TerminalName != "ttys003" ||
		event.ClientPath != "/opt/homebrew/bin/hisar" || event.Warning != "" || event.IsCompact {
		t.Errorf("event = %+v", event)
	}
	approver.Respond(event.RequestID, true, true)
	if result := waitForAgentResult(t, resultChannel); result.err != nil || result.result.IsRemembered {
		t.Errorf("result = %+v", result)
	}
	if allReason := biometric.listAllReason(); !slices.Equal(allReason, []string{`allow "Claude Code" to use Hisar`}) {
		t.Errorf("Touch ID reasons = %q", allReason)
	}
}

func TestAgentRunDialogShowsTheCommandAndSecrets(t *testing.T) {
	emitter := createFakeEmitter()
	approver := createCommandLineApprover(&fakeBiometric{isAvailable: true}, emitter, &fakeUnlocker{}, 0)
	approver.config.HomeDirectory = "/Users/alice"
	request := createAgentRequest(clisocket.AgentRequestRun)
	request.CommandLine = []string{"/usr/local/bin/npm", "run", "deploy", "--", "--message", "it's live"}
	request.WorkingDirectory = "/Users/alice/src/app"
	request.AllSecretName = []string{"Work › Stripe › api key", "Work › AWS › access key", "Work › AWS › secret key", "Work › GitHub › token", "Personal › npm › token"}
	resultChannel := startAgentApproval(approver, request)
	event := waitForApprovalEvent(t, emitter)
	if event.Action != "run a command with 5 secrets" || event.CommandLine != `npm run deploy -- --message 'it'\''s live'` || event.WorkingDirectory != "~/src/app" ||
		!slices.Equal(event.AllSecretName, request.AllSecretName[:4]) || event.MoreSecretCount != 1 || !event.IsRememberable || event.IsRememberedByDefault ||
		event.RememberLabel != "Allow these secrets for this agent for 1 hour" {
		t.Errorf("event = %+v", event)
	}
	approver.Respond(event.RequestID, true, true)
	if result := waitForAgentResult(t, resultChannel); result.err != nil || !result.result.IsRemembered {
		t.Errorf("remembered approval = %+v", result)
	}
}

func TestAgentRevealDialogWarnsAndIsNotRememberedByDefault(t *testing.T) {
	emitter := createFakeEmitter()
	approver := createCommandLineApprover(&fakeBiometric{isAvailable: false}, emitter, &fakeUnlocker{}, 0)
	request := createAgentRequest(clisocket.AgentRequestReveal)
	request.AllSecretName, request.RevealedName = []string{"Work › Stripe › api key"}, "Stripe › api key"
	resultChannel := startAgentApproval(approver, request)
	event := waitForApprovalEvent(t, emitter)
	if event.Action != "read “Stripe › api key”" || event.Warning != "The value will be visible to Claude Code and can end up in its transcript and logs." ||
		event.RememberLabel != "Allow this item for 1 hour" || event.IsRememberedByDefault {
		t.Errorf("event = %+v", event)
	}
	approver.Respond(event.RequestID, true, false)
	if result := waitForAgentResult(t, resultChannel); result.err != nil || result.result.IsRemembered {
		t.Errorf("approval = %+v", result)
	}

	resultChannel = startAgentApproval(approver, request)
	event = waitForApprovalEvent(t, emitter)
	approver.Respond(event.RequestID, false, false)
	if result := waitForAgentResult(t, resultChannel); !errors.Is(result.err, clisocket.ErrDenied) {
		t.Errorf("deny = %+v", result)
	}

	timeoutApprover := createCommandLineApprover(&fakeBiometric{isAvailable: true}, createFakeEmitter(), &fakeUnlocker{}, 20*time.Millisecond)
	_, err := timeoutApprover.ApproveAgent(context.Background(), request)
	var timeoutError clisocket.TimeoutError
	if !errors.As(err, &timeoutError) || !errors.Is(err, clisocket.ErrDenied) {
		t.Errorf("timeout = %v", err)
	}
}

func TestAgentDialogUnlocksALockedHisar(t *testing.T) {
	emitter := createFakeEmitter()
	unlocker := &fakeUnlocker{isLocked: true, correctPassword: "right"}
	approver := createCommandLineApprover(&fakeBiometric{isAvailable: true}, emitter, unlocker, 0)
	request := createAgentRequest(clisocket.AgentRequestCode)
	request.AllSecretName, request.RevealedName = []string{"Personal › GitHub › one-time code"}, "GitHub"
	resultChannel := startAgentApproval(approver, request)
	event := waitForApprovalEvent(t, emitter)
	if !event.IsUnlockRequired || !event.IsPasswordUnlock || event.Action != "get the one-time code of “GitHub”" {
		t.Errorf("event = %+v", event)
	}
	approver.RespondWithPassword(event.RequestID, "right", false)
	if result := waitForAgentResult(t, resultChannel); result.err != nil || unlocker.IsLocked() {
		t.Errorf("unlock = %+v, locked %v", result, unlocker.IsLocked())
	}
}

func TestAgentDialogGoesToThePhoneWithTheAgentName(t *testing.T) {
	emitter := createFakeEmitter()
	phone := createFakePhone()
	approver := createPhoneApprover(emitter, phone, &fakeBiometric{isAvailable: true}, nil, nil)
	request := createAgentRequest(clisocket.AgentRequestRun)
	request.CommandLine, request.WorkingDirectory = []string{"npm", "test"}, testHomeDirectory+"/src/app"
	request.AllSecretName = []string{"Work › Stripe › api key"}
	resultChannel := startAgentApproval(approver, request)
	event := waitForApprovalEvent(t, emitter)
	if !event.IsPhoneApproval {
		t.Errorf("not offered to the phone: %+v", event)
	}
	description := phone.waitForStart(t)
	want := approval.Description{
		Kind: approval.KindCommandLine, AppName: "Claude Code (via hisar mcp) in Terminal", Purpose: "run a command with 1 secret",
		CommandLine: "npm test", RepositoryPath: "~/src/app", TerminalName: "ttys003", Secrets: []string{"Work › Stripe › api key"},
		RememberLabel: "Also allows this secret for this agent for 1 hour",
	}
	if !reflect.DeepEqual(description, want) {
		t.Errorf("phone request = %+v", description)
	}
	phone.answer(true)
	if result := waitForAgentResult(t, resultChannel); result.err != nil || result.result.IsRemembered {
		t.Errorf("phone approval = %+v", result)
	}
}

func TestAgentDialogLeavesOutTheAppWhenItIsTheAgent(t *testing.T) {
	emitter := createFakeEmitter()
	approver := createCommandLineApprover(&fakeBiometric{isAvailable: true}, emitter, &fakeUnlocker{}, 0)
	request := createAgentRequest(clisocket.AgentRequestConnect)
	request.Agent, request.ClientPID = clisocket.AgentIdentity{Name: "hisar"}, 7
	resultChannel := startAgentApproval(approver, request)
	event := waitForApprovalEvent(t, emitter)
	if event.AgentName != "hisar" || event.AppName != "" || describeAgentRequester(event) != "hisar (via hisar mcp)" {
		t.Errorf("event = %+v", event)
	}
	approver.DenyAll()
	waitForAgentResult(t, resultChannel)
}
