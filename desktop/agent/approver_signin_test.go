package agent

import (
	"errors"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/agentharness"
	"github.com/tryhisar/hisar/core/clisocket"
)

type promptingBiometric struct {
	reasonChannel chan string
	resultChannel chan error
}

func createPromptingBiometric() *promptingBiometric {
	return &promptingBiometric{reasonChannel: make(chan string, 10), resultChannel: make(chan error, 10)}
}

func (biometric *promptingBiometric) IsAvailable() bool { return true }

func (biometric *promptingBiometric) Authenticate(reason string) error {
	biometric.reasonChannel <- reason
	return <-biometric.resultChannel
}

func (biometric *promptingBiometric) waitForReason(t *testing.T) string {
	t.Helper()
	select {
	case reason := <-biometric.reasonChannel:
		return reason
	case <-time.After(5 * time.Second):
		t.Fatal("Touch ID did not start")
		return ""
	}
}

func createSignInRequest(logins ...clisocket.AgentLogin) clisocket.AgentApprovalRequest {
	request := createAgentRequest(clisocket.AgentRequestSignIn)
	request.Session = &agentharness.Session{Harness: agentharness.Harness{ID: "hermes", Name: "Hermes"}, PID: 1226, StartTime: 100}
	request.GrantDurations = []clisocket.AgentGrantDuration{clisocket.AgentGrantOnce, clisocket.AgentGrantHour, clisocket.AgentGrantDay, clisocket.AgentGrantAlways}
	request.SignIn = &clisocket.AgentSignInApproval{Website: "https://tosunlab.com", BrowserName: "Google Chrome", Logins: logins}
	return request
}

var (
	labLogin      = clisocket.AgentLogin{AgentSecret: clisocket.AgentSecret{VaultID: "v", ItemID: "tosunlab", VaultName: "Agents", ItemTitle: "Tosun Lab", Origin: "https://tosunlab.com"}, Username: "user@example.com"}
	labAdminLogin = clisocket.AgentLogin{AgentSecret: clisocket.AgentSecret{VaultID: "v", ItemID: "admin", VaultName: "Agents", ItemTitle: "Tosun Lab admin", Origin: "https://tosunlab.com"}, Username: "admin@example.com"}
)

func TestAgentApprovalWithoutTouchIDNeedsThePassword(t *testing.T) {
	for _, request := range []clisocket.AgentApprovalRequest{createAgentRequest(clisocket.AgentRequestConnect), createAgentRequest(clisocket.AgentRequestRun), createSignInRequest(labLogin)} {
		emitter := createFakeEmitter()
		unlocker := &fakeUnlocker{correctPassword: "right"}
		approver := createCommandLineApprover(&fakeBiometric{isAvailable: false}, emitter, unlocker, 0)
		resultChannel := startAgentApproval(approver, request)
		event := waitForApprovalEvent(t, emitter)
		if !event.RequiresPresence || event.IsTouchIDApproval {
			t.Errorf("%s: event %+v", request.Kind, event)
		}
		approver.Respond(event.RequestID, true, false)
		if failed := waitForFailedEvent(t, emitter); failed.Message != "Enter your master password to approve." {
			t.Errorf("%s: a click without Touch ID: %+v", request.Kind, failed)
		}
		select {
		case result := <-resultChannel:
			t.Fatalf("%s: a click approved %+v", request.Kind, result)
		case <-time.After(50 * time.Millisecond):
		}
		approver.RespondWithPassword(event.RequestID, "wrong", false)
		waitForFailedEvent(t, emitter)
		approver.RespondWithPassword(event.RequestID, "right", false)
		if result := waitForAgentResult(t, resultChannel); result.err != nil {
			t.Errorf("%s: the password did not approve: %v", request.Kind, result.err)
		}
	}
}

func TestSignInDialogShowsWhereHisarSignsInAndTheLogin(t *testing.T) {
	emitter := createFakeEmitter()
	biometric := &fakeBiometric{isAvailable: true}
	approver := createCommandLineApprover(biometric, emitter, &fakeUnlocker{}, 0)
	resultChannel := startAgentApproval(approver, createSignInRequest(labLogin))
	event := waitForApprovalEvent(t, emitter)
	if event.Purpose != "sign-in" || event.Action != "sign in to https://tosunlab.com" || event.Website != "https://tosunlab.com" || event.BrowserName != "Google Chrome" ||
		len(event.Logins) != 1 || event.Logins[0] != (ApprovalLogin{VaultName: "Agents", Title: "Tosun Lab", Username: "user@example.com"}) || event.LoginIndex != 0 ||
		event.GrantDuration != "once" || len(event.GrantDurations) != 4 || event.IsRememberable || !event.RequiresPresence {
		t.Errorf("event %+v", event)
	}
	approver.SetAgentGrantDuration(event.RequestID, "day")
	approver.SetSignInLogin(event.RequestID, 3)
	approver.Respond(event.RequestID, true, false)
	if result := waitForAgentResult(t, resultChannel); result.err != nil || result.result.Duration != clisocket.AgentGrantDay || result.result.LoginIndex != 0 {
		t.Errorf("result %+v", result)
	}
	if reasons := biometric.listReasons(); len(reasons) != 1 || reasons[0] != `allow "Hermes in Terminal" to sign in to https://tosunlab.com as user@example.com for 1 day` {
		t.Errorf("Touch ID reasons %q", reasons)
	}
}

func TestChangingTheChoiceRestartsTouchID(t *testing.T) {
	emitter := createFakeEmitter()
	biometric := createPromptingBiometric()
	cancelCount := 0
	approver := NewApprover(ApproverConfig{
		Biometric: biometric, Emit: emitter.emit, Unlocker: &fakeUnlocker{},
		CancelBiometric: func() {
			cancelCount++
			biometric.resultChannel <- errors.New("cancelled")
		},
	})
	resultChannel := startAgentApproval(approver, createSignInRequest(labLogin, labAdminLogin))
	event := waitForApprovalEvent(t, emitter)
	approver.Respond(event.RequestID, true, false)
	if reason := biometric.waitForReason(t); reason != `allow "Hermes" to sign in to https://tosunlab.com as user@example.com only this time` {
		t.Errorf("first reason %q", reason)
	}
	approver.SetAgentGrantDuration(event.RequestID, "always")
	if reason := biometric.waitForReason(t); reason != `allow "Hermes" to sign in to https://tosunlab.com as user@example.com until you revoke it` {
		t.Errorf("reason after the menu changed %q", reason)
	}
	approver.SetSignInLogin(event.RequestID, 1)
	if reason := biometric.waitForReason(t); reason != `allow "Hermes" to sign in to https://tosunlab.com as admin@example.com until you revoke it` {
		t.Errorf("reason after the login changed %q", reason)
	}
	approver.SetAgentGrantDuration(event.RequestID, "always")
	biometric.resultChannel <- nil
	if result := waitForAgentResult(t, resultChannel); result.err != nil || result.result.Duration != clisocket.AgentGrantAlways || result.result.LoginIndex != 1 {
		t.Errorf("result %+v", result)
	}
	if cancelCount != 2 {
		t.Errorf("Touch ID cancelled %d times; want once per change", cancelCount)
	}
}

func TestTouchIDThatEndsAfterAChangeAsksAgain(t *testing.T) {
	emitter := createFakeEmitter()
	biometric := createPromptingBiometric()
	approver := NewApprover(ApproverConfig{Biometric: biometric, Emit: emitter.emit, Unlocker: &fakeUnlocker{}})
	resultChannel := startAgentApproval(approver, createSignInRequest(labLogin))
	event := waitForApprovalEvent(t, emitter)
	approver.Respond(event.RequestID, true, false)
	biometric.waitForReason(t)
	approver.SetAgentGrantDuration(event.RequestID, "hour")
	biometric.resultChannel <- nil
	if reason := biometric.waitForReason(t); reason != `allow "Hermes" to sign in to https://tosunlab.com as user@example.com for 1 hour` {
		t.Errorf("second reason %q", reason)
	}
	biometric.resultChannel <- nil
	if result := waitForAgentResult(t, resultChannel); result.err != nil || result.result.Duration != clisocket.AgentGrantHour {
		t.Errorf("result %+v", result)
	}
}

func TestAcceptedDenyIsFinal(t *testing.T) {
	emitter := createFakeEmitter()
	biometric := createPromptingBiometric()
	approver := NewApprover(ApproverConfig{Biometric: biometric, Emit: emitter.emit, Unlocker: &fakeUnlocker{}})
	resultChannel := startAgentApproval(approver, createSignInRequest(labLogin))
	event := waitForApprovalEvent(t, emitter)
	approver.Respond(event.RequestID, true, false)
	biometric.waitForReason(t)
	if !approver.Respond(event.RequestID, false, false) {
		t.Fatal("Deny was not accepted")
	}
	biometric.resultChannel <- nil
	if result := waitForAgentResult(t, resultChannel); result.err != clisocket.ErrDenied {
		t.Errorf("Touch ID after Deny: %+v", result)
	}

	phone := createFakePhone()
	approver = createPhoneApprover(emitter, phone, biometric, &fakeUnlocker{}, nil)
	resultChannel = startAgentApproval(approver, createSignInRequest(labLogin))
	event = waitForApprovalEvent(t, emitter)
	phone.waitForStart(t)
	approver.Respond(event.RequestID, true, false)
	biometric.waitForReason(t)
	if !approver.Respond(event.RequestID, false, false) || approver.RespondWithPassword(event.RequestID, "right", false) {
		t.Fatal("Deny was not final")
	}
	phone.answer(true)
	if result := waitForAgentResult(t, resultChannel); result.err != clisocket.ErrDenied {
		t.Errorf("the phone after Deny: %+v", result)
	}
	biometric.resultChannel <- nil
}

func TestSignInGoesToThePhoneOnlyWithOneLogin(t *testing.T) {
	emitter := createFakeEmitter()
	phone := createFakePhone()
	approver := createPhoneApprover(emitter, phone, &fakeBiometric{isAvailable: true}, nil, nil)
	resultChannel := startAgentApproval(approver, createSignInRequest(labLogin))
	event := waitForApprovalEvent(t, emitter)
	description := phone.waitForStart(t)
	if !event.IsPhoneApproval || description.Purpose != "sign in to https://tosunlab.com as user@example.com (Agents › Tosun Lab)" || description.RememberLabel != "" {
		t.Errorf("phone request %+v", description)
	}
	approver.SetAgentGrantDuration(event.RequestID, "always")
	phone.answerRemembering(true, true)
	if result := waitForAgentResult(t, resultChannel); result.err != nil || result.result.Duration != clisocket.AgentGrantOnce {
		t.Errorf("a phone approval kept the Mac's menu: %+v", result)
	}

	resultChannel = startAgentApproval(approver, createSignInRequest(labLogin, labAdminLogin))
	if event := waitForApprovalEvent(t, emitter); event.IsPhoneApproval {
		t.Errorf("a choice of logins went to the phone: %+v", event)
	}
	if starts, _ := phone.getStartAndFinishCount(); starts != 1 {
		t.Errorf("the phone was asked %d times", starts)
	}
	approver.DenyAll()
	waitForAgentResult(t, resultChannel)
}

func TestLockedSignInUnlocksWithAReasonNamingTheSignIn(t *testing.T) {
	emitter := createFakeEmitter()
	biometric := &fakeBiometric{isAvailable: true}
	unlocker := &fakeUnlocker{isLocked: true, isTouchIDUnlockReady: true}
	approver := createCommandLineApprover(biometric, emitter, unlocker, 0)
	request := createSignInRequest()
	request.SignIn.IsLocked = true
	resultChannel := startAgentApproval(approver, request)
	event := waitForApprovalEvent(t, emitter)
	if !event.IsUnlockRequired || len(event.Logins) != 0 {
		t.Errorf("event %+v", event)
	}
	approver.SetAgentGrantDuration(event.RequestID, "hour")
	approver.Respond(event.RequestID, true, false)
	if result := waitForAgentResult(t, resultChannel); result.err != nil || result.result.Duration != clisocket.AgentGrantHour || unlocker.IsLocked() {
		t.Errorf("result %+v", result)
	}
	if reasons := biometric.listReasons(); len(reasons) != 1 || reasons[0] != `unlock Hisar and allow "Hermes in Terminal" to sign in to https://tosunlab.com for 1 hour` {
		t.Errorf("Touch ID reasons %q", reasons)
	}
}

func TestAgentTouchIDWaitsForPreparedUnlockBeforeRestartingChoice(t *testing.T) {
	emitter := createFakeEmitter()
	biometric := createPromptingBiometric()
	unlocker := &fakeUnlocker{}
	approver := NewApprover(ApproverConfig{Biometric: biometric, Emit: emitter.emit, Unlocker: unlocker})
	unlocker.touchIDMutex.Lock()
	resultChannel := startAgentApproval(approver, createSignInRequest(labLogin))
	event := waitForApprovalEvent(t, emitter)
	approver.Respond(event.RequestID, true, false)
	select {
	case reason := <-biometric.reasonChannel:
		unlocker.touchIDMutex.Unlock()
		t.Fatalf("the agent took a prepared unlock's native check: %s", reason)
	case <-time.After(200 * time.Millisecond):
	}
	unlocker.touchIDMutex.Unlock()
	biometric.waitForReason(t)
	approver.SetAgentGrantDuration(event.RequestID, "day")
	biometric.resultChannel <- nil
	if reason := biometric.waitForReason(t); reason != `allow "Hermes" to sign in to https://tosunlab.com as user@example.com for 1 day` {
		t.Errorf("the restarted check did not name the new choice: %s", reason)
	}
	biometric.resultChannel <- nil
	if result := waitForAgentResult(t, resultChannel); result.err != nil || result.result.Duration != clisocket.AgentGrantDay {
		t.Errorf("result %+v", result)
	}
}

type preparingUnlocker struct {
	*fakeUnlocker
	prepareCount int
}

func (unlocker *preparingUnlocker) UnlockWithTouchID(authenticate func(string) error) error {
	unlocker.prepareCount++
	return unlocker.fakeUnlocker.UnlockWithTouchID(authenticate)
}

func TestLockedAgentChoiceRestartsProtectedKeyPreparation(t *testing.T) {
	emitter := createFakeEmitter()
	biometric := createPromptingBiometric()
	unlocker := &preparingUnlocker{fakeUnlocker: &fakeUnlocker{isLocked: true, isTouchIDUnlockReady: true}}
	approver := NewApprover(ApproverConfig{Biometric: biometric, Emit: emitter.emit, Unlocker: unlocker})
	request := createSignInRequest()
	request.SignIn.IsLocked = true
	resultChannel := startAgentApproval(approver, request)
	event := waitForApprovalEvent(t, emitter)
	approver.Respond(event.RequestID, true, false)
	biometric.waitForReason(t)
	approver.SetAgentGrantDuration(event.RequestID, "day")
	biometric.resultChannel <- nil
	if reason := biometric.waitForReason(t); reason != `unlock Hisar and allow "Hermes" to sign in to https://tosunlab.com for 1 day` {
		t.Errorf("the restarted unlock did not name the new choice: %s", reason)
	}
	biometric.resultChannel <- nil
	if result := waitForAgentResult(t, resultChannel); result.err != nil || result.result.Duration != clisocket.AgentGrantDay {
		t.Errorf("result %+v", result)
	}
	if unlocker.prepareCount != 2 {
		t.Errorf("the two native checks shared one key preparation: %d preparations", unlocker.prepareCount)
	}
}
