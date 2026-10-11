package vault

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tryhisar/hisar/cli/mcpserver"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/desktop/commandline"
)

// TestIntegrationAgentThroughApp runs the built hisar mcp as an MCP server on stdio against the app backend signed in to
// a real server; the test approver approves every dialog.
func TestIntegrationAgentThroughApp(t *testing.T) {
	serverURL, inviteLink, binaryPath := os.Getenv("HISAR_TEST_SERVER_URL"), os.Getenv("HISAR_TEST_AGENT_INVITE_LINK"), os.Getenv("HISAR_TEST_CLI_BINARY")
	if serverURL == "" || inviteLink == "" || binaryPath == "" {
		t.Skip("HISAR_TEST_SERVER_URL, HISAR_TEST_AGENT_INVITE_LINK and HISAR_TEST_CLI_BINARY not set")
	}
	const apiToken, hiddenSecret = "stripe-api-token-from-hisar", "infra-root-password"
	service, _, _ := createTestService(t, filepath.Join(t.TempDir(), "desktop.json"), createMemorySecretKeyStore())
	if _, err := service.SignUp(inviteLink, "agent@example.com", "Agent Developer", "agent master password"); err != nil {
		t.Fatal(err)
	}
	personalID := findPersonalVaultID(t, service)
	if _, err := service.SaveItem(personalID, ItemDetail{
		Title: "Stripe", Username: "billing@example.com", Password: "stripe-dashboard-password", URLs: []string{"https://dashboard.stripe.com"},
		Notes: "restricted key notes", Fields: []ItemField{{Name: "api key", Value: apiToken, Concealed: true}},
	}); err != nil {
		t.Fatal(err)
	}
	hiddenVault, err := service.CreateVault("Infra")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveItem(hiddenVault.ID, ItemDetail{Title: "Root server", Username: "root", Password: hiddenSecret}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetVaultAvailableToAgents(personalID, true); err != nil {
		t.Fatal(err)
	}

	socketDirectory, err := os.MkdirTemp("/tmp", "hisar-agent-integration-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socketDirectory) })
	socketPath := filepath.Join(socketDirectory, "hisar", "cli.sock")
	approver := &recordingAgentApprover{}
	approvals := commandline.CreateSessionApprovals(0, nil)
	runner := commandline.CreateRunner(commandline.CreateServer(commandline.ServerConfig{
		Handler: CreateCommandLineBridge(service, approvals), Approver: &countingApprover{}, AgentApprover: approver, Approvals: approvals,
	}), socketPath)
	runner.Start()
	t.Cleanup(runner.Stop)
	for range 200 {
		if connection, err := net.Dial("unix", socketPath); err == nil {
			connection.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	command := exec.Command(binaryPath, "mcp")
	command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), cliprotocol.SocketEnvironment + "=" + socketPath}
	command.Dir = t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "claude-code", Version: "2.1.0"}, nil).Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	allOutput := []string{}
	call := func(name string, argument map[string]any, output any) {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: argument})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		text := result.Content[0].(*mcp.TextContent).Text
		allOutput = append(allOutput, text)
		if result.IsError {
			t.Fatalf("%s: %s", name, text)
		}
		if err := json.Unmarshal([]byte(text), output); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}

	var vaultOutput mcpserver.ListVaultsOutput
	call("list_vaults", map[string]any{}, &vaultOutput)
	if len(vaultOutput.Vaults) != 1 || vaultOutput.Vaults[0].Name != "Personal" || vaultOutput.Vaults[0].ItemCount != 1 {
		t.Errorf("list_vaults = %+v", vaultOutput)
	}
	var searchOutput mcpserver.SearchItemsOutput
	call("search_items", map[string]any{"query": ""}, &searchOutput)
	if searchOutput.TotalCount != 1 || searchOutput.Items[0].Title != "Stripe" || !searchOutput.Items[0].HasNotes {
		t.Fatalf("search_items = %+v", searchOutput)
	}
	apiKeyReference := ""
	for _, field := range searchOutput.Items[0].Fields {
		if field.Label == "api key" {
			apiKeyReference = field.Reference
		}
	}
	if apiKeyReference != "hisar://Personal/Stripe/api%20key" {
		t.Fatalf("api key reference = %q", apiKeyReference)
	}
	var runOutput mcpserver.RunOutput
	call("run_with_secrets", map[string]any{
		"command": []string{"sh", "-c", `echo "key=$STRIPE_API_KEY"; exit 2`},
		"env":     map[string]string{"STRIPE_API_KEY": apiKeyReference},
	}, &runOutput)
	if runOutput.ExitCode != 2 || runOutput.Stdout != "key=<concealed by hisar>\n" {
		t.Errorf("run_with_secrets = %+v", runOutput)
	}
	var secretOutput mcpserver.GetSecretOutput
	call("get_secret", map[string]any{"reference": apiKeyReference}, &secretOutput)
	if secretOutput.Value != apiToken {
		t.Errorf("get_secret = %+v", secretOutput)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_secret", Arguments: map[string]any{"reference": "hisar://Infra/Root%20server/password"}})
	if err != nil || !result.IsError || result.Content[0].(*mcp.TextContent).Text != `hisar://Infra/Root%20server/password: no vault is named "Infra"` {
		t.Fatalf("hidden vault: %+v %v", result, err)
	}

	allKind := []string{}
	for _, request := range approver.allRequest {
		allKind = append(allKind, request.Kind)
		if request.Agent.Name != "claude-code" || request.Agent.Version != "2.1.0" || !strings.HasSuffix(request.ClientPath, filepath.Base(binaryPath)) {
			t.Errorf("approval request = %+v", request)
		}
	}
	if !slices.Equal(allKind, []string{commandline.AgentRequestConnect, commandline.AgentRequestRun, commandline.AgentRequestReveal}) {
		t.Errorf("approvals = %q", allKind)
	}
	expectedDirectory, _ := filepath.EvalSymlinks(command.Dir)
	if run := approver.allRequest[1]; !slices.Equal(run.AllSecretName, []string{"Personal › Stripe › api key"}) || run.CommandLine[0] != "sh" || run.WorkingDirectory != expectedDirectory {
		t.Errorf("run approval = %+v", run)
	}
	allText := strings.Join(allOutput, "\n")
	for _, hidden := range []string{"Infra", "Root server", hiddenSecret, hiddenVault.ID, "restricted key notes", "stripe-dashboard-password"} {
		if strings.Contains(allText, hidden) {
			t.Errorf("an agent saw %q:\n%s", hidden, allText)
		}
	}
}
