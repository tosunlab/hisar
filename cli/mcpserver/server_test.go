package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tryhisar/hisar/core/cliprotocol"
)

type fakeApp struct {
	mutex       sync.Mutex
	requests    []cliprotocol.Request
	dialCount   int
	closeCount  int
	dialError   error
	answer      func(request cliprotocol.Request) (any, error)
	isConnected bool
	// features is what the app announces in hello; startTestServer makes nil this build's.
	features []string
}

type fakeAppConnection struct {
	app *fakeApp
}

func (connection *fakeAppConnection) Call(_ context.Context, request cliprotocol.Request, result any) error {
	app := connection.app
	app.mutex.Lock()
	app.requests = append(app.requests, request)
	answer := app.answer
	app.mutex.Unlock()
	if request.Type == cliprotocol.TypeAgentHello {
		return nil
	}
	answered, err := answer(request)
	if err != nil {
		return err
	}
	encoded, _ := json.Marshal(answered)
	return json.Unmarshal(encoded, result)
}

func (connection *fakeAppConnection) Close() error {
	connection.app.mutex.Lock()
	defer connection.app.mutex.Unlock()
	connection.app.closeCount++
	return nil
}

func (connection *fakeAppConnection) HasFeature(feature string) bool {
	connection.app.mutex.Lock()
	defer connection.app.mutex.Unlock()
	return slices.Contains(connection.app.features, feature)
}

func (app *fakeApp) dial(context.Context) (AppConnection, error) {
	app.mutex.Lock()
	defer app.mutex.Unlock()
	app.dialCount++
	if app.dialError != nil {
		return nil, app.dialError
	}
	return &fakeAppConnection{app: app}, nil
}

func (app *fakeApp) listRequestTypes() []string {
	app.mutex.Lock()
	defer app.mutex.Unlock()
	types := []string{}
	for _, request := range app.requests {
		types = append(types, request.Type)
	}
	return types
}

func (app *fakeApp) getLastRequest() cliprotocol.Request {
	app.mutex.Lock()
	defer app.mutex.Unlock()
	return app.requests[len(app.requests)-1]
}

const testSecret = "s3cr3t-token-value"

func answerWithSecret(request cliprotocol.Request) (any, error) {
	switch request.Type {
	case cliprotocol.TypeAgentListVault:
		return cliprotocol.AgentVaultListResult{Vaults: []cliprotocol.AgentVault{{ID: "v1", Name: "Work", ItemCount: 1}}}, nil
	case cliprotocol.TypeAgentResolveSecret, cliprotocol.TypeAgentRevealSecret:
		values := []string{}
		for range request.References {
			values = append(values, testSecret)
		}
		return cliprotocol.ResolveSecretResult{Values: values}, nil
	case cliprotocol.TypeAgentGetCode:
		return cliprotocol.CodeResult{Code: "123456", ExpiresAt: "2026-10-11T09:00:30Z", Period: 30}, nil
	}
	return nil, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, "unexpected "+request.Type)
}

type testServer struct {
	app      *fakeApp
	session  *mcp.ClientSession
	mutex    sync.Mutex
	progress []*mcp.ProgressNotificationParams
}

func createTestServer(t *testing.T, app *fakeApp) *testServer {
	t.Helper()
	return startTestServer(t, app, Config{})
}

// startTestServer fills in what config leaves empty and connects a client that records progress notifications.
func startTestServer(t *testing.T, app *fakeApp, config Config) *testServer {
	t.Helper()
	if app.answer == nil {
		app.answer = answerWithSecret
	}
	if app.features == nil {
		app.features = cliprotocol.ListFeatures()
	}
	config.Version, config.Dial, config.KillDelay = "1.2.3", app.dial, 200*time.Millisecond
	config.Now = func() time.Time { return time.Date(2026, 10, 11, 9, 0, 12, 500_000_000, time.UTC) }
	if config.Environment == nil {
		config.Environment = []string{"PATH=" + os.Getenv("PATH"), "HOME=/nowhere", "TOKEN=from-the-agent"}
	}
	if config.WorkingDirectory == "" {
		config.WorkingDirectory = t.TempDir()
	}
	server, closeApp := NewServer(config)
	t.Cleanup(closeApp)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatal(err)
	}
	test := &testServer{app: app}
	client := mcp.NewClient(&mcp.Implementation{Name: "claude-code", Title: "", Version: "2.1.0"}, &mcp.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, request *mcp.ProgressNotificationClientRequest) {
			test.mutex.Lock()
			defer test.mutex.Unlock()
			test.progress = append(test.progress, request.Params)
		},
	})
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	test.session = session
	return test
}

func (server *testServer) callTool(t *testing.T, name string, argument any) *mcp.CallToolResult {
	t.Helper()
	result, err := server.session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: argument})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return result
}

// listProgress returns the progress notifications in the order the server sent them.
func (server *testServer) listProgress() []*mcp.ProgressNotificationParams {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	progress := slices.Clone(server.progress)
	slices.SortFunc(progress, func(first, second *mcp.ProgressNotificationParams) int { return int(first.Progress - second.Progress) })
	return progress
}

func waitFor(t *testing.T, description string, condition func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !condition(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting until %s", description)
		}
	}
}

func getText(result *mcp.CallToolResult) string {
	texts := []string{}
	for _, content := range result.Content {
		if text, isText := content.(*mcp.TextContent); isText {
			texts = append(texts, text.Text)
		}
	}
	return strings.Join(texts, "\n")
}

func decodeOutput[Output any](t *testing.T, result *mcp.CallToolResult) Output {
	t.Helper()
	var output Output
	if result.IsError {
		t.Fatalf("tool error: %s", getText(result))
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatal(err)
	}
	return output
}

func requireUnix(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the commands are Unix programs")
	}
}

func TestToolsAreListedWithSchemasForModels(t *testing.T) {
	server := createTestServer(t, &fakeApp{})
	result, err := server.session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	toolsByName := map[string]*mcp.Tool{}
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
		toolsByName[tool.Name] = tool
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"get_one_time_code", "get_secret", "list_vaults", "run_with_secrets", "search_items"}) {
		t.Fatalf("tools = %q", names)
	}
	required := map[string][]string{
		"list_vaults": nil, "search_items": {"query"}, "run_with_secrets": nil, "get_one_time_code": {"item"}, "get_secret": {"reference"},
	}
	for name, expectedValues := range required {
		schema, _ := json.Marshal(toolsByName[name].InputSchema)
		var decoded struct {
			Type       string                     `json:"type"`
			Required   []string                   `json:"required"`
			Properties map[string]json.RawMessage `json:"properties"`
		}
		json.Unmarshal(schema, &decoded)
		if decoded.Type != "object" || !slices.Equal(decoded.Required, expectedValues) {
			t.Errorf("%s input schema: %s", name, schema)
		}
		if toolsByName[name].OutputSchema == nil || toolsByName[name].Description == "" {
			t.Errorf("%s has no output schema or description", name)
		}
	}
	runSchema, _ := json.Marshal(toolsByName["run_with_secrets"].InputSchema)
	for _, property := range []string{`"command"`, `"shell"`, `"env"`, `"envFiles"`, `"stdin"`, `"workingDirectory"`, `"timeoutSeconds"`, `hisar://vault/item/field`, `at most 3600`} {
		if !strings.Contains(string(runSchema), property) {
			t.Errorf("run_with_secrets schema lacks %s: %s", property, runSchema)
		}
	}
	if runAnnotations := toolsByName["run_with_secrets"].Annotations; runAnnotations.ReadOnlyHint || runAnnotations.DestructiveHint == nil || !*runAnnotations.DestructiveHint {
		t.Error("run_with_secrets is not marked destructive")
	}
	initializeResult := server.session.InitializeResult()
	if initializeResult.ServerInfo.Name != "hisar" || initializeResult.ServerInfo.Version != "1.2.3" || initializeResult.Instructions == "" {
		t.Errorf("server info %+v, instructions %q", initializeResult.ServerInfo, initializeResult.Instructions)
	}
}

func TestNoRevealLeavesOutTheToolsThatReturnValues(t *testing.T) {
	server := startTestServer(t, &fakeApp{}, Config{IsRevealDisabled: true})
	result, err := server.session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
		if strings.Contains(tool.Description, "get_secret") {
			t.Errorf("%s mentions get_secret: %s", tool.Name, tool.Description)
		}
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"list_vaults", "run_with_secrets", "search_items"}) {
		t.Errorf("tools = %q", names)
	}
	if instructions := server.session.InitializeResult().Instructions; strings.Contains(instructions, "get_secret") || !strings.Contains(instructions, "hisar run --env-file .env --") {
		t.Errorf("instructions = %q", instructions)
	}
	if _, err := server.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_secret", Arguments: map[string]any{"reference": "hisar://Work/Stripe/api%20key"}}); err == nil {
		t.Error("get_secret answered")
	}
	if server.app.dialCount != 0 {
		t.Errorf("get_secret reached the app (%d dials)", server.app.dialCount)
	}
}

// TestInstructionsLeadWithTheToolsAndTheShell: Codex advises keeping the first 512 characters of server instructions
// self-contained, so they hold the tools and the shell fallback; the sandbox advice follows in both modes.
func TestInstructionsLeadWithTheToolsAndTheShell(t *testing.T) {
	for _, isRevealDisabled := range []bool{false, true} {
		instructions := startTestServer(t, &fakeApp{}, Config{IsRevealDisabled: isRevealDisabled}).session.InitializeResult().Instructions
		lead := instructions[:min(512, len(instructions))]
		for _, phrase := range []string{"search_items", "run_with_secrets", "hisar run --env-file .env --", "in your shell instead."} {
			if !strings.Contains(lead, phrase) {
				t.Errorf("no reveal %v: the first 512 characters lack %q", isRevealDisabled, phrase)
			}
		}
		if !strings.Contains(instructions, "If hisar in your shell says a sandbox blocks its socket, use run_with_secrets, or ask the user to let the command run outside your sandbox.") {
			t.Errorf("no reveal %v: no sandbox advice in %q", isRevealDisabled, instructions)
		}
	}
}

func TestServerConnectsLazilyAndIntroducesTheAgentOnce(t *testing.T) {
	app := &fakeApp{}
	server := createTestServer(t, app)
	if app.dialCount != 0 {
		t.Fatal("hisar mcp connected to the app before a tool call")
	}
	server.callTool(t, "list_vaults", map[string]any{})
	server.callTool(t, "list_vaults", map[string]any{})
	if app.dialCount != 1 || !slices.Equal(app.listRequestTypes(), []string{cliprotocol.TypeAgentHello, cliprotocol.TypeAgentListVault, cliprotocol.TypeAgentListVault}) {
		t.Fatalf("dials %d, requests %q", app.dialCount, app.listRequestTypes())
	}
	hello := app.requests[0]
	if hello.AgentName != "claude-code" || hello.AgentVersion != "2.1.0" {
		t.Errorf("agent.hello = %+v", hello)
	}
}

func TestServerConnectsAgainAfterALostConnection(t *testing.T) {
	app := &fakeApp{}
	isLost := true
	app.answer = func(request cliprotocol.Request) (any, error) {
		if isLost {
			isLost = false
			return nil, errors.New("lost the connection to the Hisar app: EOF")
		}
		return answerWithSecret(request)
	}
	server := createTestServer(t, app)
	if result := server.callTool(t, "list_vaults", map[string]any{}); !result.IsError || !strings.Contains(getText(result), "lost the connection") {
		t.Fatalf("lost connection: %s", getText(result))
	}
	output := decodeOutput[ListVaultsOutput](t, server.callTool(t, "list_vaults", map[string]any{}))
	if app.dialCount != 2 || app.closeCount != 1 || len(output.Vaults) != 1 {
		t.Errorf("dials %d, closes %d, output %+v", app.dialCount, app.closeCount, output)
	}
}

func TestAppNotRunningSaysWhatToDo(t *testing.T) {
	server := createTestServer(t, &fakeApp{dialError: errors.New("the Hisar app is not running, or does not offer /tmp/cli.sock")})
	result := server.callTool(t, "search_items", map[string]any{"query": "stripe"})
	if text := getText(result); !result.IsError || !strings.Contains(text, "Hisar is not available: the Hisar app is not running") || !strings.Contains(text, "open the Hisar app") {
		t.Errorf("result: %s", text)
	}
}

func TestListAndSearchWithoutAgentVaultsExplainHowToEnableOne(t *testing.T) {
	app := &fakeApp{answer: func(request cliprotocol.Request) (any, error) {
		if request.Type == cliprotocol.TypeAgentSearchItem {
			return cliprotocol.AgentItemSearchResult{}, nil
		}
		return cliprotocol.AgentVaultListResult{}, nil
	}}
	server := createTestServer(t, app)
	vaultOutput := decodeOutput[ListVaultsOutput](t, server.callTool(t, "list_vaults", map[string]any{}))
	if vaultOutput.Vaults == nil || len(vaultOutput.Vaults) != 0 || !strings.Contains(vaultOutput.Message, "“Available to agents”") {
		t.Errorf("list: %+v", vaultOutput)
	}
	searchOutput := decodeOutput[SearchItemsOutput](t, server.callTool(t, "search_items", map[string]any{"query": "", "vault": "Work"}))
	if searchOutput.Items == nil || !strings.Contains(searchOutput.Message, "Settings → Developer tools → Coding agents") {
		t.Errorf("search: %+v", searchOutput)
	}
	search := app.requests[len(app.requests)-2]
	if search.Type != cliprotocol.TypeAgentSearchItem || search.Vault != "Work" {
		t.Errorf("search request = %+v", search)
	}
}

func TestRunWithSecretsMasksTheOutputAndReturnsTheExitCode(t *testing.T) {
	requireUnix(t)
	app := &fakeApp{}
	server := createTestServer(t, app)
	result := server.callTool(t, "run_with_secrets", map[string]any{
		"command": []string{"sh", "-c", `echo "token=$TOKEN home=$HOME"; echo "error: $TOKEN" >&2; printf %s "$TOKEN" | tr a-z A-Z; exit 3`},
		"env":     map[string]string{"TOKEN": "hisar://Work/Stripe/api%20key"},
	})
	output := decodeOutput[RunOutput](t, result)
	if output.ExitCode != 3 || output.Stdout != "token=<concealed by hisar> home=/nowhere\nS3CR3T-TOKEN-VALUE" || output.Stderr != "error: <concealed by hisar>\n" ||
		output.IsTimedOut || output.IsStdoutTruncated || output.DurationSeconds <= 0 {
		t.Errorf("output = %+v", output)
	}
	if strings.Contains(getText(result), testSecret) {
		t.Errorf("the text content shows the secret: %s", getText(result))
	}
	request := app.getLastRequest()
	if request.Type != cliprotocol.TypeAgentResolveSecret || !slices.Equal(request.References, []string{"hisar://Work/Stripe/api%20key"}) ||
		request.CommandLine[0] != "sh" || request.WorkingDirectory == "" {
		t.Errorf("app request = %+v", request)
	}
}

func TestRunWithSecretsUsesNoShell(t *testing.T) {
	requireUnix(t)
	server := createTestServer(t, &fakeApp{})
	output := decodeOutput[RunOutput](t, server.callTool(t, "run_with_secrets", map[string]any{
		"command": []string{"echo", "$TOKEN;", "ls", "`id`", "|", "cat"},
		"env":     map[string]string{"TOKEN": "hisar://Work/Stripe/api%20key"},
	}))
	if output.Stdout != "$TOKEN; ls `id` | cat\n" || output.ExitCode != 0 {
		t.Errorf("output = %+v", output)
	}
}

func TestRunWithSecretsRunsInTheWorkingDirectoryWithTheSameReferenceTwice(t *testing.T) {
	requireUnix(t)
	app := &fakeApp{}
	server := createTestServer(t, app)
	directory := t.TempDir()
	os.WriteFile(filepath.Join(directory, "marker.txt"), []byte("here"), 0o600)
	output := decodeOutput[RunOutput](t, server.callTool(t, "run_with_secrets", map[string]any{
		"command":          []string{"sh", "-c", `cat marker.txt; test "$FIRST" = "$SECOND" && echo same`},
		"env":              map[string]string{"FIRST": "hisar://Work/Stripe/api%20key", "SECOND": "hisar://Work/Stripe/api%20key"},
		"workingDirectory": directory,
	}))
	if output.Stdout != "heresame\n" {
		t.Errorf("output = %+v", output)
	}
	if request := app.getLastRequest(); len(request.References) != 1 || request.WorkingDirectory != directory {
		t.Errorf("app request = %+v", request)
	}
}

func TestRunWithSecretsKeepsTheStartAndEndOfLongOutput(t *testing.T) {
	requireUnix(t)
	server := createTestServer(t, &fakeApp{})
	output := decodeOutput[RunOutput](t, server.callTool(t, "run_with_secrets", map[string]any{
		"command": []string{"sh", "-c", `printf start; head -c 200000 /dev/zero | tr '\0' a; printf "$TOKEN-end"`},
		"env":     map[string]string{"TOKEN": "hisar://Work/Stripe/api%20key"},
	}))
	if !output.IsStdoutTruncated || output.IsStderrTruncated || len(output.Stdout) > MaximumOutputLength+100 ||
		!strings.HasPrefix(output.Stdout, "starta") || !strings.HasSuffix(output.Stdout, "a<concealed by hisar>-end") ||
		!strings.Contains(output.Stdout, fmt.Sprintf("[… %d bytes not shown …]", len("start")+200000+len("<concealed by hisar>-end")-MaximumOutputLength)) {
		t.Errorf("truncated output: %d bytes, %q … %q", len(output.Stdout), output.Stdout[:20], output.Stdout[len(output.Stdout)-60:])
	}
}

func TestRunWithSecretsStopsTheCommandAtTheTimeout(t *testing.T) {
	requireUnix(t)
	server := createTestServer(t, &fakeApp{})
	startedAt := time.Now()
	output := decodeOutput[RunOutput](t, server.callTool(t, "run_with_secrets", map[string]any{
		"command":        []string{"sh", "-c", `trap "" TERM; echo started; sleep 30 & wait`},
		"env":            map[string]string{"TOKEN": "hisar://Work/Stripe/api%20key"},
		"timeoutSeconds": 1,
	}))
	if !output.IsTimedOut || output.ExitCode != 128+9 || output.Stdout != "started\n" || time.Since(startedAt) > 10*time.Second {
		t.Errorf("output = %+v after %v", output, time.Since(startedAt))
	}
}

func TestRunWithSecretsChecksEverythingBeforeAsking(t *testing.T) {
	app := &fakeApp{}
	server := createTestServer(t, app)
	directory := t.TempDir()
	brokenFile, plainFile := filepath.Join(directory, "broken.env"), filepath.Join(directory, "plain.env")
	os.WriteFile(brokenFile, []byte("API_KEY=hisar://Work/Stripe\n"), 0o600)
	os.WriteFile(plainFile, []byte("MODE=test\n"), 0o600)
	token := map[string]string{"TOKEN": "hisar://Work/Stripe/password"}
	cases := []struct {
		argument map[string]any
		message  string
	}{
		{map[string]any{"command": []string{"hisar-no-such-command"}, "env": token}, "hisar-no-such-command: command not found"},
		{map[string]any{"command": []string{}, "env": token}, "command is empty"},
		{map[string]any{"command": []string{"env"}, "shell": "env", "env": token}, "pass command or shell, not both"},
		{map[string]any{"command": []string{"env"}, "env": map[string]string{}}, "no secret to use"},
		{map[string]any{"shell": "env", "envFiles": []string{plainFile}}, "no secret to use"},
		{map[string]any{"command": []string{"env"}, "env": map[string]string{"TOKEN": "plain value"}}, "TOKEN: the value must be a hisar://vault/item/field reference"},
		{map[string]any{"command": []string{"env"}, "env": map[string]string{"BAD-NAME": "hisar://Work/Stripe/password"}}, `"BAD-NAME" is not a valid variable name`},
		{map[string]any{"command": []string{"env"}, "env": map[string]string{"TOKEN": "hisar://Work/Stripe"}}, "has three parts"},
		{map[string]any{"shell": "env", "envFiles": []string{brokenFile}}, "API_KEY: hisar://Work/Stripe: a secret reference has three parts"},
		{map[string]any{"shell": "env", "envFiles": []string{"missing.env"}, "env": token}, "missing.env: no such file or directory"},
		{map[string]any{"command": []string{"cat"}, "stdin": "hisar://Work"}, "stdin: hisar://Work: a secret reference has three parts"},
		{map[string]any{"command": []string{"env"}, "env": token, "timeoutSeconds": 3601}, "timeoutSeconds must be between 1 and 3600"},
		{map[string]any{"command": []string{"env"}, "env": token, "workingDirectory": "/no/such/directory"}, "working directory /no/such/directory does not exist"},
	}
	for _, testCase := range cases {
		result := server.callTool(t, "run_with_secrets", testCase.argument)
		if !result.IsError || !strings.Contains(getText(result), testCase.message) {
			t.Errorf("%v: %s", testCase.argument, getText(result))
		}
	}
	if app.dialCount != 0 {
		t.Errorf("a command that cannot run reached the app (%d dials)", app.dialCount)
	}
}

func TestDenialsAreClearToolErrors(t *testing.T) {
	app := &fakeApp{}
	server := createTestServer(t, app)
	cases := []struct {
		answer  error
		message string
	}{
		{cliprotocol.NewError(cliprotocol.ErrorDenied, "Hisar denied the request (it was declined)"), "The user denied access in Hisar."},
		{cliprotocol.NewError(cliprotocol.ErrorDenied, "Hisar denied the request (it was not answered within 2 minutes)"), "The user did not answer Hisar's approval dialog in time."},
		{cliprotocol.NewError(cliprotocol.ErrorNotFound, "hisar://Personal/Bank/password: no vault is named \"Personal\""), "no vault is named \"Personal\""},
		{cliprotocol.NewError(cliprotocol.ErrorSignedOut, "signed out"), "Hisar is not signed in."},
	}
	for _, testCase := range cases {
		app.answer = func(cliprotocol.Request) (any, error) { return nil, testCase.answer }
		result := server.callTool(t, "get_secret", map[string]any{"reference": "hisar://Personal/Bank/password"})
		if !result.IsError || getText(result) != testCase.message && !strings.Contains(getText(result), testCase.message) {
			t.Errorf("%v: %s", testCase.answer, getText(result))
		}
	}
	if app.dialCount != 1 {
		t.Errorf("an answer from the app closed the connection (%d dials)", app.dialCount)
	}
}

func TestGetSecretAndOneTimeCode(t *testing.T) {
	app := &fakeApp{}
	server := createTestServer(t, app)
	secretOutput := decodeOutput[GetSecretOutput](t, server.callTool(t, "get_secret", map[string]any{"reference": "hisar://Work/Stripe/api%20key"}))
	if secretOutput.Value != testSecret {
		t.Errorf("value = %q", secretOutput.Value)
	}
	if request := app.getLastRequest(); request.Type != cliprotocol.TypeAgentRevealSecret || !slices.Equal(request.References, []string{"hisar://Work/Stripe/api%20key"}) {
		t.Errorf("reveal request = %+v", request)
	}
	if result := server.callTool(t, "get_secret", map[string]any{"reference": "Stripe api key"}); !result.IsError || !strings.Contains(getText(result), "hisar://") {
		t.Errorf("not a reference: %s", getText(result))
	}

	codeOutput := decodeOutput[OneTimeCodeOutput](t, server.callTool(t, "get_one_time_code", map[string]any{"item": "hisar://Work/GitHub/username"}))
	if codeOutput.Code != "123456" || codeOutput.SecondsRemaining != 18 || codeOutput.Period != 30 {
		t.Errorf("code = %+v", codeOutput)
	}
	if request := app.getLastRequest(); request.Type != cliprotocol.TypeAgentGetCode || !slices.Equal(request.References, []string{"hisar://Work/GitHub/username"}) || request.ItemID != "" {
		t.Errorf("code request = %+v", request)
	}
	server.callTool(t, "get_one_time_code", map[string]any{"item": "item-github"})
	if request := app.getLastRequest(); request.ItemID != "item-github" || len(request.References) != 0 {
		t.Errorf("code request by ID = %+v", request)
	}
}

func TestOutputCaptureKeepsTheStartAndTheEnd(t *testing.T) {
	var capture outputCapture
	for index := range 3 * MaximumOutputLength / 1000 {
		capture.Write([]byte(fmt.Sprintf("%0999d\n", index)))
	}
	text, isTruncated := capture.getText()
	if !isTruncated || !strings.HasPrefix(text, "0000") || !strings.HasSuffix(text, fmt.Sprintf("%0999d\n", 3*MaximumOutputLength/1000-1)) {
		t.Errorf("truncated %v, %d bytes", isTruncated, len(text))
	}
	var short outputCapture
	short.Write([]byte("short\xff"))
	if text, isTruncated := short.getText(); isTruncated || text != "short�" {
		t.Errorf("short output %q %v", text, isTruncated)
	}
}
