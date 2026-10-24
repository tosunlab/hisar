package clisocket

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/agentharness"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/signin"
)

type fakeSignInHandler struct {
	*fakeAgentHandler
	signInMutex  sync.Mutex
	targets      []AgentSignInTarget
	prepareError error
	prepareCount int
	performed    []AgentLogin
	outcome      string
}

func (handler *fakeSignInHandler) PrepareAgentSignIn(_ context.Context, request *cliprotocol.Request) (AgentSignInTarget, error) {
	handler.signInMutex.Lock()
	defer handler.signInMutex.Unlock()
	handler.prepareCount++
	if handler.prepareError != nil {
		return AgentSignInTarget{}, handler.prepareError
	}
	target := handler.targets[min(handler.prepareCount, len(handler.targets))-1]
	if request.ItemID != "" {
		target.Logins = slices.DeleteFunc(slices.Clone(target.Logins), func(login AgentLogin) bool { return login.ItemID != request.ItemID })
	}
	return target, nil
}

func (handler *fakeSignInHandler) PerformAgentSignIn(_ context.Context, target AgentSignInTarget, login AgentLogin, _ AgentAuthorization) (cliprotocol.AgentSignInResult, error) {
	handler.signInMutex.Lock()
	defer handler.signInMutex.Unlock()
	handler.performed = append(handler.performed, login)
	outcome := handler.outcome
	if outcome == "" {
		outcome = cliprotocol.SignInSubmitted
	}
	return cliprotocol.AgentSignInResult{Outcome: outcome, Website: target.Destination.Origin.String(), Login: cliprotocol.AgentSignInLogin{ID: login.ItemID, Username: login.Username}}, nil
}

func (handler *fakeSignInHandler) setTargets(targets ...AgentSignInTarget) {
	handler.signInMutex.Lock()
	defer handler.signInMutex.Unlock()
	handler.targets, handler.prepareCount = targets, 0
}

func (handler *fakeSignInHandler) getPerformed() []AgentLogin {
	handler.signInMutex.Lock()
	defer handler.signInMutex.Unlock()
	return slices.Clone(handler.performed)
}

type signInFixture struct {
	*agentFixture
	signInHandler *fakeSignInHandler
}

func mustParseOrigin(t *testing.T, rawURL string) signin.Origin {
	t.Helper()
	origin, err := signin.ParseOrigin(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return origin
}

func createSignInFixture(t *testing.T) *signInFixture {
	t.Helper()
	fixture := &signInFixture{agentFixture: createAgentFixture(t)}
	fixture.signInHandler = &fakeSignInHandler{fakeAgentHandler: fixture.agentHandler}
	fixture.signInHandler.setTargets(fixture.buildTarget(t, "https://tosunlab.com", labLogin))
	fixture.agentHandler.secretsByReference["hisar://Work/Tosun Lab/password"] = AgentSecret{VaultID: "v2", ItemID: "tosunlab", VaultName: "Work", ItemTitle: "Tosun Lab", FieldLabel: "password"}
	fixture.agentHandler.resultBy[cliprotocol.TypeAgentResolveSecret] = cliprotocol.ResolveSecretResult{Values: []string{"value"}}
	fixture.server = NewServer(ServerConfig{
		Handler: fixture.signInHandler, AgentApprover: fixture.agentApprover,
		PeerResolver: fixture.resolver, Approvals: fixture.approvals, IsEnabled: fixture.isEnabled.Load,
	})
	return fixture
}

var (
	labLogin      = AgentLogin{AgentSecret: AgentSecret{VaultID: "v2", ItemID: "tosunlab", VaultName: "Work", ItemTitle: "Tosun Lab"}, Username: "user@example.com", Revision: "r1"}
	labAdminLogin = AgentLogin{AgentSecret: AgentSecret{VaultID: "v2", ItemID: "tosunlab-admin", VaultName: "Work", ItemTitle: "Tosun Lab admin"}, Username: "admin@example.com", Revision: "r7"}
)

func (fixture *signInFixture) buildTarget(t *testing.T, website string, logins ...AgentLogin) AgentSignInTarget {
	t.Helper()
	origin := mustParseOrigin(t, website)
	target := AgentSignInTarget{Destination: signin.Destination{
		Target: signin.Target{PID: 81234, WindowID: 40512}, Process: signin.ProcessInstance{PID: 81234, StartTime: 7},
		Browser: signin.Browser{ID: "chrome", Name: "Google Chrome"}, Origin: origin,
	}}
	for _, login := range logins {
		login.Origin = origin.String()
		target.Logins = append(target.Logins, login)
	}
	return target
}

var signInRequest = cliprotocol.Request{Type: cliprotocol.TypeAgentSignIn, Window: &cliprotocol.Window{PID: 81234, WindowID: 40512}}

func TestAgentSignInAsksWithTheObservedDestination(t *testing.T) {
	fixture := createSignInFixture(t)
	client := fixture.connectAgent(t)
	fixture.agentApprover.set(nil, AgentGrantOnce)
	var result cliprotocol.AgentSignInResult
	readResult(t, client.send(t, signInRequest), &result)
	if result.Outcome != cliprotocol.SignInSubmitted || result.Login.ID != "tosunlab" {
		t.Errorf("result %+v", result)
	}
	request := fixture.agentApprover.getLastRequest()
	if request.Kind != AgentRequestSignIn || request.SignIn == nil || request.SignIn.Website != "https://tosunlab.com" || request.SignIn.BrowserName != "Google Chrome" ||
		len(request.SignIn.Logins) != 1 || request.SignIn.Logins[0].Username != "user@example.com" || request.SignIn.IsLocked || !request.IsConnectionApproval ||
		!slices.Equal(request.GrantDurations, []AgentGrantDuration{AgentGrantOnce, AgentGrantHour}) {
		t.Errorf("approval %+v", request)
	}
	if code := getErrorCode(client.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentListVault})); code != "" || fixture.agentApprover.getRequestCount() != 1 {
		t.Errorf("the sign-in's approval did not approve the connection: %q", code)
	}
}

func TestAgentSignInOnlyThisTimeCoversOnlyThatCall(t *testing.T) {
	fixture := createSignInFixture(t)
	client := fixture.connectAgent(t)
	fixture.agentApprover.set(nil, AgentGrantOnce)
	client.mustSend(t, signInRequest)
	sameWindow := fixture.buildTarget(t, "https://tosunlab.com", labLogin)
	otherWindow := sameWindow
	otherWindow.Destination.Target.WindowID++
	fixture.agentApprover.set(ErrDenied, AgentGrantOnce)
	for _, target := range []AgentSignInTarget{sameWindow, otherWindow} {
		fixture.signInHandler.setTargets(target)
		window := &cliprotocol.Window{PID: target.Destination.Target.PID, WindowID: target.Destination.Target.WindowID}
		if code := getErrorCode(client.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentSignIn, Window: window})); code != cliprotocol.ErrorDenied {
			t.Errorf("Only this time covered another sign-in in window %d: %q", window.WindowID, code)
		}
	}
	if count, performed := fixture.agentApprover.getRequestCount(), fixture.signInHandler.getPerformed(); count != 3 || len(performed) != 1 {
		t.Errorf("%d approvals, %d sign-ins", count, len(performed))
	}
}

func TestAgentSignInGrantIsBoundToTheExactOrigin(t *testing.T) {
	fixture := createSignInFixture(t)
	client := fixture.connectAgent(t)
	fixture.agentApprover.set(nil, AgentGrantHour)
	client.mustSend(t, signInRequest)
	fixture.agentApprover.set(ErrDenied, AgentGrantOnce)
	for _, website := range []string{"https://www.tosunlab.com", "http://127.0.0.1:8765", "https://tosunlab.com:8443"} {
		fixture.signInHandler.setTargets(fixture.buildTarget(t, website, labLogin))
		if code := getErrorCode(client.send(t, signInRequest)); code != cliprotocol.ErrorDenied {
			t.Errorf("the grant for https://tosunlab.com covered %s: %q", website, code)
		}
	}
	fixture.signInHandler.setTargets(fixture.buildTarget(t, "https://tosunlab.com", labLogin))
	client.mustSend(t, signInRequest)
	fixture.advance(AgentGrantLifetime)
	if code := getErrorCode(client.send(t, signInRequest)); code != cliprotocol.ErrorDenied {
		t.Errorf("the hour's grant outlived its hour: %q", code)
	}
	if performed := fixture.signInHandler.getPerformed(); len(performed) != 2 {
		t.Errorf("performed %d sign-ins, want the approved one and the granted one", len(performed))
	}
}

func TestAgentSignInAndSecretGrantsNeverCoverEachOther(t *testing.T) {
	fixture := createSignInFixture(t)
	harness, _ := startHarness(t)
	client := fixture.connectInside(t, harness, true)
	fixture.agentApprover.set(nil, AgentGrantAlways)
	client.mustSend(t, signInRequest)
	fixture.agentApprover.set(ErrDenied, AgentGrantOnce)
	run := cliprotocol.Request{Type: cliprotocol.TypeAgentResolveSecret, References: []string{"hisar://Work/Tosun Lab/password"}, CommandLine: []string{"true"}}
	if code := getErrorCode(client.send(t, run)); code != cliprotocol.ErrorDenied {
		t.Errorf("a sign-in grant let the agent use the password in a command: %q", code)
	}
	if code := getErrorCode(client.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentRevealSecret, References: []string{"hisar://Work/Tosun Lab/password"}})); code != cliprotocol.ErrorDenied {
		t.Errorf("a sign-in grant let the agent read the password: %q", code)
	}

	fixture.approvals.RevokeAgentGrant("harness/omp/" + fixture.buildTarget(t, "https://tosunlab.com", labLogin).Logins[0].getGrantKey(AgentRequestSignIn))
	if grants := fixture.approvals.ListAgentGrants(); len(grants) != 0 {
		t.Fatalf("revoking left %+v", grants)
	}
	fixture.signInHandler.setTargets(fixture.buildTarget(t, "https://tosunlab.com", labLogin))
	fixture.agentApprover.set(nil, AgentGrantAlways)
	client.mustSend(t, run)
	client.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeAgentRevealSecret, References: []string{"hisar://Work/Tosun Lab/password"}})
	fixture.agentApprover.set(ErrDenied, AgentGrantOnce)
	if code := getErrorCode(client.send(t, signInRequest)); code != cliprotocol.ErrorDenied {
		t.Errorf("use and reveal grants let the agent sign in: %q", code)
	}
}

func TestAgentSignInFromAHarnessIsRememberedForTheHarness(t *testing.T) {
	fixture := createSignInFixture(t)
	harness, _ := startHarness(t)
	fixture.agentApprover.set(nil, AgentGrantDay)
	fixture.connectInside(t, harness, true).mustSend(t, signInRequest)
	if request := fixture.agentApprover.getLastRequest(); !slices.Equal(request.GrantDurations, []AgentGrantDuration{AgentGrantOnce, AgentGrantHour, AgentGrantDay, AgentGrantAlways}) {
		t.Errorf("a harness's sign-in offered %v", request.GrantDurations)
	}
	grants := fixture.approvals.ListAgentGrants()
	if len(grants) != 1 || grants[0].Kind != AgentRequestSignIn || grants[0].Secret.Origin != "https://tosunlab.com" || grants[0].HarnessID != "omp" {
		t.Fatalf("grants %+v", grants)
	}
	fixture.agentApprover.set(ErrDenied, AgentGrantOnce)
	other, _ := startHarness(t)
	fixture.connectInside(t, other, true).mustSend(t, signInRequest)
	if count := fixture.agentApprover.getRequestCount(); count != 1 {
		t.Errorf("another omp session asked again: %d approvals", count)
	}
}

func TestHarnessGrantNeedsTheProgramItWasApprovedFor(t *testing.T) {
	fixture := createSignInFixture(t)
	omp, _ := startHarness(t)
	fixture.agentApprover.set(nil, AgentGrantAlways)
	fixture.connectInside(t, omp, true).mustSend(t, signInRequest)

	impostorProcess := exec.Command("/bin/sleep", "60")
	impostorProcess.Args[0] = "omp"
	if err := impostorProcess.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		impostorProcess.Process.Kill()
		impostorProcess.Wait()
	})
	impostor, isFound, err := agentharness.Find(agentharness.ReadChain(impostorProcess.Process.Pid, agentharness.SystemReader()))
	if err != nil || !isFound || impostor.Harness.ID != "omp" || impostor.PID != impostorProcess.Process.Pid || impostor.Program.IsKnown() || impostor.IsSigned() {
		t.Fatalf("the impostor's session %+v, %v, %v", impostor, isFound, err)
	}
	fixture.agentApprover.set(ErrDenied, AgentGrantOnce)
	if code := getErrorCode(fixture.connectInside(t, impostor, true).send(t, signInRequest)); code != cliprotocol.ErrorDenied || fixture.agentApprover.getRequestCount() != 2 {
		t.Errorf("a program named omp in its first argument used omp's grant: %q", code)
	}

	unknown, _ := startHarness(t)
	unknown.Program = agentharness.Program{}
	fixture.connectInside(t, unknown, true).send(t, signInRequest)
	if request := fixture.agentApprover.getLastRequest(); !slices.Equal(request.GrantDurations, []AgentGrantDuration{AgentGrantOnce, AgentGrantHour}) {
		t.Errorf("a harness whose program is unknown was offered %v", request.GrantDurations)
	}
	if performed := fixture.signInHandler.getPerformed(); len(performed) != 1 {
		t.Errorf("performed %d sign-ins; want only omp's", len(performed))
	}
}

func TestHarnessGrantNeedsTheScriptItWasApprovedFor(t *testing.T) {
	runScript := func(path string) agentharness.Session {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("setTimeout(() => {}, 60000)\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		found, isFound, _ := agentharness.Find(agentharness.Chain{IsKnown: true, Processes: []agentharness.Process{
			{PID: 46809, ParentPID: 1, StartTime: 200, Path: "/opt/homebrew/bin/bun", Arguments: []string{"bun", path}},
		}})
		if !isFound || !found.Program.IsKnown() {
			t.Fatalf("script %s: %+v, %v", path, found, isFound)
		}
		session, _ := startHarness(t)
		session.Program = found.Program
		return session
	}
	directory := t.TempDir()
	approved, impostor := runScript(filepath.Join(directory, "approved", "omp")), runScript(filepath.Join(directory, "impostor", "omp"))
	fixture := createSignInFixture(t)
	fixture.agentApprover.set(nil, AgentGrantAlways)
	fixture.connectInside(t, approved, true).mustSend(t, signInRequest)
	fixture.agentApprover.set(ErrDenied, AgentGrantOnce)
	if code := getErrorCode(fixture.connectInside(t, impostor, true).send(t, signInRequest)); code != cliprotocol.ErrorDenied {
		t.Errorf("another script named omp used the grant: %q (programs %+v and %+v)", code, approved.Program, impostor.Program)
	}
	again, _ := startHarness(t)
	again.Program = approved.Program
	fixture.connectInside(t, again, true).mustSend(t, signInRequest)
	if count, performed := fixture.agentApprover.getRequestCount(), fixture.signInHandler.getPerformed(); count != 2 || len(performed) != 2 {
		t.Errorf("%d approvals, %d sign-ins; want the approved script's two sign-ins and one dialog for the other script", count, len(performed))
	}
}

func TestHermesGrantNeedsTheLauncherSourceItWasApprovedFor(t *testing.T) {
	python := "/Users/developer/.hermes/tools/python-3.14.7+20260901-darwin-arm64/bin/python3"
	launcher := "import sys\nsys.path.insert(0, '/Users/developer/.hermes/hermes-agent')\nfrom hermes_cli.main import main\nsys.exit(main())\n"
	runHermes := func(source string) agentharness.Session {
		found, isFound, _ := agentharness.Find(agentharness.Chain{IsKnown: true, Processes: []agentharness.Process{
			{PID: 35706, ParentPID: 1, StartTime: 100, Path: python, Arguments: []string{python, "-I", "-c", source, "chat"}},
		}})
		if !isFound || found.Harness.ID != "hermes" {
			t.Fatalf("Hermes: %+v, %v", found, isFound)
		}
		session, _ := startHarness(t)
		session.Harness, session.Program = found.Harness, found.Program
		return session
	}
	fixture := createSignInFixture(t)
	fixture.agentApprover.set(nil, AgentGrantAlways)
	fixture.connectInside(t, runHermes(launcher), true).mustSend(t, signInRequest)
	if request := fixture.agentApprover.getLastRequest(); !slices.Equal(request.GrantDurations, []AgentGrantDuration{AgentGrantOnce, AgentGrantHour, AgentGrantDay, AgentGrantAlways}) {
		t.Errorf("Hermes chat was offered %v", request.GrantDurations)
	}
	fixture.connectInside(t, runHermes(launcher), true).mustSend(t, signInRequest)
	if count := fixture.agentApprover.getRequestCount(); count != 1 {
		t.Errorf("another Hermes session asked again: %d approvals", count)
	}
	fixture.agentApprover.set(ErrDenied, AgentGrantOnce)
	if code := getErrorCode(fixture.connectInside(t, runHermes(launcher+"import os\n"), true).send(t, signInRequest)); code != cliprotocol.ErrorDenied {
		t.Errorf("other -c source under Hermes' Python used its grant: %q", code)
	}
	if count, performed := fixture.agentApprover.getRequestCount(), fixture.signInHandler.getPerformed(); count != 2 || len(performed) != 2 {
		t.Errorf("%d approvals, %d sign-ins; want two Hermes sign-ins and one dialog for the other source", count, len(performed))
	}
}

func TestAgentSignInCleanupFailedEndsTheGrant(t *testing.T) {
	fixture := createSignInFixture(t)
	harness, _ := startHarness(t)
	client := fixture.connectInside(t, harness, true)
	fixture.agentApprover.set(nil, AgentGrantAlways)
	fixture.signInHandler.outcome = cliprotocol.SignInCleanupFailed
	var result cliprotocol.AgentSignInResult
	readResult(t, client.send(t, signInRequest), &result)
	if result.Outcome != cliprotocol.SignInCleanupFailed {
		t.Fatalf("result %+v", result)
	}
	if grants := fixture.approvals.ListAgentGrants(); len(grants) != 0 {
		t.Errorf("a cleanup-failed sign-in kept its grant: %+v", grants)
	}
	fixture.agentApprover.set(ErrDenied, AgentGrantOnce)
	if code := getErrorCode(client.send(t, signInRequest)); code != cliprotocol.ErrorDenied {
		t.Errorf("a cleanup-failed sign-in's flow was granted: %q", code)
	}
}

func TestAgentSignInPicksTheOnlyGrantedLogin(t *testing.T) {
	fixture := createSignInFixture(t)
	client := fixture.connectAgent(t)
	fixture.agentApprover.set(nil, AgentGrantHour)
	fixture.agentApprover.loginIndex = 1
	fixture.signInHandler.setTargets(fixture.buildTarget(t, "https://tosunlab.com", labLogin, labAdminLogin))
	client.mustSend(t, signInRequest)
	if request := fixture.agentApprover.getLastRequest(); len(request.SignIn.Logins) != 2 {
		t.Fatalf("the dialog offered %+v", request.SignIn.Logins)
	}
	fixture.agentApprover.set(ErrDenied, AgentGrantOnce)
	client.mustSend(t, signInRequest)
	if performed := fixture.signInHandler.getPerformed(); len(performed) != 2 || performed[0].ItemID != "tosunlab-admin" || performed[1].ItemID != "tosunlab-admin" {
		t.Errorf("performed %+v", performed)
	}
	fixture.agentApprover.set(nil, AgentGrantHour)
	fixture.agentApprover.loginIndex = 0
	client.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentSignIn, Window: signInRequest.Window, ItemID: "tosunlab"})
	fixture.agentApprover.set(ErrDenied, AgentGrantOnce)
	if code := getErrorCode(client.send(t, signInRequest)); code != cliprotocol.ErrorDenied {
		t.Errorf("with two granted logins the agent picked one: %q", code)
	}
}

func TestAgentSignInRejectsALoginTheDialogDidNotOffer(t *testing.T) {
	fixture := createSignInFixture(t)
	client := fixture.connectAgent(t)
	fixture.agentApprover.set(nil, AgentGrantOnce)
	fixture.agentApprover.loginIndex = 1
	if code := getErrorCode(client.send(t, signInRequest)); code != cliprotocol.ErrorInvalidRequest {
		t.Errorf("login index out of range: %q", code)
	}
	if performed := fixture.signInHandler.getPerformed(); len(performed) != 0 {
		t.Errorf("performed %+v", performed)
	}
}

func TestAgentSignInWhileLockedPreparesAgainAfterTheUnlock(t *testing.T) {
	fixture := createSignInFixture(t)
	client := fixture.connectAgent(t)
	locked := fixture.buildTarget(t, "https://tosunlab.com")
	locked.IsLocked = true
	fixture.agentApprover.set(nil, AgentGrantOnce)

	fixture.signInHandler.setTargets(locked, fixture.buildTarget(t, "https://tosunlab.com", labLogin))
	client.mustSend(t, signInRequest)
	if count := fixture.agentApprover.getRequestCount(); count != 1 || !fixture.agentApprover.getLastRequest().SignIn.IsLocked {
		t.Errorf("one login after the unlock: %d approvals", count)
	}

	fixture.agentApprover.loginIndex = 1
	fixture.signInHandler.setTargets(locked, fixture.buildTarget(t, "https://tosunlab.com", labLogin, labAdminLogin))
	client.mustSend(t, signInRequest)
	if count := fixture.agentApprover.getRequestCount(); count != 3 {
		t.Fatalf("two logins after the unlock: %d approvals, want a second dialog to choose", count)
	}
	if request := fixture.agentApprover.getLastRequest(); request.SignIn.IsLocked || len(request.SignIn.Logins) != 2 {
		t.Errorf("the choice %+v", request.SignIn)
	}
	if performed := fixture.signInHandler.getPerformed(); len(performed) != 2 || performed[1].ItemID != "tosunlab-admin" {
		t.Errorf("performed %+v", performed)
	}

	fixture.signInHandler.setTargets(locked, fixture.buildTarget(t, "https://tosunlab-login.example.net", labLogin))
	response := client.send(t, signInRequest)
	if getErrorCode(response) != cliprotocol.ErrorSignInRefused || response.Error.Message !=
		"The window changed while the user was approving: it now shows https://tosunlab-login.example.net. Hisar entered nothing. Call sign_in again if the sign-in page is back." {
		t.Errorf("a window that changed during the unlock: %+v", response.Error)
	}
	if performed := fixture.signInHandler.getPerformed(); len(performed) != 2 {
		t.Errorf("signed in to a changed window: %+v", performed)
	}
}

func TestAgentSignInRevokedDuringTheDialogNeverPerforms(t *testing.T) {
	fixture := createSignInFixture(t)
	harness, _ := startHarness(t)
	fixture.agentApprover.set(nil, AgentGrantAlways)
	fixture.agentApprover.onApprove = func() { fixture.approvals.RevokeAgentSession(harness.PID, harness.StartTime) }
	client := fixture.connectInside(t, harness, true)
	client.connection.SetDeadline(time.Now().Add(5 * time.Second))
	if err := cliprotocol.WriteJSON(client.connection, cliprotocol.Request{ID: "1", Type: signInRequest.Type, Window: signInRequest.Window}); err != nil {
		t.Fatal(err)
	}
	var response cliprotocol.Response
	if err := cliprotocol.ReadJSON(client.connection, &response); err == nil && getErrorCode(response) != cliprotocol.ErrorDenied {
		t.Errorf("a revoked session signed in: %+v", response)
	}
	if performed := fixture.signInHandler.getPerformed(); len(performed) != 0 || len(fixture.approvals.ListAgentGrants()) != 0 {
		t.Errorf("performed %+v", performed)
	}
}

func TestAgentRequestClearedDuringTheDialogIsDenied(t *testing.T) {
	fixture := createSignInFixture(t)
	client := fixture.connectAgent(t)
	fixture.agentApprover.set(nil, AgentGrantHour)
	fixture.agentApprover.onApprove = fixture.approvals.ClearAll
	response := client.send(t, signInRequest)
	if getErrorCode(response) != cliprotocol.ErrorDenied || response.Error.Message != "Hisar denied the request: it locked or signed out, or the agent's session ended, before the request ran" ||
		len(fixture.signInHandler.getPerformed()) != 0 {
		t.Errorf("a sign-in approved before the lock returned: %+v", response)
	}
	run := cliprotocol.Request{Type: cliprotocol.TypeAgentResolveSecret, References: []string{"hisar://Work/Tosun Lab/password"}, CommandLine: []string{"true"}}
	code := getErrorCode(client.send(t, run))
	fixture.agentHandler.mutex.Lock()
	isHandled := slices.Contains(fixture.agentHandler.types, cliprotocol.TypeAgentResolveSecret)
	fixture.agentHandler.mutex.Unlock()
	if code != cliprotocol.ErrorDenied || isHandled {
		t.Errorf("secrets approved before the lock were handed over: %q", code)
	}
	fixture.agentApprover.onApprove = nil
	fixture.agentApprover.set(ErrDenied, AgentGrantOnce)
	client.send(t, signInRequest)
	client.send(t, run)
	if count := fixture.agentApprover.getRequestCount(); count != 4 {
		t.Errorf("an approval cleared during its dialog was remembered: %d approvals", count)
	}
}

type hookedAgentGrants struct {
	memoryAgentGrants
	onAdd  func()
	onList func()
}

func (store *hookedAgentGrants) AddAgentGrants(ctx context.Context, authorization AgentAuthorization, grants []AgentGrant) error {
	if hook := store.onAdd; hook != nil {
		store.onAdd = nil
		hook()
	}
	return store.memoryAgentGrants.AddAgentGrants(ctx, authorization, grants)
}

func (store *hookedAgentGrants) ListAgentGrants() []AgentGrant {
	grants := store.memoryAgentGrants.ListAgentGrants()
	if hook := store.onList; hook != nil {
		store.onList = nil
		hook()
	}
	return grants
}

func TestAgentAuthorizationEndedWhileGrantsAreSavedOrReadIsDenied(t *testing.T) {
	for name, end := range map[string]func(*signInFixture, agentharness.Session){
		"lock":            func(fixture *signInFixture, _ agentharness.Session) { fixture.approvals.ClearAll() },
		"sign-out":        func(fixture *signInFixture, _ agentharness.Session) { fixture.approvals.SetAccount("") },
		"another account": func(fixture *signInFixture, _ agentharness.Session) { fixture.approvals.SetAccount("other") },
		"session revoked": func(fixture *signInFixture, harness agentharness.Session) {
			fixture.approvals.RevokeAgentSession(harness.PID, harness.StartTime)
		},
	} {
		fixture := createSignInFixture(t)
		fixture.approvals.SetAccount("account")
		store := &hookedAgentGrants{}
		fixture.approvals.SetAgentGrantStore(store)
		harness, _ := startHarness(t)
		fixture.resolver.setAgentSession(harness)
		peer, isIdentified := fixture.server.identifyPeer(nil, 99999)
		if !isIdentified {
			t.Fatal("the peer is unknown")
		}
		peer.kind = connectionAgent
		fixture.agentApprover.set(nil, AgentGrantAlways)
		store.onAdd = func() { end(fixture, harness) }
		var responseError *cliprotocol.ResponseError
		if _, err := fixture.server.answerAgentSignIn(context.Background(), &peer, &signInRequest); !errors.As(err, &responseError) || responseError.Code != cliprotocol.ErrorDenied ||
			responseError.Message != "Hisar denied the request: it locked or signed out, or the agent's session ended, before the request ran" {
			t.Errorf("%s while saving: %v", name, err)
		}
		if performed, saved := fixture.signInHandler.getPerformed(), store.memoryAgentGrants.ListAgentGrants(); len(performed) != 0 || len(saved) != 0 {
			t.Errorf("%s while saving: %d sign-ins, %d grants saved", name, len(performed), len(saved))
		}
	}

	fixture := createSignInFixture(t)
	store := &hookedAgentGrants{}
	fixture.approvals.SetAgentGrantStore(store)
	harness, _ := startHarness(t)
	client := fixture.connectInside(t, harness, true)
	fixture.agentApprover.set(nil, AgentGrantAlways)
	client.mustSend(t, signInRequest)
	fixture.agentApprover.set(ErrDenied, AgentGrantOnce)
	store.onList = fixture.approvals.ClearAll
	if code := getErrorCode(client.send(t, signInRequest)); code != cliprotocol.ErrorDenied || len(fixture.signInHandler.getPerformed()) != 1 {
		t.Errorf("a lock while the grants were read: %q, %d sign-ins", code, len(fixture.signInHandler.getPerformed()))
	}
	run := cliprotocol.Request{Type: cliprotocol.TypeAgentResolveSecret, References: []string{"hisar://Work/Tosun Lab/password"}, CommandLine: []string{"true"}}
	fixture.agentApprover.set(nil, AgentGrantAlways)
	client.mustSend(t, run)
	store.onList = fixture.approvals.ClearAll
	if code := getErrorCode(client.send(t, run)); code != cliprotocol.ErrorDenied || len(fixture.agentHandler.approvedSecrets) != 1 {
		t.Errorf("a lock while the grants were read handed over secrets: %q, %d answers", code, len(fixture.agentHandler.approvedSecrets))
	}
}

func TestAgentSignInIsCheckedBeforeAnything(t *testing.T) {
	fixture := createSignInFixture(t)
	client := fixture.connectAgent(t)
	for _, window := range []*cliprotocol.Window{nil, {PID: 0, WindowID: 1}, {PID: 1, WindowID: 0}} {
		if code := getErrorCode(client.send(t, cliprotocol.Request{Type: cliprotocol.TypeAgentSignIn, Window: window})); code != cliprotocol.ErrorInvalidRequest {
			t.Errorf("window %+v: %q", window, code)
		}
	}
	fixture.signInHandler.prepareError = cliprotocol.NewError(cliprotocol.ErrorSignInRefused, "No login in the vaults available to agents is saved for https://tosunlab-login.example.net.")
	response := client.send(t, signInRequest)
	if getErrorCode(response) != cliprotocol.ErrorSignInRefused || response.Error.Message != "No login in the vaults available to agents is saved for https://tosunlab-login.example.net." {
		t.Errorf("refusal %+v", response.Error)
	}
	if fixture.agentApprover.getRequestCount() != 0 || fixture.signInHandler.prepareCount != 1 {
		t.Errorf("%d approvals, %d prepares", fixture.agentApprover.getRequestCount(), fixture.signInHandler.prepareCount)
	}

	terminal := fixture.connect(t)
	if code := getErrorCode(terminal.send(t, signInRequest)); code != cliprotocol.ErrorInvalidRequest {
		t.Errorf("a command-line connection sent agent.signin: %q", code)
	}
	harness, _ := startHarness(t)
	if code := getErrorCode(fixture.connectInside(t, harness, false).send(t, signInRequest)); code != cliprotocol.ErrorInvalidRequest {
		t.Errorf("hisar inside a harness sent agent.signin: %q", code)
	}
	if fixture.signInHandler.prepareCount != 1 {
		t.Errorf("%d prepares", fixture.signInHandler.prepareCount)
	}
}
