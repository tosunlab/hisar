package signin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/tryhisar/hisar/core/totp"
)

const (
	canaryPassword         = "HISAR-CANARY-7f3a9c2e41d8b605"
	fixtureOneTimePassword = "otpauth://totp/Fixture:user?secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP&digits=8"
)

var testNow = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

func loadLogin(login Login) func(context.Context) (Login, error) {
	return func(context.Context) (Login, error) { return login, nil }
}

func testLogin() Login {
	return Login{Username: "user@example.com", Password: NewSecret(canaryPassword), OneTimePassword: NewSecret(fixtureOneTimePassword)}
}

func expectedCode(t *testing.T) string {
	t.Helper()
	config, err := totp.Parse(fixtureOneTimePassword)
	if err != nil {
		t.Fatal(err)
	}
	code, _ := config.GenerateCode(testNow)
	return code
}

func signIn(t *testing.T, browser *fakeBrowser, login Login, changeDuringApproval func()) (Result, error) {
	t.Helper()
	return signInWith(t, context.Background(), browser, login, changeDuringApproval)
}

func signInWith(t *testing.T, ctx context.Context, browser *fakeBrowser, login Login, changeDuringApproval func()) (Result, error) {
	t.Helper()
	signer := newTestSigner(browser, testNow)
	destination, err := signer.Inspect(ctx, testTarget)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if changeDuringApproval != nil {
		changeDuringApproval()
	}
	result, err := signer.Run(ctx, destination, loadLogin(login))
	checkNoSecret(t, result, err, expectedCode(t))
	return result, err
}

func checkNoSecret(t *testing.T, result Result, err error, code string) {
	t.Helper()
	encoded, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	shown := fmt.Sprintf("%v %+v %#v %s", result, result, result, encoded)
	if err != nil {
		shown += err.Error() + fmt.Sprintf("%+v %#v", err, err)
	}
	for _, secret := range []string{canaryPassword, code} {
		if strings.Contains(shown, secret) {
			t.Fatalf("a secret reached the agent: %s", shown)
		}
	}
}

func checkRefusal(t *testing.T, err error, reason Reason) *Refusal {
	t.Helper()
	var refusal *Refusal
	if !errors.As(err, &refusal) || refusal.Reason != reason {
		t.Fatalf("err = %v, want a %s refusal", err, reason)
	}
	return refusal
}

func checkResult(t *testing.T, result Result, err error, outcome Outcome, cleanup Cleanup, steps ...Step) {
	t.Helper()
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	isSameSteps := slices.EqualFunc(result.Steps, steps, func(found, expected Step) bool {
		return found.IsSubmitted == expected.IsSubmitted && slices.Equal(found.Filled, expected.Filled)
	})
	if result.Outcome != outcome || result.Cleanup != cleanup || !isSameSteps {
		t.Fatalf("result = %+v, want %s, %s, %+v", result, outcome, cleanup, steps)
	}
}

func checkLog(t *testing.T, browser *fakeBrowser, expected ...string) {
	t.Helper()
	if found := browser.log(); !slices.Equal(found, expected) {
		t.Fatalf("actions = %q, want %q", found, expected)
	}
}

var (
	usernameAndPassword = Step{Filled: []Part{PartUsername, PartPassword}, IsSubmitted: true}
	usernameOnly        = Step{Filled: []Part{PartUsername}, IsSubmitted: true}
	passwordOnly        = Step{Filled: []Part{PartPassword}, IsSubmitted: true}
	codeOnly            = Step{Filled: []Part{PartCode}, IsSubmitted: true}
)

func TestSignInOnOnePage(t *testing.T) {
	for _, browserID := range recordedBrowsers {
		for _, page := range []string{"plain", "controlled"} {
			browser := newFakeBrowser(t, browserID)
			browser.show(page, "")
			browser.onSubmit = func(browser *fakeBrowser) {
				browser.show("signed-in", "")
				browser.prompts = []Prompt{savePasswordPrompt}
			}
			result, err := signIn(t, browser, testLogin(), nil)
			checkResult(t, result, err, Submitted, CleanupPageLeft, usernameAndPassword)
			checkLog(t, browser, "enter username", "enter password", "press Sign in", "dismiss prompt")
			if submits := cloneSubmits(browser); len(submits) != 1 || submits[0]["username"] != "user@example.com" || submits[0]["password"] != canaryPassword {
				t.Errorf("%s/%s: the site got %v", browserID, page, submits)
			}
		}
	}
}

func TestSignInOverTwoPages(t *testing.T) {
	browser := newFakeBrowser(t, "chrome")
	browser.show("step1", "")
	browser.onSubmit = func(browser *fakeBrowser) {
		if browser.page.Frames[0].URL == fixtureMain+"/step1" {
			browser.show("step2", "")
			return
		}
		browser.show("signed-in", "")
	}
	result, err := signIn(t, browser, testLogin(), nil)
	checkResult(t, result, err, Submitted, CleanupPageLeft, usernameOnly, passwordOnly)
	checkLog(t, browser, "enter email", "press Next", "enter password", "press Sign in")
}

func TestSignInWhereTheSamePageShowsTheNextStep(t *testing.T) {
	browser := newFakeBrowser(t, "edge")
	browser.show("spa-steps", "")
	browser.onSubmit = func(browser *fakeBrowser) {
		frame := browser.page.Frames[0]
		next := "step2"
		if browser.page.Fields[0].Identifier == "password" {
			next = "signed-in"
		}
		browser.show(next, "")
		browser.page.Frames[0] = frame
	}
	result, err := signIn(t, browser, testLogin(), nil)
	checkResult(t, result, err, Submitted, CleanupFieldsRemoved, usernameOnly, passwordOnly)
}

func TestSignInWithAOneTimeCode(t *testing.T) {
	browser := newFakeBrowser(t, "brave")
	browser.show("totp", "")
	browser.onSubmit = func(browser *fakeBrowser) {
		if browser.page.Frames[0].URL == fixtureMain+"/totp" {
			browser.show("totp-code", "")
			return
		}
		browser.show("signed-in", "")
	}
	result, err := signIn(t, browser, testLogin(), nil)
	checkResult(t, result, err, Submitted, CleanupPageLeft, usernameAndPassword, codeOnly)
	if submits := cloneSubmits(browser); len(submits) != 2 || submits[1]["otp"] != expectedCode(t) {
		t.Errorf("the site got %v, want the code %s", submits, expectedCode(t))
	}
}

func TestSignInWithACodeSplitIntoDigits(t *testing.T) {
	browser := newFakeBrowser(t, "brave")
	browser.show("split-totp", "")
	browser.onSubmit = func(browser *fakeBrowser) { browser.show("signed-in", "") }
	result, err := signIn(t, browser, testLogin(), nil)
	checkResult(t, result, err, Submitted, CleanupPageLeft, codeOnly)
	submitted := cloneSubmits(browser)[0]
	code := ""
	for digit := 1; digit <= 8; digit++ {
		code += submitted[fmt.Sprintf("digit-%d", digit)]
	}
	if code != expectedCode(t) {
		t.Errorf("the digits are %q, want %q", code, expectedCode(t))
	}
}

func TestSignInEntersACodeFieldThatShowsBelowThePassword(t *testing.T) {
	browser := newFakeBrowser(t, "brave")
	browser.show("plain", "")
	browser.onSubmit = func(browser *fakeBrowser) {
		if len(browser.submits) == 1 {
			code := readRecordedPage(t, "brave", "totp-code").Fields[0]
			code.ID = 99
			browser.page.Fields = append(browser.page.Fields, code)
			return
		}
		browser.show("signed-in", "")
	}
	result, err := signIn(t, browser, testLogin(), nil)
	checkResult(t, result, err, Submitted, CleanupPageLeft, usernameAndPassword, codeOnly)
	checkLog(t, browser, "enter username", "enter password", "press Sign in", "enter otp", "press Sign in")
}

func TestWrongPasswordOnAPageThatKeepsIt(t *testing.T) {
	browser := newFakeBrowser(t, "brave")
	browser.show("wrong", "")
	result, err := signIn(t, browser, testLogin(), nil)
	checkResult(t, result, err, NotAccepted, CleanupFieldsCleared, usernameAndPassword)
	log := browser.log()
	entries := slices.DeleteFunc(slices.Clone(log), func(action string) bool { return action != "enter password" })
	if len(entries) != 1 || !slices.Contains(log, "clear password") {
		t.Errorf("actions = %q, want one password entry and its clearing", log)
	}
	if browser.fieldValue("password") != "" || browser.fieldValue("username") != "" {
		t.Error("the entries are still on the page")
	}
}

func TestWrongPasswordOnAPageThatReloads(t *testing.T) {
	browser := newFakeBrowser(t, "brave")
	browser.show("plain", "")
	browser.onSubmit = func(browser *fakeBrowser) { browser.show("plain", "") }
	result, err := signIn(t, browser, testLogin(), nil)
	checkResult(t, result, err, NotAccepted, CleanupFieldsCleared, usernameAndPassword)
	checkLog(t, browser, "enter username", "enter password", "press Sign in", "clear username", "clear password")
}

func TestTheWindowChangedDuringTheApproval(t *testing.T) {
	for name, change := range map[string]func(browser *fakeBrowser){
		"another origin":      func(browser *fakeBrowser) { browser.show("phish", "") },
		"a restarted browser": func(browser *fakeBrowser) { browser.process.StartTime++ },
		"remote debugging": func(browser *fakeBrowser) {
			browser.process.Arguments = append(browser.process.Arguments, "--remote-debugging-pipe")
		},
		"the form left":        func(browser *fakeBrowser) { browser.show("signed-in", "") },
		"another browser":      func(browser *fakeBrowser) { browser.process.Signature.Identifier = "com.google.Chrome" },
		"the window is closed": func(browser *fakeBrowser) { browser.observeErrors = []error{ErrWindowNotFound} },
	} {
		browser := newFakeBrowser(t, "brave")
		browser.show("plain", "")
		_, err := signIn(t, browser, testLogin(), func() { change(browser) })
		refusal := checkRefusal(t, err, ReasonChanged)
		if !strings.HasPrefix(refusal.Message, "The window changed while the user was approving: it now shows ") {
			t.Errorf("%s: %q", name, refusal.Message)
		}
		checkLog(t, browser)
	}
}

func TestTheNextStepOnAnotherOriginStops(t *testing.T) {
	browser := newFakeBrowser(t, "brave")
	browser.show("step1", "")
	browser.onSubmit = func(browser *fakeBrowser) { browser.show("phish", "") }
	result, err := signIn(t, browser, testLogin(), nil)
	checkResult(t, result, err, Stopped, CleanupPageLeft, usernameOnly)
	if result.Detail != "the page moved to "+fixtureOther {
		t.Errorf("detail = %q", result.Detail)
	}
	checkLog(t, browser, "enter email", "press Next")
}

func TestPagesHisarDoesNotFillStopTheSignIn(t *testing.T) {
	for name, test := range map[string]struct {
		first, next string
		login       Login
		steps       []Step
		detail      string
	}{
		"an account chooser after the email": {first: "step1", next: "signed-in", login: testLogin(), steps: []Step{usernameOnly}, detail: "something Hisar doesn't fill"},
		"a code the login lacks":             {first: "totp", next: "totp-code", login: Login{Username: "u", Password: NewSecret(canaryPassword)}, steps: []Step{usernameAndPassword}, detail: "a code Hisar doesn't have, such as one sent by SMS or email"},
		"a sign-up form":                     {first: "step1", next: "signup", login: testLogin(), steps: []Step{usernameOnly}, detail: "a form Hisar doesn't fill"},
	} {
		browser := newFakeBrowser(t, "brave")
		browser.show(test.first, "")
		browser.onSubmit = func(browser *fakeBrowser) { browser.show(test.next, "") }
		result, err := signIn(t, browser, test.login, nil)
		checkResult(t, result, err, Stopped, CleanupPageLeft, test.steps...)
		if result.Detail != test.detail {
			t.Errorf("%s: detail = %q", name, result.Detail)
		}
	}
}

func TestACodePageForALoginWithoutACode(t *testing.T) {
	browser := newFakeBrowser(t, "brave")
	browser.show("totp-code", "")
	_, err := signIn(t, browser, Login{Username: "u", Password: NewSecret(canaryPassword)}, nil)
	refusal := checkRefusal(t, err, ReasonNoForm)
	if refusal.Message != "The page on http://127.0.0.1:47811 asks for a code Hisar doesn't have, such as one sent by SMS or email, which Hisar doesn't fill with this login. Handle it, then call sign_in again if a username, password or one-time code field shows." {
		t.Errorf("message = %q", refusal.Message)
	}
	checkLog(t, browser)
}

func TestInterruptedBeforeTheSubmit(t *testing.T) {
	browser := newFakeBrowser(t, "brave")
	browser.show("plain", "")
	ctx, cancel := context.WithCancel(context.Background())
	browser.onEnter = func(identifier string) {
		if identifier == "password" {
			cancel()
		}
	}
	_, err := signInWith(t, ctx, browser, testLogin(), nil)
	checkRefusal(t, err, ReasonInterrupted)
	log := browser.log()
	if slices.ContainsFunc(log, func(action string) bool { return strings.HasPrefix(action, "press") }) ||
		!slices.Contains(log, "clear password") || !slices.Contains(log, "clear username") {
		t.Errorf("actions = %q", log)
	}
	if browser.fieldValue("password") != "" {
		t.Error("the password is still on the page")
	}
}

func TestReadBack(t *testing.T) {
	browser := newFakeBrowser(t, "brave")
	browser.show("plain", "")
	browser.readBack = func(field Field, value string) int {
		if field.IsSecure && value != "" {
			return len(value) + 1
		}
		return len(value)
	}
	_, err := signIn(t, browser, testLogin(), nil)
	checkRefusal(t, err, ReasonEntryFailed)
	checkLog(t, browser, "enter username", "enter password", "clear username", "clear password")

	hidden := newFakeBrowser(t, "brave")
	hidden.show("plain", "")
	hidden.readBack = func(Field, string) int { return 0 }
	hidden.onSubmit = func(browser *fakeBrowser) { browser.show("signed-in", "") }
	result, err := signIn(t, hidden, testLogin(), nil)
	checkResult(t, result, err, Submitted, CleanupPageLeft, usernameAndPassword)
}

func TestTheFieldChangesBeforeTheEntry(t *testing.T) {
	browser := newFakeBrowser(t, "brave")
	browser.show("plain", "")
	browser.onEnter = func(identifier string) {
		if identifier == "username" {
			defer browser.lock()()
			browser.page.Frames[0].URL = fixtureOther + "/plain"
		}
	}
	_, err := signIn(t, browser, testLogin(), nil)
	checkRefusal(t, err, ReasonEntryFailed)
	checkLog(t, browser, "enter username", "clear username", "clear password")
}

func TestCleanupThatCannotBeVerified(t *testing.T) {
	for name, prepare := range map[string]func(browser *fakeBrowser){
		"the save prompt stays open": func(browser *fakeBrowser) {
			browser.isPromptPermanent = true
			browser.onSubmit = func(browser *fakeBrowser) {
				browser.show("signed-in", "")
				browser.prompts = []Prompt{savePasswordPrompt}
			}
		},
		"the save prompt has no close control Hisar knows": func(browser *fakeBrowser) {
			browser.onSubmit = func(browser *fakeBrowser) {
				browser.show("signed-in", "")
				prompt := savePasswordPrompt
				prompt.HasClose = false
				browser.prompts = []Prompt{prompt}
			}
		},
		"the browser stops answering for its prompts": func(browser *fakeBrowser) {
			browser.show("wrong", "")
			browser.onSubmit = func(browser *fakeBrowser) {
				browser.onListPrompts = func() error { return ErrUnreadable }
			}
		},
		"the browser changes while Hisar watches for the prompt": func(browser *fakeBrowser) {
			browser.show("wrong", "")
			browser.onSubmit = func(browser *fakeBrowser) {
				browser.onListPrompts = func() error { return ErrChanged }
			}
		},
		"clearing fails": func(browser *fakeBrowser) {
			browser.show("wrong", "")
			browser.failClear = "username"
		},
		"the window leaves the Space": func(browser *fakeBrowser) {
			browser.show("wrong", "")
			browser.onSubmit = func(browser *fakeBrowser) {
				for range 500 {
					browser.observeErrors = append(browser.observeErrors, ErrNotVisible)
				}
			}
		},
		"the window cannot be found again": func(browser *fakeBrowser) {
			browser.show("wrong", "")
			browser.onSubmit = func(browser *fakeBrowser) {
				for range 5000 {
					browser.observeErrors = append(browser.observeErrors, ErrWindowNotFound)
				}
			}
		},
		"the page removes the fields before the submit": func(browser *fakeBrowser) {
			browser.onEnter = func(identifier string) {
				if identifier == "password" {
					defer browser.lock()()
					browser.page.Fields = nil
				}
			}
		},
	} {
		browser := newFakeBrowser(t, "brave")
		browser.show("plain", "")
		prepare(browser)
		result, err := signIn(t, browser, testLogin(), nil)
		if err != nil || result.Outcome != CleanupFailed || result.Cleanup != CleanupUnverified {
			t.Errorf("%s: %+v, %v", name, result, err)
		}
		if name == "clearing fails" && browser.fieldValue("password") != "" {
			t.Error("a field that failed to clear stopped the clearing of the password")
		}
	}
}

func TestTheWindowClosesAfterTheSubmit(t *testing.T) {
	browser := newFakeBrowser(t, "brave")
	browser.show("plain", "")
	browser.onSubmit = func(browser *fakeBrowser) {
		for range 500 {
			browser.observeErrors = append(browser.observeErrors, ErrWindowClosed)
		}
		browser.prompts = []Prompt{savePasswordPrompt}
	}
	result, err := signIn(t, browser, testLogin(), nil)
	checkResult(t, result, err, Submitted, CleanupPageLeft, usernameAndPassword)
	if !slices.Contains(browser.log(), "dismiss prompt") {
		t.Errorf("actions = %q: the save prompt the closed window left was not dismissed", browser.log())
	}
}

func TestAFailedWindowLookupIsNotAClosedWindow(t *testing.T) {
	browser := newFakeBrowser(t, "brave")
	browser.show("wrong", "")
	browser.onSubmit = func(browser *fakeBrowser) { browser.observeErrors = []error{ErrWindowNotFound} }
	result, err := signIn(t, browser, testLogin(), nil)
	checkResult(t, result, err, NotAccepted, CleanupFieldsCleared, usernameAndPassword)
	if browser.fieldValue("password") != "" || browser.fieldValue("username") != "" {
		t.Error("the entries are still on the page")
	}
}

func TestAFieldThePageHidesIsNotPageLeft(t *testing.T) {
	browser := newFakeBrowser(t, "brave")
	browser.show("plain", "")
	browser.onSubmit = func(browser *fakeBrowser) {
		// The document and the values in it stay; only the accessibility tree loses the form, as with display: none.
		browser.page.Fields = nil
		browser.page.Buttons = nil
	}
	result, err := signIn(t, browser, testLogin(), nil)
	checkResult(t, result, err, Submitted, CleanupFieldsRemoved, usernameAndPassword)
}

func TestAPasswordFieldUnderANewElementIsCleared(t *testing.T) {
	browser := newFakeBrowser(t, "brave")
	browser.show("wrong", "")
	browser.onSubmit = func(browser *fakeBrowser) {
		for index, field := range browser.page.Fields {
			if field.IsSecure {
				browser.nextID++
				rebuilt := ElementID(browser.nextID)
				browser.page.Fields[index].ID = rebuilt
				browser.values[rebuilt] = browser.values[field.ID]
			}
		}
	}
	result, err := signIn(t, browser, testLogin(), nil)
	checkResult(t, result, err, NotAccepted, CleanupFieldsRemoved, usernameAndPassword)
	if browser.fieldValue("password") != "" {
		t.Error("the password is still in the field the browser rebuilt")
	}
}

func TestALookalikeFieldIsNoEvidenceTheEntryWasCleared(t *testing.T) {
	browser := newFakeBrowser(t, "brave")
	browser.show("plain", "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var hidden ElementID
	browser.onSubmit = func(browser *fakeBrowser) {
		// The page hides the password, which keeps its value, and shows another input it describes the same way.
		for index, field := range browser.page.Fields {
			if field.IsSecure {
				hidden = field.ID
				browser.nextID++
				lookalike := ElementID(browser.nextID)
				browser.page.Fields[index].ID = lookalike
				browser.values[lookalike] = "an unrelated value"
			}
		}
		cancel()
	}
	result, err := signInWith(t, ctx, browser, testLogin(), nil)
	if browser.values[hidden] != canaryPassword {
		t.Fatal("the hidden password field lost its value")
	}
	checkResult(t, result, err, CleanupFailed, CleanupUnverified, usernameAndPassword)
	if browser.fieldValue("password") != "" {
		t.Error("the lookalike field was not cleared")
	}
}

func TestALandingOnAnotherOriginStops(t *testing.T) {
	browser := newFakeBrowser(t, "brave")
	browser.show("plain", "")
	browser.onSubmit = func(browser *fakeBrowser) { browser.show("signed-in", fixtureOther) }
	result, err := signIn(t, browser, testLogin(), nil)
	checkResult(t, result, err, Stopped, CleanupPageLeft, usernameAndPassword)
	if result.Detail != "the page moved to "+fixtureOther {
		t.Errorf("detail = %q", result.Detail)
	}
}

func TestCancellingDuringThePromptWatchStillCleansUp(t *testing.T) {
	for _, isPromptPermanent := range []bool{false, true} {
		browser := newFakeBrowser(t, "brave")
		browser.show("plain", "")
		browser.isPromptPermanent = isPromptPermanent
		ctx, cancel := context.WithCancel(context.Background())
		browser.onSubmit = func(browser *fakeBrowser) {
			browser.show("signed-in", "")
			browser.prompts = []Prompt{savePasswordPrompt}
			browser.onListPrompts = func() error {
				cancel()
				return nil
			}
		}
		result, err := signInWith(t, ctx, browser, testLogin(), nil)
		if isPromptPermanent {
			if err != nil || result.Outcome != CleanupFailed || result.Cleanup != CleanupUnverified {
				t.Errorf("a prompt that stays open after the cancellation: %+v, %v", result, err)
			}
			continue
		}
		checkRefusal(t, err, ReasonInterrupted)
		if !slices.Contains(browser.log(), "dismiss prompt") {
			t.Errorf("actions = %q: the cancellation stopped the prompt's dismissal", browser.log())
		}
	}
}

func TestAHungBrowserCannotHoldTheSignIn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		browser := newFakeBrowser(t, "brave")
		browser.show("wrong", "")
		browser.onSubmit = func(browser *fakeBrowser) { browser.isHung = true }
		signer := newTestSigner(browser, testNow)
		destination, err := signer.Inspect(context.Background(), testTarget)
		if err != nil {
			t.Fatal(err)
		}
		started := time.Now()
		result, err := signer.Run(context.Background(), destination, loadLogin(testLogin()))
		if err != nil || result.Outcome != CleanupFailed || result.Cleanup != CleanupUnverified {
			t.Errorf("a browser that stopped answering: %+v, %v", result, err)
		}
		if elapsed, bound := time.Since(started), testTiming.await+hungObserveBound+testTiming.cleanup; elapsed > bound {
			t.Errorf("the sign-in took %v, more than the %v its bounds allow", elapsed, bound)
		}
		_, err = signer.Run(context.Background(), destination, loadLogin(testLogin()))
		checkRefusal(t, err, ReasonUnreadable)
	})
}

func TestTheWindowLeavesTheSpaceForAMoment(t *testing.T) {
	browser := newFakeBrowser(t, "brave")
	browser.show("plain", "")
	browser.onSubmit = func(browser *fakeBrowser) {
		browser.observeErrors = []error{ErrNotVisible, ErrNotVisible, ErrNotVisible}
		browser.show("signed-in", "")
	}
	result, err := signIn(t, browser, testLogin(), nil)
	checkResult(t, result, err, Submitted, CleanupPageLeft, usernameAndPassword)
}

func TestOneSignInAtATime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		browser := newFakeBrowser(t, "brave")
		browser.show("wrong", "")
		signer := newTestSigner(browser, testNow)
		destination, err := signer.Inspect(context.Background(), testTarget)
		if err != nil {
			t.Fatal(err)
		}
		release := make(chan struct{})
		firstDone := make(chan struct{})
		go func() {
			defer close(firstDone)
			signer.Run(context.Background(), destination, func(context.Context) (Login, error) {
				<-release
				return testLogin(), nil
			})
		}()
		synctest.Wait()
		isSecondLoaded := false
		secondDone := make(chan struct{})
		go func() {
			defer close(secondDone)
			signer.Run(context.Background(), destination, func(context.Context) (Login, error) {
				isSecondLoaded = true
				return testLogin(), nil
			})
		}()
		synctest.Wait()
		if isSecondLoaded {
			t.Fatal("the second sign-in read its login while the first one ran")
		}
		queued, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := signer.Run(queued, destination, loadLogin(testLogin())); err == nil {
			t.Error("a sign-in whose request ended while it waited for its turn ran")
		} else {
			checkRefusal(t, err, ReasonInterrupted)
		}
		close(release)
		<-firstDone
		<-secondDone
		if !isSecondLoaded {
			t.Error("the second sign-in never ran")
		}
	})
}

func TestSignInWithReturnWhereTheFormHasNoButton(t *testing.T) {
	browser := newFakeBrowser(t, "chrome")
	browser.show("return", "")
	browser.onSubmit = func(browser *fakeBrowser) { browser.show("signed-in", "") }
	result, err := signIn(t, browser, testLogin(), nil)
	checkResult(t, result, err, Submitted, CleanupPageLeft, usernameAndPassword)
	checkLog(t, browser, "enter username", "enter password", "return password")
}

func TestTheLoginIsReadOnlyForAWindowThatStillMatches(t *testing.T) {
	browser := newFakeBrowser(t, "brave")
	browser.show("plain", "")
	signer := newTestSigner(browser, testNow)
	destination, err := signer.Inspect(context.Background(), testTarget)
	if err != nil {
		t.Fatal(err)
	}
	isLoaded := false
	load := func(context.Context) (Login, error) {
		isLoaded = true
		return testLogin(), nil
	}
	browser.show("phish", "")
	_, err = signer.Run(context.Background(), destination, load)
	checkRefusal(t, err, ReasonChanged)
	if isLoaded {
		t.Error("the login was read for a window that changed")
	}

	browser.show("plain", "")
	destination, err = signer.Inspect(context.Background(), testTarget)
	if err != nil {
		t.Fatal(err)
	}
	changed := &Refusal{Reason: ReasonChanged, Message: "The login changed while the user was approving."}
	_, err = signer.Run(context.Background(), destination, func(context.Context) (Login, error) { return Login{}, changed })
	if err != changed {
		t.Errorf("err = %v, want load's refusal", err)
	}
	checkLog(t, browser)
}
