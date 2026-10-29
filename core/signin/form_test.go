package signin

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

type expectedStep struct {
	origin   string
	username string
	password string
	code     []string
	submit   string
	reason   Reason
}

var expectedSteps = map[string]expectedStep{
	"plain":      {origin: fixtureMain, username: "username", password: "password", submit: "Sign in"},
	"controlled": {origin: fixtureMain, username: "username", password: "password", submit: "Sign in"},
	"wrong":      {origin: fixtureMain, username: "username", password: "password", submit: "Sign in"},
	"totp":       {origin: fixtureMain, username: "username", password: "password", submit: "Sign in"},
	"step1":      {origin: fixtureMain, username: "email", submit: "Next"},
	"spa-steps":  {origin: fixtureMain, username: "email", submit: "Next"},
	"step2":      {origin: fixtureMain, password: "password", submit: "Sign in"},
	"totp-code":  {origin: fixtureMain, code: []string{"otp"}, submit: "Verify"},
	"split-totp": {
		origin: fixtureMain, submit: "Verify",
		code: []string{"digit-1", "digit-2", "digit-3", "digit-4", "digit-5", "digit-6", "digit-7", "digit-8"},
	},
	// A transparent password field reads like a visible one; the field moved off the page does not count.
	"invisible":  {origin: fixtureMain, username: "username", password: "transparent", submit: "Sign in"},
	"phish":      {origin: fixtureOther, username: "username", password: "password", submit: "Sign in"},
	"return":     {origin: fixtureMain, username: "username", password: "password"},
	"iframe":     {reason: ReasonCrossOriginFrame},
	"srcdoc":     {reason: ReasonNotWebPage},
	"spoof":      {reason: ReasonNotSecureField},
	"signup":     {reason: ReasonSignUpForm},
	"newsletter": {reason: ReasonNoForm},
	"signed-in":  {reason: ReasonNoForm},
}

func identifiers(fields []Field) []string {
	names := []string{}
	for _, field := range fields {
		names = append(names, field.Identifier)
	}
	return names
}

func TestEveryRecordedPage(t *testing.T) {
	for _, browserID := range recordedBrowsers {
		for name, expected := range expectedSteps {
			page := readRecordedPage(t, browserID, name)
			step, origin, refusal := inspectPage(page)
			if expected.reason != "" {
				if refusal == nil || refusal.Reason != expected.reason {
					t.Errorf("%s/%s: %+v, want %s", browserID, name, refusal, expected.reason)
				}
				continue
			}
			if refusal != nil {
				t.Errorf("%s/%s: %v", browserID, name, refusal)
				continue
			}
			found := expectedStep{origin: origin.String(), code: identifiers(step.code)}
			if step.username != nil {
				found.username = step.username.Identifier
			}
			if step.password != nil {
				found.password = step.password.Identifier
			}
			if step.submit != nil {
				found.submit = step.submit.Title
			}
			if expected.code == nil {
				expected.code = []string{}
			}
			if found.origin != expected.origin || found.username != expected.username || found.password != expected.password ||
				found.submit != expected.submit || !slices.Equal(found.code, expected.code) {
				t.Errorf("%s/%s: %+v, want %+v", browserID, name, found, expected)
			}
		}
	}
}

func TestInspectNamesTheDestination(t *testing.T) {
	for _, browserID := range recordedBrowsers {
		browser := newFakeBrowser(t, browserID)
		browser.show("plain", "")
		destination, err := newTestSigner(browser, testNow).Inspect(context.Background(), testTarget)
		if err != nil {
			t.Fatal(err)
		}
		if destination.Origin.String() != fixtureMain || destination.Browser.ID != strings.TrimPrefix(browserID, "linux-") || destination.Target != testTarget ||
			destination.Process != (ProcessInstance{PID: 4242, StartTime: browser.process.StartTime}) {
			t.Errorf("%s: %+v", browserID, destination)
		}
		if len(browser.log()) != 0 {
			t.Errorf("%s: Inspect acted on the page: %v", browserID, browser.log())
		}
	}
}

func TestInspectRefusals(t *testing.T) {
	refuse := func(prepare func(browser *fakeBrowser)) *Refusal {
		browser := newFakeBrowser(t, "brave")
		browser.show("plain", "")
		prepare(browser)
		_, err := newTestSigner(browser, testNow).Inspect(context.Background(), testTarget)
		var refusal *Refusal
		if !errors.As(err, &refusal) {
			t.Fatalf("%v is not a refusal", err)
		}
		return refusal
	}
	for name, test := range map[string]struct {
		prepare func(browser *fakeBrowser)
		reason  Reason
		message string
	}{
		"no permission": {
			prepare: func(browser *fakeBrowser) { browser.observeErrors = []error{ErrNoAccess} }, reason: ReasonNoAccess,
			message: "Hisar can't fill browser windows yet: macOS hasn't given it the Accessibility permission. Ask the user to open Hisar → Settings → Developer tools → Coding agents → Website sign-in and allow it.",
		},
		"no window": {
			prepare: func(browser *fakeBrowser) { browser.observeErrors = []error{ErrWindowNotFound} }, reason: ReasonWindowNotFound,
			message: "Process 4242 has no window 7. Get the current pid and window_id from list_windows.",
		},
		"another Space": {
			prepare: func(browser *fakeBrowser) { browser.observeErrors = []error{ErrNotVisible} }, reason: ReasonNotVisible,
			message: "Window 7 is on another Space (or minimized), where Hisar can't read the page. Bring it to the current Space with focus_app or bring_to_front, then call sign_in again.",
		},
		"not a browser": {
			prepare: func(browser *fakeBrowser) {
				browser.process.Signature.Identifier, browser.process.Path = "com.apple.Preview", "/System/Applications/Preview.app/Contents/MacOS/Preview"
			},
			reason:  ReasonNotBrowser,
			message: "Window 7 belongs to Preview, not a browser Hisar signs in with. Hisar supports Brave Browser, Google Chrome and Microsoft Edge.",
		},
		"remote debugging": {
			prepare: func(browser *fakeBrowser) {
				browser.process.Arguments = append(browser.process.Arguments, "--remote-debugging-port=0")
			},
			reason:  ReasonDebuggable,
			message: "Brave Browser was started with remote debugging, so any program on this Mac could read what Hisar types into its pages. Hisar won't sign in there. Use a browser window that was started normally.",
		},
		"a blank page": {
			prepare: func(browser *fakeBrowser) { browser.page.Frames[0].URL = "about:blank" }, reason: ReasonNotWebPage,
			message: "The window shows about:blank, not a website. Open the site's sign-in page and call sign_in again.",
		},
		"plain http": {
			prepare: func(browser *fakeBrowser) { browser.show("plain", "http://example.com") }, reason: ReasonInsecure,
			message: "The page is http://example.com. Hisar signs in only on https pages.",
		},
		"a framed form": {
			prepare: func(browser *fakeBrowser) { browser.show("iframe", "") }, reason: ReasonCrossOriginFrame,
			message: "The sign-in form comes from http://localhost:47812 inside a page of http://127.0.0.1:47811. Open http://localhost:47812 itself and call sign_in again.",
		},
		"a disguised password field": {
			prepare: func(browser *fakeBrowser) { browser.show("spoof", "https://bank.example") }, reason: ReasonNotSecureField,
			message: "The password field on https://bank.example is not a real password input, so the password would show on screen. Hisar won't fill it.",
		},
		"no form": {
			prepare: func(browser *fakeBrowser) { browser.show("newsletter", "https://news.example") }, reason: ReasonNoForm,
			message: "Hisar found no username, password or one-time code field on https://news.example. Open the sign-in page, wait until its form shows, and call sign_in again.",
		},
	} {
		if refusal := refuse(test.prepare); refusal.Reason != test.reason || refusal.Message != test.message {
			t.Errorf("%s: %s %q", name, refusal.Reason, refusal.Message)
		}
	}
}

func TestSignInButtons(t *testing.T) {
	for title, isSignIn := range map[string]bool{
		"Sign in": true, "Log in": true, "LOGIN": true, "Next →": true, "Continue": true, "Verify": true,
		"Sign in to GitHub": true, "Sign in with Google": false, "Continue with Apple": false, "Sign in with a passkey": false,
		"Create account": false, "Subscribe": false, "Forgot password?": false, "Send code": false, "Search": false, "": false,
	} {
		if isSignInButton(Button{Title: title}) != isSignIn {
			t.Errorf("%q: sign-in %v", title, !isSignIn)
		}
	}
}
