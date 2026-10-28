package agent

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/approval"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/desktop/appinfo"
)

// fakePhone stands in for the vault backend and the iPhone: the test decides when the phone answers.
type fakePhone struct {
	mutex          sync.Mutex
	isAvailable    bool
	descriptions   []approval.Description
	decisions      []func(answer approval.AcceptedAnswer)
	expiresAt      []time.Time
	finishCount    int
	startedChannel chan struct{}
	posted         []func(bool)
}

func createFakePhone() *fakePhone {
	return &fakePhone{isAvailable: true, startedChannel: make(chan struct{}, 10)}
}

func (phone *fakePhone) IsPhoneApprovalAvailable() bool {
	phone.mutex.Lock()
	defer phone.mutex.Unlock()
	return phone.isAvailable
}

// StartPhoneApproval seals the request as the vault backend does, so a request too long for a phone fails as it would.
func (phone *fakePhone) StartPhoneApproval(description approval.Description, expiresAt time.Time, posted func(bool), decide func(answer approval.AcceptedAnswer)) (func(), error) {
	request := approval.NewRequest("4a1c9e2b-7d3f-4e8a-9b6c-2f5d8e1a3c7b", "MacBook Pro", description, time.Now(), approval.MaximumLifetime)
	if _, err := approval.SealRequest(getPhoneTestKeyset(), "account", request); err != nil {
		return nil, err
	}
	phone.mutex.Lock()
	phone.descriptions = append(phone.descriptions, description)
	phone.decisions = append(phone.decisions, decide)
	phone.expiresAt = append(phone.expiresAt, expiresAt)
	phone.posted = append(phone.posted, posted)
	phone.mutex.Unlock()
	phone.startedChannel <- struct{}{}
	return func() {
		phone.mutex.Lock()
		defer phone.mutex.Unlock()
		phone.finishCount++
	}, nil
}

func TestPhonePostFailureKeepsLocalApprovalAndDeadline(t *testing.T) {
	emitter, phone := createFakeEmitter(), createFakePhone()
	approver := createPhoneApprover(emitter, phone, &fakeBiometric{}, nil, nil)
	request := createAgentRequest(clisocket.AgentRequestRun)
	result := startAgentApproval(approver, request)
	first := waitForApprovalEvent(t, emitter)
	if first.PhoneApprovalState != "pending" {
		t.Fatalf("phone state = %q", first.PhoneApprovalState)
	}
	phone.mutex.Lock()
	posted := phone.posted[0]
	phone.mutex.Unlock()
	posted(false)
	updated := waitForApprovalEvent(t, emitter)
	if updated.PhoneApprovalState != "failed" || updated.TimeoutAt != first.TimeoutAt {
		t.Fatalf("updated event = %+v", updated)
	}
	approver.Respond(updated.RequestID, false, false)
	if answer := waitForAgentResult(t, result); !errors.Is(answer.err, clisocket.ErrDenied) {
		t.Fatalf("local denial = %v", answer.err)
	}
}

var getPhoneTestKeyset = sync.OnceValue(func() *crypt.Keyset {
	keyset, err := crypt.GenerateKeyset()
	if err != nil {
		panic(err)
	}
	return keyset
})

func (phone *fakePhone) waitForStart(t *testing.T) approval.Description {
	t.Helper()
	select {
	case <-phone.startedChannel:
	case <-time.After(5 * time.Second):
		t.Fatal("the request never reached the phone")
	}
	phone.mutex.Lock()
	defer phone.mutex.Unlock()
	return phone.descriptions[len(phone.descriptions)-1]
}

// answer is an answer without the remember choice, as an older phone sends it.
func (phone *fakePhone) answer(isApproved bool) {
	phone.answerRemembering(isApproved, false)
}

func (phone *fakePhone) answerRemembering(isApproved, isRemembered bool) {
	phone.mutex.Lock()
	decide := phone.decisions[len(phone.decisions)-1]
	phone.mutex.Unlock()
	decide(createPhoneAnswer(isApproved, isRemembered))
}

func createPhoneAnswer(isApproved, isRemembered bool) approval.AcceptedAnswer {
	if isApproved {
		return approval.AcceptedAnswer{Decision: approval.DecisionApprove, IsRemembered: isRemembered}
	}
	return approval.AcceptedAnswer{Decision: approval.DecisionDeny}
}

func (phone *fakePhone) getStartAndFinishCount() (int, int) {
	phone.mutex.Lock()
	defer phone.mutex.Unlock()
	return len(phone.decisions), phone.finishCount
}

func createPhoneApprover(emitter *fakeEmitter, phone *fakePhone, biometric Biometric, unlocker Unlocker, processes []appinfo.Process) *Approver {
	config := ApproverConfig{
		Biometric: biometric, Emit: emitter.emit, ShowApprovalWindow: emitter.showWindow, HideApprovalWindow: emitter.hideWindow,
		ResolveRequester: func(int) appinfo.Requester { return createTerminalRequester(processes) },
		HomeDirectory:    testHomeDirectory,
		PhoneApprover:    phone,
		Timeout:          5 * time.Second,
	}
	if unlocker != nil {
		config.Unlocker = unlocker
	}
	return NewApprover(config)
}

func TestPhoneApprovesTheDialog(t *testing.T) {
	emitter := createFakeEmitter()
	phone := createFakePhone()
	biometric := &fakeBiometric{isAvailable: true}
	approver := createPhoneApprover(emitter, phone, biometric, nil, createPushChain())
	resultChannel := startApproval(approver, context.Background(), createTestRequest())
	event := waitForApprovalEvent(t, emitter)
	if !event.IsPhoneApproval {
		t.Errorf("the dialog does not offer the phone: %+v", event)
	}
	description := phone.waitForStart(t)
	want := approval.Description{
		Kind: approval.KindSSH, AppName: "Terminal", Purpose: "push to git.tryhisar.com", Host: "git.tryhisar.com:22",
		CommandLine: "git push origin main", RepositoryPath: "~/src/core", KeyName: "Laptop",
		RememberLabel: "Approve SSH requests from Terminal with this key for up to 1 hour. Any host. Ends when Hisar locks.", IsRememberedByDefault: true,
	}
	if !reflect.DeepEqual(description, want) {
		t.Errorf("phone request = %+v", description)
	}
	phone.answerRemembering(true, true)
	result := waitForResult(t, resultChannel)
	if !result.result.IsAllowed || !result.result.IsRemembered {
		t.Errorf("result = %+v", result)
	}
	if reasons := biometric.listReasons(); len(reasons) != 0 {
		t.Errorf("Touch ID asked after the phone approved: %q", reasons)
	}
	if startCount, finishCount := phone.getStartAndFinishCount(); startCount != 1 || finishCount != 1 {
		t.Errorf("phone requests started %d, finished %d", startCount, finishCount)
	}
	phone.answer(true)

	resultChannel = startApproval(approver, context.Background(), createTestRequest())
	event = waitForApprovalEvent(t, emitter)
	phone.waitForStart(t)
	approver.SetRemembered(event.RequestID, false)
	phone.answerRemembering(true, true)
	if result := waitForResult(t, resultChannel); !result.result.IsAllowed || !result.result.IsRemembered {
		t.Errorf("the phone's choice to remember was ignored: %+v", result)
	}
}

func TestPhoneAnswerNeverTakesTheMacCheckbox(t *testing.T) {
	emitter := createFakeEmitter()
	phone := createFakePhone()
	approver := createPhoneApprover(emitter, phone, &fakeBiometric{isAvailable: true}, nil, createPushChain())
	for _, testCase := range []struct {
		name                 string
		isMacCheckboxChecked bool
		answer               func()
		wantIsRemembered     bool
	}{
		{"an older phone, Mac box checked", true, func() { phone.answer(true) }, false},
		{"once on the phone, Mac box checked", true, func() { phone.answerRemembering(true, false) }, false},
		{"remembered on the phone, Mac box unchecked", false, func() { phone.answerRemembering(true, true) }, true},
		{"denied on the phone with remember", true, func() { phone.answerRemembering(false, true) }, false},
	} {
		resultChannel := startApproval(approver, context.Background(), createTestRequest())
		event := waitForApprovalEvent(t, emitter)
		phone.waitForStart(t)
		approver.SetRemembered(event.RequestID, testCase.isMacCheckboxChecked)
		testCase.answer()
		if result := waitForResult(t, resultChannel); result.result.IsRemembered != testCase.wantIsRemembered {
			t.Errorf("%s: result = %+v", testCase.name, result)
		}
	}
}

func TestPhoneDeniesTheDialog(t *testing.T) {
	emitter := createFakeEmitter()
	phone := createFakePhone()
	approver := createPhoneApprover(emitter, phone, &fakeBiometric{isAvailable: true}, nil, createPushChain())
	resultChannel := startApproval(approver, context.Background(), createTestRequest())
	waitForApprovalEvent(t, emitter)
	phone.waitForStart(t)
	phone.answer(false)
	if result := waitForResult(t, resultChannel); result.result.IsAllowed {
		t.Errorf("a phone's Deny allowed the request: %+v", result)
	}
	if _, finishCount := phone.getStartAndFinishCount(); finishCount != 1 {
		t.Errorf("the phone request was not withdrawn")
	}
}

func TestPhoneAnswerCancelsTouchIDInProgress(t *testing.T) {
	for _, isApproved := range []bool{true, false} {
		emitter := createFakeEmitter()
		phone := createFakePhone()
		biometric := createWaitingBiometric()
		approver := createPhoneApprover(emitter, phone, biometric, nil, createPushChain())
		cancelCount := 0
		approver.config.CancelBiometric = func() {
			cancelCount++
			biometric.resultChannel <- errors.New("Touch ID was cancelled")
		}
		resultChannel := startApproval(approver, context.Background(), createTestRequest())
		event := waitForApprovalEvent(t, emitter)
		phone.waitForStart(t)
		approver.Respond(event.RequestID, true, true)
		biometric.waitForStart(t)
		phone.answer(isApproved)
		if result := waitForResult(t, resultChannel); result.result.IsAllowed != isApproved {
			t.Errorf("phone answered %v while Touch ID waited: %+v", isApproved, result)
		}
		if cancelCount != 1 {
			t.Errorf("Touch ID was cancelled %d times", cancelCount)
		}
		approver.presenter.waitUntilIdle()
		for _, eventName := range emitter.listEventNames() {
			if eventName == ApprovalFailedEventName {
				t.Error("the cancelled Touch ID check was shown as a failure")
			}
		}
	}
}

func TestPhoneIsNotAskedWhenLockedOrUnavailable(t *testing.T) {
	emitter := createFakeEmitter()
	phone := createFakePhone()
	unlocker := &fakeUnlocker{isLocked: true, correctPassword: "master password"}
	approver := createPhoneApprover(emitter, phone, &fakeBiometric{isAvailable: false}, unlocker, createPushChain())
	resultChannel := startApproval(approver, context.Background(), createTestRequest())
	event := waitForApprovalEvent(t, emitter)
	if event.IsPhoneApproval || !event.IsUnlockRequired {
		t.Errorf("a locked Mac offered the phone: %+v", event)
	}
	approver.Respond(event.RequestID, false, false)
	waitForResult(t, resultChannel)

	unlocker.isLocked = false
	phone.isAvailable = false
	resultChannel = startApproval(approver, context.Background(), createTestRequest())
	event = waitForApprovalEvent(t, emitter)
	if event.IsPhoneApproval {
		t.Errorf("the dialog offers an unavailable phone: %+v", event)
	}
	approver.Respond(event.RequestID, false, false)
	waitForResult(t, resultChannel)
	if startCount, _ := phone.getStartAndFinishCount(); startCount != 0 {
		t.Errorf("the phone was asked %d times", startCount)
	}
}

func TestPhoneApprovalOfForwardedAndSigningRequestsIsOnce(t *testing.T) {
	emitter := createFakeEmitter()
	phone := createFakePhone()
	approver := createPhoneApprover(emitter, phone, &fakeBiometric{isAvailable: true}, nil, createSigningChain())
	request := createTestRequest()
	request.SignatureNamespace, request.DestinationHost, request.DestinationFingerprint = "git", "", ""
	resultChannel := startApproval(approver, context.Background(), request)
	waitForApprovalEvent(t, emitter)
	if description := phone.waitForStart(t); description.Purpose != "sign a commit" {
		t.Errorf("signing request on the phone = %+v", description)
	}
	phone.answer(true)
	if result := waitForResult(t, resultChannel); !result.result.IsAllowed || result.result.IsRemembered {
		t.Errorf("a commit signature approved on the phone was remembered: %+v", result)
	}

	forwardedApprover := createPhoneApprover(emitter, phone, &fakeBiometric{isAvailable: true}, nil, append([]appinfo.Process{createSSHProcess("-A", "build.example.com")}, createShellAndTerminal()...))
	forwarded := createTestRequest()
	forwarded.IsForwarded, forwarded.ForwardedFromHost = true, "build.example.com"
	resultChannel = startApproval(forwardedApprover, context.Background(), forwarded)
	waitForApprovalEvent(t, emitter)
	if description := phone.waitForStart(t); !description.IsForwarded || description.ForwardedFromHost != "build.example.com" {
		t.Errorf("forwarded request on the phone = %+v", description)
	}
	phone.answer(true)
	if result := waitForResult(t, resultChannel); !result.result.IsAllowed || result.result.IsRemembered {
		t.Errorf("a forwarded request approved on the phone was remembered: %+v", result)
	}
}

func TestPhoneApprovesCommandLine(t *testing.T) {
	emitter := createFakeEmitter()
	phone := createFakePhone()
	approver := NewApprover(ApproverConfig{
		Biometric: &fakeBiometric{isAvailable: true}, Emit: emitter.emit, ShowApprovalWindow: emitter.showWindow, HideApprovalWindow: emitter.hideWindow,
		ResolveRequester: func(int) appinfo.Requester { return createTerminalRequester(nil) },
		PhoneApprover:    phone,
		Timeout:          5 * time.Second,
	})
	resultChannel := startCommandLineApproval(approver, context.Background())
	waitForApprovalEvent(t, emitter)
	description := phone.waitForStart(t)
	want := approval.Description{
		Kind: approval.KindCommandLine, AppName: "hisar in Terminal", Purpose: "use Hisar", CommandLine: "/Applications/Hisar.app/Contents/Helpers/hisar", TerminalName: "ttys003",
		Allowance: commandLineAllowance, Lifetime: "until 10 minutes after this terminal’s last command, or until Hisar locks",
	}
	if !reflect.DeepEqual(description, want) {
		t.Errorf("command-line request on the phone = %+v", description)
	}
	phone.answer(true)
	if err := waitForCommandLineResult(t, resultChannel); err != nil {
		t.Errorf("approved on the phone: %v", err)
	}
}

func TestLatePhoneAnswerIsIgnored(t *testing.T) {
	emitter := createFakeEmitter()
	phone := createFakePhone()
	approver := createPhoneApprover(emitter, phone, &fakeBiometric{isAvailable: true}, nil, createPushChain())
	resultChannel := startApproval(approver, context.Background(), createTestRequest())
	event := waitForApprovalEvent(t, emitter)
	phone.waitForStart(t)
	approver.Respond(event.RequestID, false, false)
	if result := waitForResult(t, resultChannel); result.result.IsAllowed {
		t.Fatal("Deny on the Mac was overruled")
	}
	phone.answer(true)
	if pendingRequests := approver.ListPendingApprovals(); len(pendingRequests) != 0 {
		t.Errorf("pending after a late answer: %+v", pendingRequests)
	}
}

func TestDialogTheIPhoneCanAnswerWaitsLonger(t *testing.T) {
	emitter := createFakeEmitter()
	phone := createFakePhone()
	approver := NewApprover(ApproverConfig{
		Emit: emitter.emit, ShowApprovalWindow: emitter.showWindow, HideApprovalWindow: emitter.hideWindow,
		PhoneApprover: phone, Timeout: 100 * time.Millisecond, PhoneApprovalTimeout: 3 * time.Second,
	})
	startedAt := time.Now()
	resultChannel := startApproval(approver, context.Background(), createTestRequest())
	event := waitForApprovalEvent(t, emitter)
	phone.waitForStart(t)
	if event.TimeoutMilliseconds != 3000 {
		t.Errorf("the phone dialog's lifetime is %d ms, want 3000", event.TimeoutMilliseconds)
	}
	timeoutAt, err := time.Parse(time.RFC3339, event.TimeoutAt)
	if err != nil || timeoutAt.Before(startedAt.Add(2*time.Second)) {
		t.Errorf("the dialog times out at %q", event.TimeoutAt)
	}
	phone.mutex.Lock()
	expiresAt := phone.expiresAt[0]
	phone.mutex.Unlock()
	if expiresAt.Sub(startedAt) < 2900*time.Millisecond || expiresAt.Sub(startedAt) > 3100*time.Millisecond {
		t.Errorf("the phone's request expires after %v, not with the dialog", expiresAt.Sub(startedAt))
	}
	time.Sleep(500 * time.Millisecond)
	phone.answer(true)
	if result := waitForResult(t, resultChannel); !result.result.IsAllowed {
		t.Errorf("the phone's approval after the Mac-only timeout was lost: %+v", result)
	}

	phone.mutex.Lock()
	phone.isAvailable = false
	phone.mutex.Unlock()
	resultChannel = startApproval(approver, context.Background(), createTestRequest())
	waitForApprovalEvent(t, emitter)
	select {
	case result := <-resultChannel:
		if result.result.IsAllowed {
			t.Error("timed out into an approval")
		}
	case <-time.After(2 * time.Second):
		t.Error("a dialog without the phone kept the longer timeout")
	}
}

// TestRequestTooLongForThePhoneStaysOnTheMac: a command too long to send to a phone is shown whole on the Mac, which
// says the phone cannot approve it and keeps its own deadline.
func TestRequestTooLongForThePhoneStaysOnTheMac(t *testing.T) {
	commandLine := []string{"sh", "-c", strings.Repeat("true; ", 4000) + "curl https://example.invalid/"}
	for _, isAgent := range []bool{true, false} {
		emitter := createFakeEmitter()
		phone := createFakePhone()
		approver := createPhoneApprover(emitter, phone, &fakeBiometric{isAvailable: true}, nil, nil)
		approver.config.PhoneApprovalTimeout = 2 * time.Minute
		var respond func()
		if isAgent {
			request := createAgentRequest(clisocket.AgentRequestRun)
			request.CommandLine = commandLine
			resultChannel := startAgentApproval(approver, request)
			respond = func() { waitForAgentResult(t, resultChannel) }
		} else {
			resultChannel := make(chan error, 1)
			go func() {
				resultChannel <- approver.ApproveCommandLine(context.Background(), createSecretRequest([]string{"Work › Deploy token › password"}, commandLine))
			}()
			respond = func() { <-resultChannel }
		}
		event := waitForApprovalEvent(t, emitter)
		timeoutAt, err := time.Parse(time.RFC3339, event.TimeoutAt)
		if err != nil || event.IsPhoneApproval || !event.IsTooLongForPhone || time.Until(timeoutAt) > time.Minute {
			t.Errorf("agent %v: event = phone %v, too long %v, timeout at %v", isAgent, event.IsPhoneApproval, event.IsTooLongForPhone, event.TimeoutAt)
		}
		if !strings.HasSuffix(event.CommandLine, "curl https://example.invalid/'") {
			t.Errorf("agent %v: the command was cut: %q", isAgent, event.CommandLine[len(event.CommandLine)-40:])
		}
		if startCount, _ := phone.getStartAndFinishCount(); startCount != 0 {
			t.Errorf("agent %v: the phone was asked %d times", isAgent, startCount)
		}
		approver.Respond(event.RequestID, false, false)
		respond()
	}

	emitter := createFakeEmitter()
	phone := createFakePhone()
	approver := createPhoneApprover(emitter, phone, &fakeBiometric{isAvailable: true}, nil, nil)
	request := createAgentRequest(clisocket.AgentRequestRun)
	request.CommandLine = []string{"npm", "test"}
	resultChannel := startAgentApproval(approver, request)
	if event := waitForApprovalEvent(t, emitter); !event.IsPhoneApproval || event.IsTooLongForPhone {
		t.Errorf("a short command: phone %v, too long %v", event.IsPhoneApproval, event.IsTooLongForPhone)
	}
	phone.waitForStart(t)
	phone.answer(false)
	waitForAgentResult(t, resultChannel)
}
