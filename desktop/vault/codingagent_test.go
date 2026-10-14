package vault

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tryhisar/hisar/cli/mcpserver"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/localstate"
)

type recordingAgentApprover struct {
	mutex        sync.Mutex
	allRequest   []clisocket.AgentApprovalRequest
	result       error
	isRemembered bool
}

func (approver *recordingAgentApprover) ApproveAgent(_ context.Context, request clisocket.AgentApprovalRequest) (clisocket.AgentApprovalResult, error) {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	approver.allRequest = append(approver.allRequest, request)
	return clisocket.AgentApprovalResult{IsRemembered: approver.isRemembered}, approver.result
}

func (approver *recordingAgentApprover) answer(result error, isRemembered bool) {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	approver.result, approver.isRemembered = result, isRemembered
}

func (approver *recordingAgentApprover) getRequestCount() int {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	return len(approver.allRequest)
}

func (approver *recordingAgentApprover) getLastRequest() clisocket.AgentApprovalRequest {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	return approver.allRequest[len(approver.allRequest)-1]
}

type codingAgentTest struct {
	testService   unlockedTestService
	hiddenVaultID string
	approver      *recordingAgentApprover
	session       *mcp.ClientSession
	nowMutex      sync.Mutex
	now           time.Time
}

const (
	githubPassword = "github-password-value"
	githubToken    = "github-token-value"
	githubNotes    = "recovery codes: 1111-2222"
	hiddenPassword = "hidden-vault-password"
)

func (test *codingAgentTest) getNow() time.Time {
	test.nowMutex.Lock()
	defer test.nowMutex.Unlock()
	return test.now
}

func (test *codingAgentTest) advance(duration time.Duration) {
	test.nowMutex.Lock()
	defer test.nowMutex.Unlock()
	test.now = test.now.Add(duration)
}

// createCodingAgentTest connects hisar mcp's server to the app's socket handler and an MCP client, all in memory.
func createCodingAgentTest(t *testing.T) *codingAgentTest {
	t.Helper()
	test := &codingAgentTest{testService: createUnlockedTestService(t), approver: &recordingAgentApprover{}, now: time.Now()}
	service := test.testService.service
	if _, err := service.SaveItem(test.testService.personalID, ItemDetail{
		Title: "GitHub", Username: "octocat", Password: githubPassword, URLs: []string{"https://github.com"}, Notes: githubNotes,
		Fields: []ItemField{{Name: "API token", Value: githubToken, Concealed: true}}, OneTimePassword: testOneTimePassword,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GenerateSSHKey(test.testService.personalID, "Deploy key", "ed25519"); err != nil {
		t.Fatal(err)
	}
	hiddenVault, err := service.CreateVault("Infra")
	if err != nil {
		t.Fatal(err)
	}
	test.hiddenVaultID = hiddenVault.ID
	if _, err := service.SaveItem(test.hiddenVaultID, ItemDetail{Title: "Production database", Username: "admin", Password: hiddenPassword}); err != nil {
		t.Fatal(err)
	}

	approvals := clisocket.CreateSessionApprovals(0, test.getNow)
	server := clisocket.CreateServer(clisocket.ServerConfig{
		Handler: CreateCommandLineBridge(service, approvals), Approver: &countingApprover{}, AgentApprover: test.approver,
		PeerResolver: staticPeerResolver{}, Approvals: approvals,
	})
	dial := func(context.Context) (mcpserver.AppConnection, error) {
		serverSide, clientSide := net.Pipe()
		go server.ServeConnection(context.Background(), serverSide)
		return mcpserver.CreateSocketConnection(clientSide, 10*time.Second), nil
	}
	mcpServer, closeApp := mcpserver.CreateServer(mcpserver.Config{Version: "test", Dial: dial, WorkingDirectory: t.TempDir(), Environment: []string{"PATH=" + os.Getenv("PATH")}})
	t.Cleanup(closeApp)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := mcpServer.Connect(context.Background(), serverTransport, nil); err != nil {
		t.Fatal(err)
	}
	test.session, err = mcp.NewClient(&mcp.Implementation{Name: "claude-code", Version: "2.1.0"}, nil).Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { test.session.Close() })
	return test
}

func (test *codingAgentTest) call(t *testing.T, name string, argument map[string]any) (string, bool) {
	t.Helper()
	result, err := test.session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: argument})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	allText := []string{}
	for _, content := range result.Content {
		if text, isText := content.(*mcp.TextContent); isText {
			allText = append(allText, text.Text)
		}
	}
	return strings.Join(allText, "\n"), result.IsError
}

func (test *codingAgentTest) mustCall(t *testing.T, name string, argument map[string]any, output any) string {
	t.Helper()
	text, isError := test.call(t, name, argument)
	if isError {
		t.Fatalf("%s: %s", name, text)
	}
	if output != nil {
		if err := json.Unmarshal([]byte(text), output); err != nil {
			t.Fatalf("%s: %v in %s", name, err, text)
		}
	}
	return text
}

func (test *codingAgentTest) makePersonalAvailable(t *testing.T) {
	t.Helper()
	if _, err := test.testService.service.SetVaultAvailableToAgents(test.testService.personalID, true); err != nil {
		t.Fatal(err)
	}
}

func TestAgentSeesOnlyTheVaultsAvailableToAgents(t *testing.T) {
	test := createCodingAgentTest(t)
	var vaultOutput mcpserver.ListVaultsOutput
	test.mustCall(t, "list_vaults", map[string]any{}, &vaultOutput)
	if len(vaultOutput.Vaults) != 0 || !strings.Contains(vaultOutput.Message, "Settings → Developer tools → Coding agents") {
		t.Fatalf("vaults are visible by default: %+v", vaultOutput)
	}
	if text, isError := test.call(t, "get_secret", map[string]any{"reference": "hisar://Personal/GitHub/password"}); !isError || !strings.Contains(text, "no vault is available to coding agents") {
		t.Errorf("get_secret with no vault available: %s", text)
	}

	test.makePersonalAvailable(t)
	test.mustCall(t, "list_vaults", map[string]any{}, &vaultOutput)
	if len(vaultOutput.Vaults) != 1 || vaultOutput.Vaults[0].ID != test.testService.personalID || vaultOutput.Vaults[0].Name != "Personal" || vaultOutput.Vaults[0].ItemCount != 2 {
		t.Errorf("vaults = %+v", vaultOutput)
	}
	var searchOutput mcpserver.SearchItemsOutput
	allText := test.mustCall(t, "search_items", map[string]any{"query": ""}, &searchOutput)
	if searchOutput.TotalCount != 2 || strings.Contains(allText, "Infra") || strings.Contains(allText, "Production database") || strings.Contains(allText, test.hiddenVaultID) {
		t.Errorf("search shows the hidden vault: %s", allText)
	}
	for _, query := range []string{"production", "admin", "infra"} {
		test.mustCall(t, "search_items", map[string]any{"query": query}, &searchOutput)
		if searchOutput.TotalCount != 0 {
			t.Errorf("search %q found %+v", query, searchOutput.Items)
		}
	}
	if text, isError := test.call(t, "search_items", map[string]any{"query": "", "vault": test.hiddenVaultID}); !isError || !strings.Contains(text, "no vault available to agents is named") {
		t.Errorf("search in the hidden vault: %s", text)
	}

	approvalCount := test.approver.getRequestCount()
	for _, reference := range []string{"hisar://Infra/Production%20database/password", "hisar://" + test.hiddenVaultID + "/Production%20database/password"} {
		text, isError := test.call(t, "get_secret", map[string]any{"reference": reference})
		if !isError || !strings.Contains(text, "no vault is named") || strings.Contains(text, hiddenPassword) {
			t.Errorf("get_secret %s: %s", reference, text)
		}
		text, isError = test.call(t, "run_with_secrets", map[string]any{"command": []string{"env"}, "env": map[string]string{"PASSWORD": reference}})
		if !isError || !strings.Contains(text, "no vault is named") {
			t.Errorf("run_with_secrets %s: %s", reference, text)
		}
	}
	if test.approver.getRequestCount() != approvalCount {
		t.Error("a reference into the hidden vault opened a dialog")
	}
}

func TestAgentSearchReturnsMetadataAndReferencesButNeverValues(t *testing.T) {
	test := createCodingAgentTest(t)
	test.makePersonalAvailable(t)
	var searchOutput mcpserver.SearchItemsOutput
	allText := test.mustCall(t, "search_items", map[string]any{"query": "git"}, &searchOutput)
	if searchOutput.TotalCount != 1 || len(searchOutput.Items) != 1 {
		t.Fatalf("search = %s", allText)
	}
	github := searchOutput.Items[0]
	allFieldLabel := []string{}
	for _, field := range github.Fields {
		allFieldLabel = append(allFieldLabel, field.Label+"="+field.Reference)
	}
	if github.Title != "GitHub" || github.Type != "login" || github.Username != "octocat" || !slices.Equal(github.URLs, []string{"https://github.com"}) ||
		!github.HasNotes || !github.HasOneTimePassword || github.VaultName != "Personal" || !slices.Equal(allFieldLabel, []string{
		"username=hisar://Personal/GitHub/username", "password=hisar://Personal/GitHub/password", "one-time code=hisar://Personal/GitHub/otp",
		"API token=hisar://Personal/GitHub/API%20token",
	}) {
		t.Errorf("item = %+v", github)
	}
	allText += test.mustCall(t, "search_items", map[string]any{"query": "deploy"}, &searchOutput)
	if len(searchOutput.Items) != 1 || searchOutput.Items[0].Type != "ssh-key" || len(searchOutput.Items[0].Fields) != 2 {
		t.Errorf("SSH key = %+v", searchOutput.Items)
	}
	for _, secret := range []string{githubPassword, githubToken, githubNotes, "recovery codes", "JBSWY3DPEHPK3PXP", "PRIVATE KEY"} {
		if strings.Contains(allText, secret) {
			t.Errorf("search returned %q: %s", secret, allText)
		}
	}
	if test.approver.getRequestCount() != 1 {
		t.Errorf("search asked %d times; want once for the connection", test.approver.getRequestCount())
	}
	if request := test.approver.getLastRequest(); request.Kind != clisocket.AgentRequestConnect || request.Agent.Name != "claude-code" || request.Agent.Version != "2.1.0" {
		t.Errorf("connection approval = %+v", request)
	}
}

func TestAgentGetSecretNeedsApprovalForTheItem(t *testing.T) {
	test := createCodingAgentTest(t)
	test.makePersonalAvailable(t)
	reference := map[string]any{"reference": "hisar://Personal/GitHub/password"}

	test.approver.answer(clisocket.ErrDenied, false)
	if text, isError := test.call(t, "get_secret", reference); !isError || text != "The user denied access in Hisar." {
		t.Errorf("denied: %s", text)
	}
	request := test.approver.getLastRequest()
	if request.Kind != clisocket.AgentRequestReveal || request.RevealedName != "GitHub › password" || !slices.Equal(request.AllSecretName, []string{"Personal › GitHub › password"}) ||
		request.Agent.Name != "claude-code" || !request.IsConnectionApproval {
		t.Errorf("reveal approval = %+v", request)
	}
	test.approver.answer(clisocket.TimeoutError{Timeout: 2 * time.Minute}, false)
	if text, _ := test.call(t, "get_secret", reference); text != "The user did not answer Hisar's approval dialog in time." {
		t.Errorf("timeout: %s", text)
	}

	test.approver.answer(nil, false)
	var secretOutput mcpserver.GetSecretOutput
	test.mustCall(t, "get_secret", reference, &secretOutput)
	test.mustCall(t, "get_secret", reference, &secretOutput)
	if secretOutput.Value != githubPassword || test.approver.getRequestCount() != 4 {
		t.Fatalf("allowed: %+v after %d approvals", secretOutput, test.approver.getRequestCount())
	}

	test.approver.answer(nil, true)
	test.mustCall(t, "get_secret", reference, &secretOutput)
	test.approver.answer(clisocket.ErrDenied, false)
	test.mustCall(t, "get_secret", map[string]any{"reference": "hisar://Personal/GitHub/API%20token"}, &secretOutput)
	if secretOutput.Value != githubToken || test.approver.getRequestCount() != 5 {
		t.Errorf("the item allowed for an hour asked again: %d approvals", test.approver.getRequestCount())
	}
	for range 7 {
		test.advance(9 * time.Minute)
		test.mustCall(t, "list_vaults", map[string]any{}, nil)
	}
	if text, isError := test.call(t, "get_secret", reference); !isError || text != "The user denied access in Hisar." {
		t.Errorf("the allowed item outlived an hour: %s", text)
	}

	approvalCount := test.approver.getRequestCount()
	text, isError := test.call(t, "get_secret", map[string]any{"reference": "hisar://Personal/Deploy%20key/private%20key"})
	if !isError || !strings.Contains(text, "SSH private keys never leave Hisar") || strings.Contains(text, "BEGIN") || test.approver.getRequestCount() != approvalCount {
		t.Errorf("private key: %s", text)
	}
}

func TestAgentRunsCommandsWithMaskedSecrets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the command is a Unix shell")
	}
	test := createCodingAgentTest(t)
	test.makePersonalAvailable(t)
	directory := t.TempDir()
	var runOutput mcpserver.RunOutput
	allText := test.mustCall(t, "run_with_secrets", map[string]any{
		"command":          []string{"sh", "-c", `echo "password=$GITHUB_PASSWORD"; echo "token=$GITHUB_TOKEN" >&2; exit 4`},
		"env":              map[string]string{"GITHUB_PASSWORD": "hisar://Personal/GitHub/password", "GITHUB_TOKEN": "hisar://Personal/GitHub/API%20token"},
		"workingDirectory": directory,
	}, &runOutput)
	if runOutput.ExitCode != 4 || runOutput.Stdout != "password=<concealed by hisar>\n" || runOutput.Stderr != "token=<concealed by hisar>\n" ||
		strings.Contains(allText, githubPassword) || strings.Contains(allText, githubToken) {
		t.Errorf("run = %s", allText)
	}
	request := test.approver.getLastRequest()
	if request.Kind != clisocket.AgentRequestRun || request.Agent.Name != "claude-code" || request.WorkingDirectory != directory ||
		!slices.Equal(request.CommandLine, []string{"sh", "-c", `echo "password=$GITHUB_PASSWORD"; echo "token=$GITHUB_TOKEN" >&2; exit 4`}) ||
		!slices.Equal(request.AllSecretName, []string{"Personal › GitHub › password", "Personal › GitHub › API token"}) || request.ClientPath != "/usr/local/bin/hisar" {
		t.Errorf("run approval = %+v", request)
	}
}

func TestAgentGetsOneTimeCodesAfterApproval(t *testing.T) {
	test := createCodingAgentTest(t)
	test.makePersonalAvailable(t)
	var codeOutput mcpserver.OneTimeCodeOutput
	test.mustCall(t, "get_one_time_code", map[string]any{"item": "hisar://Personal/GitHub/username"}, &codeOutput)
	if len(codeOutput.Code) != 6 || codeOutput.Period != 30 || codeOutput.SecondsRemaining < 0 || codeOutput.SecondsRemaining > 30 {
		t.Errorf("code = %+v", codeOutput)
	}
	if request := test.approver.getLastRequest(); request.Kind != clisocket.AgentRequestCode || request.RevealedName != "GitHub" ||
		!slices.Equal(request.AllSecretName, []string{"Personal › GitHub › one-time code"}) {
		t.Errorf("code approval = %+v", request)
	}
	var searchOutput mcpserver.SearchItemsOutput
	test.mustCall(t, "search_items", map[string]any{"query": "github"}, &searchOutput)
	test.mustCall(t, "get_one_time_code", map[string]any{"item": searchOutput.Items[0].ID}, &codeOutput)
	if len(codeOutput.Code) != 6 {
		t.Errorf("code by item ID = %+v", codeOutput)
	}
	if text, isError := test.call(t, "get_one_time_code", map[string]any{"item": "hisar://Personal/Deploy%20key/fingerprint"}); !isError || !strings.Contains(text, "has no one-time password") {
		t.Errorf("an item without a one-time password: %s", text)
	}
}

func TestVaultAvailableToAgentsIsSavedAndOffByDefault(t *testing.T) {
	testService := createUnlockedTestService(t)
	service := testService.service
	if allVaultID := service.ListAllAgentVaultID(); len(allVaultID) != 0 {
		t.Fatalf("available by default: %q", allVaultID)
	}
	if allVaultID, err := service.SetVaultAvailableToAgents(testService.personalID, true); err != nil || !slices.Equal(allVaultID, []string{testService.personalID}) {
		t.Fatalf("turn on: %q %v", allVaultID, err)
	}
	service.SetVaultAvailableToAgents(testService.personalID, true)
	service.SetVaultAvailableToAgents(testService.familyID, true)
	if allVaultID, _ := service.SetVaultAvailableToAgents(testService.familyID, false); !slices.Equal(allVaultID, []string{testService.personalID}) {
		t.Errorf("turn off: %q", allVaultID)
	}
	state, err := localstate.Load(service.config.StatePath)
	if err != nil || !slices.Equal(state.AllAgentVaultID, []string{testService.personalID}) {
		t.Errorf("saved %q %v", state.AllAgentVaultID, err)
	}
	if _, err := service.SetVaultAvailableToAgents("no-such-vault", true); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown vault: %v", err)
	}
	service.Lock()
	if _, err := service.SetVaultAvailableToAgents(testService.personalID, false); !errors.Is(err, ErrLocked) {
		t.Errorf("locked: %v", err)
	}
	if allVaultID := service.ListAllAgentVaultID(); !slices.Equal(allVaultID, []string{testService.personalID}) {
		t.Errorf("after lock: %q", allVaultID)
	}
}
