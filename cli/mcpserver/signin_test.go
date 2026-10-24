package mcpserver

import (
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tryhisar/hisar/core/cliprotocol"
)

func answerSignIn(request cliprotocol.Request) (any, error) {
	if request.Type != cliprotocol.TypeAgentSignIn {
		return answerWithSecret(request)
	}
	return cliprotocol.AgentSignInResult{
		Outcome: cliprotocol.SignInSubmitted, Cleanup: cliprotocol.SignInCleanupPageLeft, Website: "https://tosunlab.com", Browser: "Google Chrome",
		Login:   cliprotocol.AgentSignInLogin{ID: "tosunlab", VaultID: "v1", VaultName: "Agents", Title: "Tosun Lab", Username: "user@example.com"},
		Steps:   []cliprotocol.AgentSignInStep{{Filled: []string{"username"}, IsSubmitted: true}, {Filled: []string{"password", "one-time code"}, IsSubmitted: true}},
		Message: "Hisar signed in to https://tosunlab.com as user@example.com and submitted it. Take a screenshot to see the page.",
	}, nil
}

func TestSignInSendsTheWindowAndAnswersWithTheMessage(t *testing.T) {
	app := &fakeApp{answer: answerSignIn}
	server := startTestServer(t, app, Config{IsRevealDisabled: true})
	result := server.callTool(t, "sign_in", map[string]any{"pid": 81234, "window_id": 40512, "login": "tosunlab"})
	if result.IsError {
		t.Fatalf("sign_in failed: %s", getText(result))
	}
	request := app.getLastRequest()
	if request.Type != cliprotocol.TypeAgentSignIn || request.Window == nil || *request.Window != (cliprotocol.Window{PID: 81234, WindowID: 40512}) || request.ItemID != "tosunlab" {
		t.Errorf("request %+v", request)
	}
	if text := getText(result); text != "Hisar signed in to https://tosunlab.com as user@example.com and submitted it. Take a screenshot to see the page." {
		t.Errorf("text %q", text)
	}
	output := decodeOutput[SignInOutput](t, result)
	if output.Outcome != "submitted" || output.Cleanup != "page-left" || output.Login.Username != "user@example.com" || len(output.Steps) != 2 ||
		!slices.Equal(output.Steps[1].Filled, []string{"password", "one-time code"}) || !output.Steps[1].Submitted {
		t.Errorf("output %+v", output)
	}
}

func TestSignInDescriptionTellsTheModelWhatEachOutcomeMeans(t *testing.T) {
	server := startTestServer(t, &fakeApp{}, Config{IsRevealDisabled: true})
	tools, err := server.session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	index := slices.IndexFunc(tools.Tools, func(tool *mcp.Tool) bool { return tool.Name == "sign_in" })
	if index < 0 {
		t.Fatal("no sign_in")
	}
	for _, phrase := range []string{"pid", "window_id", "list_windows", "native computer_use", "not a CDP browser tool", "submitted", "not-accepted", "stopped", "cleanup-failed", "fields-removed", "Never type a password yourself"} {
		if !strings.Contains(tools.Tools[index].Description, phrase) {
			t.Errorf("the description lacks %q", phrase)
		}
	}
	if instructions := server.session.InitializeResult().Instructions; !strings.Contains(instructions, "call sign_in with the window's pid and window_id") {
		t.Errorf("instructions %q", instructions)
	}
}

func TestSignInRefusalsReachTheModel(t *testing.T) {
	refusal := "No login in the vaults available to agents is saved for https://tosunlab-login.example.net. Ask the user to add one in Hisar or make its vault available to agents. Don't ask for the password."
	for _, testCase := range []struct {
		err  error
		text string
	}{
		{cliprotocol.NewError(cliprotocol.ErrorSignInRefused, refusal), refusal},
		{cliprotocol.NewError(cliprotocol.ErrorDenied, "Hisar denied the request (it was declined)"), ErrDeniedInHisar.Error()},
		{cliprotocol.NewError(cliprotocol.ErrorDenied, "Hisar denied the request (it was not answered within 2 minutes)"), ErrDialogTimedOut.Error()},
		{cliprotocol.NewError(cliprotocol.ErrorDenied, "Hisar denied the request: it locked or signed out, or the agent's session ended, before the request ran"),
			"Hisar stopped the request because it locked or signed out, or the agent session ended."},
	} {
		app := &fakeApp{answer: func(cliprotocol.Request) (any, error) { return nil, testCase.err }}
		result := startTestServer(t, app, Config{}).callTool(t, "sign_in", map[string]any{"pid": 81234, "window_id": 40512})
		if !result.IsError || getText(result) != testCase.text {
			t.Errorf("%v: %+v", testCase.err, result.Content)
		}
	}
}

func TestSignInNeedsAnAppThatSignsIn(t *testing.T) {
	app := &fakeApp{answer: answerSignIn, features: []string{cliprotocol.FeatureAgentRunInputs}}
	result := createTestServer(t, app).callTool(t, "sign_in", map[string]any{"pid": 81234, "window_id": 40512})
	if !result.IsError || getText(result) != "This Hisar app can't sign in to websites. Ask the user to update Hisar." {
		t.Errorf("old app: %+v", result.Content)
	}
	if types := app.listRequestTypes(); slices.Contains(types, cliprotocol.TypeAgentSignIn) {
		t.Errorf("sent %q to an app without %s", types, cliprotocol.FeatureAgentSignIn)
	}
	result = createTestServer(t, &fakeApp{answer: answerSignIn}).callTool(t, "sign_in", map[string]any{"pid": 0, "window_id": 40512})
	if !result.IsError || !strings.Contains(getText(result), "pid and window_id") {
		t.Errorf("no pid: %+v", result.Content)
	}
}
