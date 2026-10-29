package mcpserver

import (
	"context"
	"errors"
	"runtime"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tryhisar/hisar/core/cliprotocol"
)

// signInDescription is the sign_in tool's description for the system the agent runs on: the window it names and the
// window tools it uses differ between the Mac and Linux.
func signInDescription() string {
	window := "Pass the window's pid and window_id from list_windows (a macOS window ID). Use native computer_use window controls, not a CDP browser tool; Hisar refuses debuggable browsers. "
	works := "Works with Hisar for Mac, in a supported browser window on the current Space. "
	if runtime.GOOS == "linux" {
		window = "Pass the browser's pid and its X11 window ID as window_id (for example xdotool getactivewindow, then xdotool getwindowpid). " +
			"The window must be in front. Use native window controls, not a CDP browser tool; Hisar refuses debuggable browsers. "
		works = "Works with Hisar for Linux on X11 (or a browser under XWayland), in Chrome, Chromium, Brave or Edge started with ACCESSIBILITY_ENABLED=1 and " +
			"--force-renderer-accessibility. Hisar types what it enters into the focused field and stops if the focus moves. "
	}
	return "Sign in to the website open in a browser window. This tool returns no password or one-time code. Page scripts and captures can expose entered values. " +
		window +
		"Call it while the page shows the sign-in form, and do not type into the form first. Hisar reads " +
		"the page's address from the browser itself and finds the login saved for that website in the vaults available to agents. It asks the user to " +
		"approve unless a grant covers the login and origin. Then it enters the username, password and one-time code and submits each step (for example an email page, then a " +
		"password page, then a code page) while the page stays on that website. login is optional: the item ID from search_items, for when the user has " +
		"several accounts on the site. Returns outcome. submitted: Hisar submitted the last step; take a screenshot to see where the site went. " +
		"not-accepted: the site kept its sign-in form after Hisar submitted it; tell the user, and do not retry. " +
		"stopped: the page needs something Hisar doesn't fill, such as an account chooser, a CAPTCHA, a code sent by SMS or a page on another website; " +
		"handle it, then call sign_in again when a sign-in field shows. cleanup-failed: Hisar could not confirm it cleared what it entered from the page; " +
		"stop and tell the user. cleanup reports evidence, not a deletion guarantee: page-left (the document that held the fields is gone), " +
		"fields-cleared (Hisar cleared the fields and read back zero characters), or fields-removed (the fields left the accessibility tree, " +
		"but the document may retain the values). Hisar attempts cleanup; retained values and page-script copies have no enforced deletion limit. " +
		works + "Never type a password yourself, and never ask the user to paste one into the chat."
}

var errSignInNeedsNewerApp = errors.New("This Hisar app can't sign in to websites. Ask the user to update Hisar.")

// SignInInput names the window as cua-driver's list_windows does, so the model copies pid and window_id as it read them.
type SignInInput struct {
	PID      int    `json:"pid" jsonschema:"the process ID of the browser window, from list_windows"`
	WindowID uint32 `json:"window_id" jsonschema:"the window ID of the browser window: macOS from list_windows, X11 on Linux"`
	Login    string `json:"login,omitempty" jsonschema:"the item ID from search_items of the login to use, when the user has several accounts on the site"`
}

type SignInLogin struct {
	ID        string `json:"id"`
	VaultID   string `json:"vaultId"`
	VaultName string `json:"vaultName"`
	Title     string `json:"title"`
	Username  string `json:"username"`
}

type SignInStep struct {
	Filled    []string `json:"filled" jsonschema:"what Hisar entered on this page: username, password, one-time code"`
	Submitted bool     `json:"submitted"`
}

type SignInOutput struct {
	Outcome string       `json:"outcome" jsonschema:"submitted, not-accepted, stopped or cleanup-failed"`
	Cleanup string       `json:"cleanup" jsonschema:"cleanup evidence: nothing-entered, page-left, fields-cleared, fields-removed (the page may retain values), or unverified with cleanup-failed"`
	Website string       `json:"website" jsonschema:"the website Hisar signed in to, as it read it from the browser"`
	Browser string       `json:"browser"`
	Login   SignInLogin  `json:"login" jsonschema:"the login's metadata, including its username, as search_items shows; no password or one-time code"`
	Steps   []SignInStep `json:"steps" jsonschema:"each page of the sign-in Hisar filled"`
	Message string       `json:"message"`
}

func (server *toolServer) signIn(ctx context.Context, request *mcp.CallToolRequest, input SignInInput) (*mcp.CallToolResult, SignInOutput, error) {
	if input.PID <= 0 || input.WindowID == 0 {
		return nil, SignInOutput{}, errors.New("pass the pid and window_id of the browser window from list_windows")
	}
	appRequest := cliprotocol.Request{Type: cliprotocol.TypeAgentSignIn, Window: &cliprotocol.Window{PID: input.PID, WindowID: input.WindowID}, ItemID: input.Login}
	var result cliprotocol.AgentSignInResult
	if err := server.call(ctx, request, appRequest, &result); err != nil {
		return nil, SignInOutput{}, err
	}
	output := SignInOutput{
		Outcome: result.Outcome, Cleanup: result.Cleanup, Website: result.Website, Browser: result.Browser, Message: result.Message,
		Login: SignInLogin{ID: result.Login.ID, VaultID: result.Login.VaultID, VaultName: result.Login.VaultName, Title: result.Login.Title, Username: result.Login.Username},
		Steps: make([]SignInStep, 0, len(result.Steps)),
	}
	for _, step := range result.Steps {
		output.Steps = append(output.Steps, SignInStep{Filled: append([]string{}, step.Filled...), Submitted: step.IsSubmitted})
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: output.Message}}}, output, nil
}
