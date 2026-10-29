package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/tryhisar/hisar/core/agentharness"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/signin"
)

type fakeSignInDriver struct {
	mutex          sync.Mutex
	hasAccess      bool
	requestedCount int
}

func (driver *fakeSignInDriver) HasAccess() bool {
	driver.mutex.Lock()
	defer driver.mutex.Unlock()
	return driver.hasAccess
}

func (*fakeSignInDriver) System() signin.System { return signin.SystemMac }

func (driver *fakeSignInDriver) RequestAccess() {
	driver.mutex.Lock()
	defer driver.mutex.Unlock()
	driver.requestedCount++
}

func (driver *fakeSignInDriver) Observe(context.Context, signin.Target) (signin.Snapshot, error) {
	return nil, signin.ErrWindowNotFound
}

func (*fakeSignInDriver) ListPrompts(context.Context, signin.ProcessInstance) ([]signin.Prompt, error) {
	return nil, signin.ErrChanged
}

func (*fakeSignInDriver) DismissPrompt(context.Context, signin.ProcessInstance, signin.ElementID) error {
	return signin.ErrChanged
}

func mustParseOrigin(t *testing.T, rawURL string) signin.Origin {
	t.Helper()
	origin, err := signin.ParseOrigin(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return origin
}

func getRefusal(err error) string {
	if responseError, isResponse := err.(*cliprotocol.ResponseError); isResponse && responseError.Code == cliprotocol.ErrorSignInRefused {
		return responseError.Message
	}
	return ""
}

func TestAgentSignInSettingIsOffByDefaultAndAsksForAccessibility(t *testing.T) {
	testService := createUnlockedTestService(t)
	service := testService.service
	if settings := service.GetAgentSignIn(); settings.IsAvailable || settings.IsEnabled {
		t.Errorf("without a driver: %+v", settings)
	}
	if _, err := service.SetAgentSignInEnabled(true); err == nil {
		t.Error("turned on without a driver")
	}

	driver := &fakeSignInDriver{}
	service.config.SignInDriver = driver
	if settings := service.GetAgentSignIn(); !settings.IsAvailable || settings.IsEnabled || settings.HasAccessibility {
		t.Errorf("by default: %+v", settings)
	}
	settings, err := service.SetAgentSignInEnabled(true)
	if err != nil || !settings.IsEnabled || driver.requestedCount != 1 {
		t.Fatalf("turn on: %+v %v, %d access requests", settings, err, driver.requestedCount)
	}
	if state, err := localstate.Load(service.config.StatePath); err != nil || !state.IsAgentSignInEnabled {
		t.Errorf("saved %v %v", state.IsAgentSignInEnabled, err)
	}
	driver.hasAccess = true
	service.SetAgentSignInEnabled(true)
	if driver.requestedCount != 1 {
		t.Error("asked for Accessibility again while Hisar has it")
	}
	service.Lock()
	if _, err := service.SetAgentSignInEnabled(false); err == nil {
		t.Error("turned off while locked")
	}
}

func TestAgentSignInRefusesUntilItIsOnAndAllowed(t *testing.T) {
	testService := createUnlockedTestService(t)
	service := testService.service
	bridge := NewCommandLineBridge(service, clisocket.NewSessionApprovals(0, nil, nil))
	request := &cliprotocol.Request{Type: cliprotocol.TypeAgentSignIn, Window: &cliprotocol.Window{PID: 1, WindowID: 2}}
	if _, err := bridge.PrepareAgentSignIn(bridge.BindSession(context.Background()), request); getRefusal(err) != "Signing in to websites works only in Hisar for Mac and Linux." {
		t.Errorf("without a driver: %v", err)
	}

	driver := &fakeSignInDriver{}
	service.config.SignInDriver = driver
	service.signer = signin.NewSigner(driver, t.TempDir(), service.config.Now)
	if _, err := bridge.PrepareAgentSignIn(bridge.BindSession(context.Background()), request); !strings.HasPrefix(getRefusal(err), "Website sign-in is off in Hisar.") {
		t.Errorf("switched off: %v", err)
	}
	service.SetAgentSignInEnabled(true)
	noAccess := service.signer.RefuseNoAccess().Message
	if _, err := bridge.PrepareAgentSignIn(bridge.BindSession(context.Background()), request); getRefusal(err) != noAccess {
		t.Errorf("without Accessibility: %v", err)
	}
	if _, err := bridge.PerformAgentSignIn(bridge.BindSession(context.Background()), clisocket.AgentSignInTarget{}, clisocket.AgentLogin{}, clisocket.AgentAuthorization{}); getRefusal(err) != noAccess {
		t.Errorf("perform without Accessibility: %v", err)
	}
}

type signInLogins struct {
	tosunlab, labStaging, github, familyLab ItemDetail
}

func saveSignInLogins(t *testing.T, testService unlockedTestService) signInLogins {
	t.Helper()
	service := testService.service
	save := func(vaultID string, detail ItemDetail) ItemDetail {
		saved, err := service.SaveItem(vaultID, detail)
		if err != nil {
			t.Fatal(err)
		}
		return saved
	}
	logins := signInLogins{
		tosunlab:   save(testService.personalID, ItemDetail{Title: "Tosun Lab", Username: "user@example.com", Password: "tosunlab-password", URLs: []string{"https://tosunlab.com/sign-in"}}),
		labStaging: save(testService.personalID, ItemDetail{Title: "Tosun Lab staging", Username: "ops@example.com", Password: "staging-password", URLs: []string{"staging.tosunlab.com"}}),
		github:     save(testService.personalID, ItemDetail{Title: "GitHub", Username: "developer", Password: "github-password", URLs: []string{"https://github.com"}}),
		familyLab:  save(testService.familyID, ItemDetail{Title: "Family Tosun Lab", Username: "user@example.com", Password: "family-password", URLs: []string{"https://tosunlab.com"}}),
	}
	save(testService.personalID, ItemDetail{Title: "Tosun Lab note", Category: "secure-note", Notes: "https://tosunlab.com"})
	if _, err := service.SetVaultAvailableToAgents(testService.personalID, true); err != nil {
		t.Fatal(err)
	}
	return logins
}

func listAgentLoginTitles(t *testing.T, service *Service, website, itemID string) ([]string, error) {
	t.Helper()
	service.mutex.Lock()
	defer service.mutex.Unlock()
	logins, err := service.listAgentLogins(context.Background(), mustParseOrigin(t, website), itemID)
	titles := make([]string, 0, len(logins))
	for _, login := range logins {
		if login.Origin != mustParseOrigin(t, website).String() {
			t.Errorf("%s is bound to %q", login.ItemTitle, login.Origin)
		}
		titles = append(titles, login.ItemTitle+" "+login.Username)
	}
	return titles, err
}

func TestAgentSignInOffersOnlyAgentVaultLoginsSavedForTheOrigin(t *testing.T) {
	testService := createUnlockedTestService(t)
	service := testService.service
	logins := saveSignInLogins(t, testService)

	if titles, err := listAgentLoginTitles(t, service, "https://tosunlab.com", ""); err != nil || strings.Join(titles, ", ") != "Tosun Lab user@example.com, Tosun Lab staging ops@example.com" {
		t.Errorf("logins for tosunlab.com: %q %v", titles, err)
	}
	service.mutex.Lock()
	service.state.IsExactHostMatch = true
	service.mutex.Unlock()
	if titles, err := listAgentLoginTitles(t, service, "https://tosunlab.com", ""); err != nil || strings.Join(titles, ", ") != "Tosun Lab user@example.com" {
		t.Errorf("logins for tosunlab.com with exact hosts: %q %v", titles, err)
	}
	if titles, err := listAgentLoginTitles(t, service, "https://tosunlab.com", logins.tosunlab.ID); err != nil || len(titles) != 1 {
		t.Errorf("the named login: %q %v", titles, err)
	}

	for _, testCase := range []struct {
		website, itemID string
		reason          signin.Reason
		refusal         string
	}{
		{"https://tosunlab-login.example.net", "", signin.ReasonNoLogin, "No login in the vaults available to agents is saved for https://tosunlab-login.example.net. Ask the user to add one in Hisar or make its vault available to agents. Don't ask for the password."},
		{"http://tosunlab.com", "", signin.ReasonNoLogin, "No login in the vaults available to agents is saved for http://tosunlab.com."},
		{"https://tosunlab.com", logins.familyLab.ID, signin.ReasonNoLogin, "No login available to agents has the ID"},
		{"https://tosunlab.com", logins.github.ID, signin.ReasonLoginNotForSite, "“GitHub” is saved for https://github.com, but the page is https://tosunlab.com. Hisar only signs in on a login's saved websites."},
	} {
		_, err := listAgentLoginTitles(t, service, testCase.website, testCase.itemID)
		var refusal *signin.Refusal
		if !errors.As(err, &refusal) || refusal.Reason != testCase.reason || !strings.HasPrefix(getRefusal(convertSignInError(err)), testCase.refusal) {
			t.Errorf("%s %s: %v", testCase.website, testCase.itemID, err)
		}
	}
}

func TestAgentSignInRefusesALoginChangedDuringTheApproval(t *testing.T) {
	for name, change := range map[string]func(service *Service, testService unlockedTestService, detail ItemDetail){
		"a new revision": func(service *Service, testService unlockedTestService, detail ItemDetail) {
			detail.Notes = "edited"
			service.SaveItem(testService.personalID, detail)
		},
		"another website": func(service *Service, testService unlockedTestService, detail ItemDetail) {
			detail.URLs = []string{"https://tosunlab-login.example.net"}
			service.SaveItem(testService.personalID, detail)
		},
		"a vault hidden from agents": func(service *Service, testService unlockedTestService, _ ItemDetail) {
			service.SetVaultAvailableToAgents(testService.personalID, false)
		},
	} {
		testService := createUnlockedTestService(t)
		service := testService.service
		window := &signInWindow{website: "https://tosunlab.com"}
		test := startAgentSignIn(t, testService, window)
		test.approver.onApprove = func() {
			detail, err := service.GetItem(testService.personalID, test.logins.tosunlab.ID)
			if err != nil {
				t.Fatal(err)
			}
			change(service, testService, detail)
		}
		if _, failure := test.signIn(t, test.logins.tosunlab.ID); !strings.HasPrefix(failure, "sign-in-refused: “Tosun Lab” changed while the user was approving") {
			t.Errorf("%s: %s", name, failure)
		}
		if len(window.posted) != 0 || len(window.values) != 0 {
			t.Errorf("%s: the page got %v", name, window.posted)
		}
	}
}

func TestAgentSignInResultNamesWhatHisarDidAndNoValue(t *testing.T) {
	destination := signin.Destination{Browser: signin.Browser{Name: "Google Chrome"}, Origin: mustParseOrigin(t, "https://tosunlab.com")}
	login := clisocket.AgentLogin{AgentSecret: clisocket.AgentSecret{VaultID: "v", ItemID: "i", VaultName: "Agents", ItemTitle: "Tosun Lab"}, Username: "user@example.com"}
	usernameThenPassword := []signin.Step{{Filled: []signin.Part{signin.PartUsername}, IsSubmitted: true}, {Filled: []signin.Part{signin.PartPassword}, IsSubmitted: true}}
	usernameAndPassword := []signin.Step{{Filled: []signin.Part{signin.PartUsername, signin.PartPassword}, IsSubmitted: true}}
	for _, testCase := range []struct {
		result           signin.Result
		outcome, cleanup string
		message          string
	}{
		{signin.Result{Outcome: signin.Submitted, Cleanup: signin.CleanupPageLeft, Steps: usernameThenPassword}, cliprotocol.SignInSubmitted, cliprotocol.SignInCleanupPageLeft,
			"Hisar signed in to https://tosunlab.com as user@example.com and submitted it. Take a screenshot to see the page."},
		{signin.Result{Outcome: signin.Submitted, Cleanup: signin.CleanupFieldsRemoved, Steps: usernameThenPassword}, cliprotocol.SignInSubmitted, cliprotocol.SignInCleanupFieldsRemoved,
			"Hisar signed in to https://tosunlab.com as user@example.com and submitted it. Take a screenshot to see the page."},
		{signin.Result{Outcome: signin.NotAccepted, Cleanup: signin.CleanupFieldsCleared, Steps: usernameAndPassword}, cliprotocol.SignInNotAccepted, cliprotocol.SignInCleanupFieldsCleared,
			"The site kept its sign-in form after Hisar submitted user@example.com's username and password, so it may be wrong or the site refused it. Hisar cleared what it entered. Tell the user; do not retry."},
		{signin.Result{Outcome: signin.NotAccepted, Cleanup: signin.CleanupFieldsRemoved, Steps: usernameAndPassword}, cliprotocol.SignInNotAccepted, cliprotocol.SignInCleanupFieldsRemoved,
			"The site kept its sign-in form after Hisar submitted user@example.com's username and password, so it may be wrong or the site refused it. The page removed the fields Hisar filled itself. Tell the user; do not retry."},
		{signin.Result{Outcome: signin.NotAccepted, Cleanup: signin.CleanupPageLeft, Steps: usernameAndPassword}, cliprotocol.SignInNotAccepted, cliprotocol.SignInCleanupPageLeft,
			"The site kept its sign-in form after Hisar submitted user@example.com's username and password, so it may be wrong or the site refused it. The page Hisar entered it on is gone. Tell the user; do not retry."},
		{signin.Result{Outcome: signin.Stopped, Cleanup: signin.CleanupPageLeft, Detail: "the page moved to https://accounts.example.com", Steps: []signin.Step{{Filled: []signin.Part{signin.PartUsername}, IsSubmitted: true}}},
			cliprotocol.SignInStopped, cliprotocol.SignInCleanupPageLeft,
			"Hisar submitted the username and stopped: the page moved to https://accounts.example.com. Take a screenshot, handle the page, and call sign_in again if a username, password or one-time code field shows."},
		{signin.Result{Outcome: signin.CleanupFailed, Cleanup: signin.CleanupUnverified, Steps: []signin.Step{{Filled: []signin.Part{signin.PartPassword}, IsSubmitted: true}}},
			cliprotocol.SignInCleanupFailed, cliprotocol.SignInCleanupUnverified,
			"Hisar entered user@example.com's login on https://tosunlab.com but could not confirm it cleared it from the page. Stop, and tell the user to check the browser window. Do not call sign_in again."},
	} {
		described := describeAgentSignIn(destination, login, testCase.result)
		if described.Message != testCase.message || described.Outcome != testCase.outcome || described.Cleanup != testCase.cleanup ||
			described.Website != "https://tosunlab.com" || described.Browser != "Google Chrome" || described.Login.Title != "Tosun Lab" || len(described.Steps) != len(testCase.result.Steps) {
			t.Errorf("%s %s: %+v", testCase.result.Outcome, testCase.result.Cleanup, described)
		}
	}
}

type signInWindow struct {
	mutex      sync.Mutex
	website    string
	values     map[signin.ElementID]string
	isSignedIn bool
	posted     []map[signin.ElementID]string
	failClear  bool
	// onEnter runs after an entry, outside the window's lock.
	onEnter func(field signin.ElementID)
}

const (
	usernameField signin.ElementID = 11
	passwordField signin.ElementID = 12
	signInButton  signin.ElementID = 13
)

var signInWindowTarget = &cliprotocol.Window{PID: 4242, WindowID: 7}

func (*signInWindow) HasAccess() bool { return true }

func (*signInWindow) RequestAccess() {}

func (*signInWindow) System() signin.System { return signin.SystemMac }

func (*signInWindow) ListPrompts(context.Context, signin.ProcessInstance) ([]signin.Prompt, error) {
	return nil, nil
}

func (*signInWindow) DismissPrompt(context.Context, signin.ProcessInstance, signin.ElementID) error {
	return signin.ErrChanged
}

func (window *signInWindow) Observe(_ context.Context, target signin.Target) (signin.Snapshot, error) {
	if target != (signin.Target{PID: 4242, WindowID: 7}) {
		return nil, signin.ErrWindowNotFound
	}
	window.mutex.Lock()
	defer window.mutex.Unlock()
	page := signin.Page{Process: agentharness.Process{
		PID: 4242, StartTime: 1_700_000_000, Path: "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		Arguments: []string{"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"},
		Signature: agentharness.Signature{Identifier: "com.google.Chrome", TeamID: "EQHXZ8M8AV"},
	}}
	if window.isSignedIn {
		page.Frames = []signin.Frame{{Parent: -1, Document: 2, URL: window.website + "/dashboard"}}
		return &signInSnapshot{window: window, page: page}, nil
	}
	page.Frames = []signin.Frame{{Parent: -1, Document: 1, URL: window.website + "/login"}}
	page.Fields = []signin.Field{
		{ID: usernameField, InputType: "email", Label: "Email", Identifier: "email", IsEditable: true, IsVisible: true, Length: utf8.RuneCountInString(window.values[usernameField])},
		{ID: passwordField, IsSecure: true, InputType: "password", Label: "Password", Identifier: "password", IsEditable: true, IsVisible: true, Length: utf8.RuneCountInString(window.values[passwordField])},
	}
	page.Buttons = []signin.Button{{ID: signInButton, Title: "Sign in", IsVisible: true}}
	return &signInSnapshot{window: window, page: page}, nil
}

func (window *signInWindow) getValue(field signin.ElementID) string {
	window.mutex.Lock()
	defer window.mutex.Unlock()
	return window.values[field]
}

type signInSnapshot struct {
	window *signInWindow
	page   signin.Page
}

func (snapshot *signInSnapshot) Page() signin.Page { return snapshot.page }

func (snapshot *signInSnapshot) isCurrent() bool {
	return len(snapshot.page.Fields) > 0 && !snapshot.window.isSignedIn
}

func (snapshot *signInSnapshot) Enter(field signin.ElementID, value signin.Secret) error {
	window := snapshot.window
	window.mutex.Lock()
	if !snapshot.isCurrent() {
		window.mutex.Unlock()
		return signin.ErrChanged
	}
	window.values[field] = value.Reveal()
	onEnter := window.onEnter
	window.mutex.Unlock()
	if onEnter != nil {
		onEnter(field)
	}
	return nil
}

func (snapshot *signInSnapshot) Clear(field signin.ElementID) error {
	window := snapshot.window
	window.mutex.Lock()
	defer window.mutex.Unlock()
	if window.failClear {
		return errors.New("browser refused to clear the field")
	}
	if !snapshot.isCurrent() {
		return signin.ErrChanged
	}
	delete(window.values, field)
	return nil
}

func (snapshot *signInSnapshot) Press(signin.ElementID) error {
	window := snapshot.window
	window.mutex.Lock()
	defer window.mutex.Unlock()
	if !snapshot.isCurrent() {
		return signin.ErrChanged
	}
	window.posted = append(window.posted, map[signin.ElementID]string{usernameField: window.values[usernameField], passwordField: window.values[passwordField]})
	window.isSignedIn, window.values = true, map[signin.ElementID]string{}
	return nil
}

func (snapshot *signInSnapshot) PressReturn(field signin.ElementID) error {
	return snapshot.Press(field)
}

func (snapshot *signInSnapshot) ReadBack(field signin.ElementID) (int, error) {
	window := snapshot.window
	window.mutex.Lock()
	defer window.mutex.Unlock()
	if !snapshot.isCurrent() {
		return 0, signin.ErrChanged
	}
	return utf8.RuneCountInString(window.values[field]), nil
}

func (*signInSnapshot) Close() {}

type agentSignInTest struct {
	bridge    *CommandLineBridge
	approvals *clisocket.SessionApprovals
	logins    signInLogins
	approver  *approvingAgentApprover
	agent     *commandLineTest
}

func startAgentSignIn(t *testing.T, testService unlockedTestService, window *signInWindow) *agentSignInTest {
	t.Helper()
	service := testService.service
	window.values = map[signin.ElementID]string{}
	service.config.SignInDriver = window
	service.signer = signin.NewSigner(window, t.TempDir(), service.config.Now)
	if _, err := service.SetAgentSignInEnabled(true); err != nil {
		t.Fatal(err)
	}
	test := &agentSignInTest{approvals: clisocket.NewSessionApprovals(0, nil, nil), logins: saveSignInLogins(t, testService), approver: &approvingAgentApprover{duration: clisocket.AgentGrantOnce}}
	test.bridge = NewCommandLineBridge(service, test.approvals)
	test.connect(t, test.bridge)
	return test
}

func (test *agentSignInTest) connect(t *testing.T, handler clisocket.Handler) {
	t.Helper()
	server := clisocket.NewServer(clisocket.ServerConfig{Handler: handler, AgentApprover: test.approver, Approvals: test.approvals, PeerResolver: staticPeerResolver{}})
	serverSide, clientSide := net.Pipe()
	go server.ServeConnection(context.Background(), serverSide)
	t.Cleanup(func() { clientSide.Close() })
	test.agent = &commandLineTest{connection: clientSide}
	test.agent.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeAgentHello, AgentName: "claude-code"}, nil)
}

func (test *agentSignInTest) signIn(t *testing.T, itemID string) (result cliprotocol.AgentSignInResult, failure string) {
	t.Helper()
	failure = test.agent.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentSignIn, Window: signInWindowTarget, ItemID: itemID}, &result)
	return result, failure
}

// editDuringApproval has the approval save a new revision of the Tosun Lab login, which reading the login refuses, so a
// refusal that names it shows the login was read.
func (test *agentSignInTest) editDuringApproval(t *testing.T, testService unlockedTestService, alsoDuringApproval func()) {
	test.approver.onApprove = func() {
		detail, err := testService.service.GetItem(testService.personalID, test.logins.tosunlab.ID)
		if err != nil {
			t.Error(err)
			return
		}
		detail.Notes = "edited"
		testService.service.SaveItem(testService.personalID, detail)
		alsoDuringApproval()
	}
}

// TestAgentSignInReadsNoLoginForAChangedWindow: the window is observed again before the login is read, so a window that
// moved to another website refuses as a changed window, before the login's own change is ever looked at.
func TestAgentSignInReadsNoLoginForAChangedWindow(t *testing.T) {
	testService := createUnlockedTestService(t)
	window := &signInWindow{website: "https://tosunlab.com"}
	test := startAgentSignIn(t, testService, window)
	test.editDuringApproval(t, testService, func() {
		window.mutex.Lock()
		window.website = "https://unapproved.example"
		window.mutex.Unlock()
	})
	if _, failure := test.signIn(t, test.logins.tosunlab.ID); !strings.HasPrefix(failure, "sign-in-refused: The window changed while the user was approving: it now shows https://unapproved.example.") {
		t.Errorf("refusal %q", failure)
	}
	if len(window.posted) != 0 || len(window.values) != 0 {
		t.Errorf("the page got %v", window.posted)
	}
}

type cancellingBridge struct {
	*CommandLineBridge
}

func (bridge cancellingBridge) PerformAgentSignIn(ctx context.Context, target clisocket.AgentSignInTarget, login clisocket.AgentLogin, authorization clisocket.AgentAuthorization) (cliprotocol.AgentSignInResult, error) {
	ended, cancel := context.WithCancel(ctx)
	cancel()
	return bridge.CommandLineBridge.PerformAgentSignIn(ended, target, login, authorization)
}

// TestAgentSignInReadsNoLoginForAnEndedRequest: a request that ended is refused before the login is read. The signer
// refuses it while waiting for its turn, or, when it takes the turn first, the app refuses it before reading the login;
// a read login would refuse as changed, since the approval edited it.
func TestAgentSignInReadsNoLoginForAnEndedRequest(t *testing.T) {
	testService := createUnlockedTestService(t)
	window := &signInWindow{website: "https://tosunlab.com"}
	test := startAgentSignIn(t, testService, window)
	test.connect(t, cancellingBridge{test.bridge})
	test.editDuringApproval(t, testService, func() {})
	_, failure := test.signIn(t, test.logins.tosunlab.ID)
	if !strings.HasPrefix(failure, "sign-in-refused: The sign-in request ended before Hisar entered anything.") &&
		!strings.HasPrefix(failure, "sign-in-refused: The sign-in was interrupted") {
		t.Errorf("refusal %q", failure)
	}
	if len(window.posted) != 0 || len(window.values) != 0 {
		t.Errorf("the page got %v", window.posted)
	}
}

func TestAgentSignInSignsInWithTheApprovedLoginAndTellsNoValue(t *testing.T) {
	testService := createUnlockedTestService(t)
	window := &signInWindow{website: "https://tosunlab.com"}
	test := startAgentSignIn(t, testService, window)
	test.approver.loginIndex = 1
	result, failure := test.signIn(t, "")
	if failure != "" {
		t.Fatal(failure)
	}
	approval := test.approver.requests[0].SignIn
	if approval == nil || approval.IsLocked || approval.Website != "https://tosunlab.com" || approval.BrowserName != "Google Chrome" || len(approval.Logins) != 2 ||
		approval.Logins[1].ItemID != test.logins.labStaging.ID || approval.Logins[1].Origin != "https://tosunlab.com" {
		t.Fatalf("approval %+v", approval)
	}
	if result.Outcome != cliprotocol.SignInSubmitted || result.Website != "https://tosunlab.com" || result.Login.Username != "ops@example.com" ||
		result.Message != "Hisar signed in to https://tosunlab.com as ops@example.com and submitted it. Take a screenshot to see the page." {
		t.Errorf("result %+v", result)
	}
	if len(window.posted) != 1 || window.posted[0][usernameField] != "ops@example.com" || window.posted[0][passwordField] != "staging-password" {
		t.Errorf("the site got %v", window.posted)
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "staging-password") {
		t.Errorf("the result carries the password: %s", encoded)
	}
}

func TestAgentSignInStopsAndClearsWhenHisarLocks(t *testing.T) {
	testService := createUnlockedTestService(t)
	service := testService.service
	window := &signInWindow{website: "https://tosunlab.com"}
	test := startAgentSignIn(t, testService, window)
	window.onEnter = func(field signin.ElementID) {
		if field == passwordField {
			service.Lock()
		}
	}
	_, failure := test.signIn(t, test.logins.tosunlab.ID)
	if !strings.HasPrefix(failure, cliprotocol.ErrorDenied+": ") {
		t.Fatalf("a sign-in Hisar locked during: %s", failure)
	}
	if len(window.posted) != 0 || window.getValue(usernameField) != "" || window.getValue(passwordField) != "" {
		t.Errorf("after the lock the page holds %q / %q and got %v", window.getValue(usernameField), window.getValue(passwordField), window.posted)
	}
	if strings.Contains(failure, "tosunlab-password") {
		t.Errorf("the refusal carries the password: %s", failure)
	}

	target, err := test.bridge.PrepareAgentSignIn(test.bridge.BindSession(context.Background()), &cliprotocol.Request{Type: cliprotocol.TypeAgentSignIn, Window: signInWindowTarget})
	if err != nil || !target.IsLocked || len(target.Logins) != 0 || target.Destination.Origin.String() != "https://tosunlab.com" {
		t.Errorf("prepared while locked: %+v %v", target, err)
	}
}

func TestAgentSignInRefusesAPageNoAgentLoginIsSavedFor(t *testing.T) {
	testService := createUnlockedTestService(t)
	window := &signInWindow{website: "https://tosunlab-login.example.net"}
	test := startAgentSignIn(t, testService, window)
	_, err := test.bridge.PrepareAgentSignIn(test.bridge.BindSession(context.Background()), &cliprotocol.Request{Type: cliprotocol.TypeAgentSignIn, Window: signInWindowTarget})
	if !strings.HasPrefix(getRefusal(err), "No login in the vaults available to agents is saved for https://tosunlab-login.example.net.") {
		t.Errorf("phishing page: %v", err)
	}
	if len(window.values) != 0 || len(window.posted) != 0 {
		t.Error("Hisar entered something on a page no login is saved for")
	}
}

func TestAgentSignInCleanupFailureCannotReuseSavedGrant(t *testing.T) {
	for _, cause := range []string{"locked", "keychain write failed"} {
		t.Run(cause, func(t *testing.T) {
			test := createAgentSessionTest(t, clisocket.AgentGrantAlways)
			service := test.testService.service
			prepareTestApprovalUnlock(t, test.testService, "master password")
			logins := saveSignInLogins(t, test.testService)
			window := &signInWindow{website: "https://tosunlab.com", values: map[signin.ElementID]string{}}
			service.config.SignInDriver = window
			service.signer = signin.NewSigner(window, t.TempDir(), service.config.Now)
			if _, err := service.SetAgentSignInEnabled(true); err != nil {
				t.Fatal(err)
			}
			client := test.connectAgent(t)
			request := cliprotocol.Request{Type: cliprotocol.TypeAgentSignIn, Window: signInWindowTarget, ItemID: logins.tosunlab.ID}
			client.mustSend(t, request, nil)
			if grants := service.ListAgentGrants(); len(grants) != 1 {
				t.Fatalf("saved grants %+v", grants)
			}

			window.mutex.Lock()
			window.isSignedIn, window.failClear = false, true
			window.onEnter = func(field signin.ElementID) {
				if field == passwordField {
					if cause == "keychain write failed" {
						getTestAgentGrantHeads(service).setError(errKeychainDenied)
					}
					service.Lock()
				}
			}
			window.mutex.Unlock()
			var result cliprotocol.AgentSignInResult
			failure := client.send(t, request, &result)
			if cause == "locked" {
				if !strings.HasPrefix(failure, cliprotocol.ErrorDenied+": ") || result.Outcome != "" {
					t.Errorf("locked cleanup returned %+v, %q", result, failure)
				}
			} else if !strings.Contains(failure, "could not save the grant revocation") {
				t.Errorf("grant-store failure was swallowed: %+v, %q", result, failure)
			}

			getTestAgentGrantHeads(service).setError(nil)
			if _, err := service.Unlock("master password"); err != nil {
				t.Fatal(err)
			}
			if grants := service.ListAgentGrants(); len(grants) != 0 {
				t.Errorf("cleanup-failed grant reopened: %+v", grants)
			}
			if sealed := readSealedAgentGrants(t, service); len(sealed) != 0 {
				t.Error("cleanup-failed grant remains on disk")
			}
			window.mutex.Lock()
			window.onEnter, window.failClear = nil, false
			window.mutex.Unlock()
			test.connectAgent(t).mustSend(t, request, nil)
			if count := test.agentApprover.getCount(); count != 2 {
				t.Errorf("sign-in after cleanup-failed reused its saved grant: %d approvals", count)
			}
		})
	}
}

func TestAlwaysSignInWhileLocked(t *testing.T) {
	for _, isEnabled := range []bool{false, true} {
		t.Run(fmt.Sprint(isEnabled), func(t *testing.T) {
			test := createAgentSessionTest(t, clisocket.AgentGrantAlways)
			service := test.testService.service
			prepareTestApprovalUnlock(t, test.testService, "master password")
			logins := saveSignInLogins(t, test.testService)
			window := &signInWindow{website: "https://tosunlab.com", values: map[signin.ElementID]string{}}
			service.config.SignInDriver = window
			service.signer = signin.NewSigner(window, t.TempDir(), service.config.Now)
			service.SetAgentSignInEnabled(true)
			service.SetAlwaysAgentAccessWhileLocked(isEnabled)
			client := test.connectAgent(t)
			request := cliprotocol.Request{Type: cliprotocol.TypeAgentSignIn, Window: signInWindowTarget, ItemID: logins.tosunlab.ID}
			client.mustSend(t, request, nil)
			window.mutex.Lock()
			window.isSignedIn = false
			window.mutex.Unlock()
			service.Lock()
			test.agentApprover.onApprove = func() {
				if _, err := service.Unlock("master password"); err != nil {
					t.Error(err)
				}
			}
			var result cliprotocol.AgentSignInResult
			client.mustSend(t, request, &result)
			expectedCount := 2
			if isEnabled {
				expectedCount = 1
			}
			if count := test.agentApprover.getCount(); count != expectedCount || result.Outcome != cliprotocol.SignInSubmitted {
				t.Fatalf("locked sign-in: %d approvals, %+v", count, result)
			}
			if len(window.posted) != 2 || window.posted[1][passwordField] != "tosunlab-password" {
				t.Fatalf("site received %+v", window.posted)
			}
			if isEnabled && service.GetStatus().State != "locked" {
				t.Fatal("Always sign-in unlocked Hisar")
			}
		})
	}
}

func TestLockedSignInStopsWhenTheRetainedViewEnds(t *testing.T) {
	for _, action := range []string{"unlock", "switch off", "revoke", "lock", "cleanup-failed"} {
		t.Run(action, func(t *testing.T) {
			test := createAgentSessionTest(t, clisocket.AgentGrantAlways)
			service := test.testService.service
			prepareTestApprovalUnlock(t, test.testService, "master password")
			logins := saveSignInLogins(t, test.testService)
			window := &signInWindow{website: "https://tosunlab.com", values: map[signin.ElementID]string{}}
			service.config.SignInDriver = window
			service.signer = signin.NewSigner(window, t.TempDir(), service.config.Now)
			service.SetAgentSignInEnabled(true)
			service.SetAlwaysAgentAccessWhileLocked(true)
			client := test.connectAgent(t)
			request := cliprotocol.Request{Type: cliprotocol.TypeAgentSignIn, Window: signInWindowTarget, ItemID: logins.tosunlab.ID}
			client.mustSend(t, request, nil)
			grantID := service.ListAgentGrants()[0].ID
			window.mutex.Lock()
			window.isSignedIn = false
			window.failClear = action == "cleanup-failed"
			window.onEnter = func(field signin.ElementID) {
				if field != usernameField {
					return
				}
				switch action {
				case "unlock":
					service.Unlock("master password")
				case "switch off":
					service.SetAlwaysAgentAccessWhileLocked(false)
				case "revoke":
					service.RevokeAgentGrant(grantID)
				case "lock", "cleanup-failed":
					service.Lock()
				}
			}
			window.mutex.Unlock()
			service.Lock()
			failure := client.send(t, request, nil)
			if failure == "" || len(window.posted) != 1 || window.getValue(passwordField) != "" {
				t.Fatalf("continued after %s: %q, posted %+v", action, failure, window.posted)
			}
			if test.agentApprover.getCount() != 1 {
				t.Fatal("locked sign-in opened approval")
			}
			if action == "cleanup-failed" && (service.lockedGrants != nil || len(service.state.SealedAgentGrants) != 0) {
				t.Fatal("cleanup-failed kept the locked grant view")
			}
		})
	}
}
