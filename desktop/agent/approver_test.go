package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/agentharness"
	"github.com/tryhisar/hisar/core/sshagent"
	"github.com/tryhisar/hisar/desktop/appinfo"
)

type fakeBiometric struct {
	mutex               sync.Mutex
	isAvailable         bool
	authenticateResults []error
	reasons             []string
}

func (biometric *fakeBiometric) IsAvailable() bool { return biometric.isAvailable }

func (biometric *fakeBiometric) Authenticate(reason string) error {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	biometric.reasons = append(biometric.reasons, reason)
	if len(biometric.authenticateResults) == 0 {
		return nil
	}
	result := biometric.authenticateResults[0]
	biometric.authenticateResults = biometric.authenticateResults[1:]
	return result
}

func (biometric *fakeBiometric) listReasons() []string {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	return append([]string{}, biometric.reasons...)
}

type emittedEvent struct {
	name string
	data any
}

type fakeEmitter struct {
	mutex                sync.Mutex
	events               []emittedEvent
	approvalEventChannel chan ApprovalEvent
	failedEventChannel   chan ApprovalFailedEvent
	showCount            int
	hideCount            int
	shownCompact         []bool
}

func createFakeEmitter() *fakeEmitter {
	return &fakeEmitter{approvalEventChannel: make(chan ApprovalEvent, 10), failedEventChannel: make(chan ApprovalFailedEvent, 10)}
}

func (emitter *fakeEmitter) emit(eventName string, data any) {
	emitter.mutex.Lock()
	emitter.events = append(emitter.events, emittedEvent{name: eventName, data: data})
	emitter.mutex.Unlock()
	switch event := data.(type) {
	case ApprovalEvent:
		emitter.approvalEventChannel <- event
	case ApprovalFailedEvent:
		emitter.failedEventChannel <- event
	}
}

func (emitter *fakeEmitter) showWindow(isCompact bool) {
	emitter.mutex.Lock()
	defer emitter.mutex.Unlock()
	emitter.showCount++
	emitter.shownCompact = append(emitter.shownCompact, isCompact)
}

func (emitter *fakeEmitter) listShownCompact() []bool {
	emitter.mutex.Lock()
	defer emitter.mutex.Unlock()
	return append([]bool{}, emitter.shownCompact...)
}

func (emitter *fakeEmitter) hideWindow() {
	emitter.mutex.Lock()
	defer emitter.mutex.Unlock()
	emitter.hideCount++
}

func (emitter *fakeEmitter) getWindowCount() (int, int) {
	emitter.mutex.Lock()
	defer emitter.mutex.Unlock()
	return emitter.showCount, emitter.hideCount
}

func (emitter *fakeEmitter) listEventNames() []string {
	emitter.mutex.Lock()
	defer emitter.mutex.Unlock()
	names := []string{}
	for _, event := range emitter.events {
		names = append(names, event.name)
	}
	return names
}

func createTestRequest() sshagent.ApprovalRequest {
	return sshagent.ApprovalRequest{
		KeyID:                  "key-1",
		KeyTitle:               "Laptop",
		Fingerprint:            "SHA256:key",
		ClientName:             "ssh",
		ClientPath:             "/usr/bin/ssh",
		ClientPID:              42,
		DestinationHost:        "github.com",
		DestinationFingerprint: "SHA256:host",
		IsRememberable:         true,
	}
}

func createTestApprover(biometric Biometric, emitter *fakeEmitter) *Approver {
	return NewApprover(ApproverConfig{
		Biometric: biometric, Emit: emitter.emit, ShowApprovalWindow: emitter.showWindow, HideApprovalWindow: emitter.hideWindow,
		ResolveRequester: func(pid int) appinfo.Requester {
			if pid != 42 {
				return appinfo.Requester{Kind: appinfo.RequesterKindProcess, Name: "A process without an app"}
			}
			return createTerminalRequester(nil)
		},
	})
}

const terminalBundlePath = "/System/Applications/Utilities/Terminal.app"

func createTerminalRequester(processes []appinfo.Process) appinfo.Requester {
	app := appinfo.App{Name: "Terminal", Path: terminalBundlePath, IconURL: "data:image/png;base64,AA==", Signature: appinfo.Signature{Status: appinfo.SignatureStatusApple}}
	return appinfo.Requester{Kind: appinfo.RequesterKindApp, Name: "Terminal", App: app, Processes: processes, ChainProcesses: processes}
}

const ternBundlePath = "/Applications/Tern.app"

// ompInPaneA and ompInPaneB are two live sessions of omp in Tern; their processes do not run, which the approver never
// checks.
var (
	testAgentSessions = agentharness.NewSessions(nil)
	ompInPaneA        = testAgentSessions.Join(agentharness.Session{Harness: agentharness.Harness{ID: "omp", Name: "omp"}, PID: 1217, StartTime: 1_760_000_000_000_000}, ternBundlePath)
	ompInPaneB        = testAgentSessions.Join(agentharness.Session{Harness: agentharness.Harness{ID: "omp", Name: "omp"}, PID: 1388, StartTime: 1_760_000_100_000_000}, ternBundlePath)
)

func createTernRequester(processes []appinfo.Process) appinfo.Requester {
	app := appinfo.App{Name: "Tern", Path: ternBundlePath, Signature: appinfo.Signature{Status: appinfo.SignatureStatusDeveloper, TeamID: "VTLHGQC72S"}}
	return appinfo.Requester{Kind: appinfo.RequesterKindApp, Name: "Tern", App: app, Processes: processes, ChainProcesses: processes}
}

type approveResult struct {
	result sshagent.ApprovalResult
	err    error
}

func startApproval(approver *Approver, ctx context.Context, request sshagent.ApprovalRequest) chan approveResult {
	resultChannel := make(chan approveResult, 1)
	go func() {
		result, err := approver.Approve(ctx, request)
		resultChannel <- approveResult{result: result, err: err}
	}()
	return resultChannel
}

func waitForResult(t *testing.T, resultChannel chan approveResult) approveResult {
	t.Helper()
	select {
	case result := <-resultChannel:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("approval did not finish")
		return approveResult{}
	}
}

func waitForApprovalEvent(t *testing.T, emitter *fakeEmitter) ApprovalEvent {
	t.Helper()
	select {
	case approvalEvent := <-emitter.approvalEventChannel:
		return approvalEvent
	case <-time.After(5 * time.Second):
		t.Fatal("no approval event")
		return ApprovalEvent{}
	}
}

func TestApproveShowsAppAndAuthorizesWithTouchID(t *testing.T) {
	biometric := &fakeBiometric{isAvailable: true}
	emitter := createFakeEmitter()
	approver := createTestApprover(biometric, emitter)
	resultChannel := startApproval(approver, context.Background(), createTestRequest())
	approvalEvent := waitForApprovalEvent(t, emitter)
	if approvalEvent.AppName != "Terminal" || approvalEvent.AppIconURL == "" || approvalEvent.ClientName != "ssh" || approvalEvent.KeyTitle != "Laptop" ||
		approvalEvent.DestinationHost != "github.com" || !approvalEvent.IsTouchIDApproval || approvalEvent.TimeoutAt == "" {
		t.Errorf("event = %+v", approvalEvent)
	}
	if pendingRequests := approver.ListPendingApprovals(); len(pendingRequests) != 1 || pendingRequests[0].RequestID != approvalEvent.RequestID {
		t.Errorf("pending = %+v", pendingRequests)
	}
	if len(biometric.listReasons()) != 0 {
		t.Error("Touch ID asked before Authorize")
	}
	if approver.Respond("unknown-request", true, false) {
		t.Error("unknown request ID accepted")
	}
	if !approver.Respond(approvalEvent.RequestID, true, false) {
		t.Fatal("Authorize not accepted")
	}
	result := waitForResult(t, resultChannel)
	if result.err != nil || !result.result.IsAllowed || result.result.IsRemembered {
		t.Errorf("result = %+v", result)
	}
	if reasons := biometric.listReasons(); len(reasons) != 1 || reasons[0] != `allow "Terminal" to sign in to github.com` {
		t.Errorf("Touch ID reasons = %q", reasons)
	}
	approver.presenter.waitUntilIdle()
	if showCount, hideCount := emitter.getWindowCount(); showCount != 1 || hideCount != 1 {
		t.Errorf("window shown %d, hidden %d times", showCount, hideCount)
	}
	if names := emitter.listEventNames(); len(names) != 2 || names[1] != ApprovalClosedEventName {
		t.Errorf("events = %v", names)
	}
	if approver.Respond(approvalEvent.RequestID, true, false) || len(approver.ListPendingApprovals()) != 0 {
		t.Error("finished request still pending")
	}
}

func TestApproveRetriesAfterFailedTouchID(t *testing.T) {
	biometric := &fakeBiometric{isAvailable: true, authenticateResults: []error{errors.New("cancelled")}}
	emitter := createFakeEmitter()
	approver := createTestApprover(biometric, emitter)
	resultChannel := startApproval(approver, context.Background(), createTestRequest())
	approvalEvent := waitForApprovalEvent(t, emitter)
	approver.Respond(approvalEvent.RequestID, true, true)
	select {
	case failed := <-emitter.failedEventChannel:
		if failed.RequestID != approvalEvent.RequestID || failed.Message != errTouchIDFailed.Error() {
			t.Errorf("failed event = %+v", failed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no failure event")
	}
	select {
	case result := <-resultChannel:
		t.Fatalf("request finished after a cancelled Touch ID: %+v", result)
	case <-time.After(50 * time.Millisecond):
	}
	if !approver.Respond(approvalEvent.RequestID, true, true) {
		t.Fatal("second Authorize not accepted")
	}
	if result := waitForResult(t, resultChannel); !result.result.IsAllowed || !result.result.IsRemembered {
		t.Errorf("result after retry = %+v", result)
	}
}

func TestApproveDenyAndWithoutTouchID(t *testing.T) {
	emitter := createFakeEmitter()
	approver := createTestApprover(&fakeBiometric{isAvailable: false}, emitter)
	resultChannel := startApproval(approver, context.Background(), createTestRequest())
	approvalEvent := waitForApprovalEvent(t, emitter)
	if approvalEvent.IsTouchIDApproval {
		t.Error("Touch ID offered without Touch ID")
	}
	approver.Respond(approvalEvent.RequestID, false, false)
	if result := waitForResult(t, resultChannel); result.result.IsAllowed {
		t.Error("Deny allowed the request")
	}

	resultChannel = startApproval(approver, context.Background(), createTestRequest())
	approvalEvent = waitForApprovalEvent(t, emitter)
	approver.Respond(approvalEvent.RequestID, true, false)
	if result := waitForResult(t, resultChannel); !result.result.IsAllowed {
		t.Error("Authorize without Touch ID denied the request")
	}
}

func TestApproveTimeoutCancelAndDenyAll(t *testing.T) {
	emitter := createFakeEmitter()
	approver := NewApprover(ApproverConfig{Emit: emitter.emit, Timeout: 50 * time.Millisecond})
	resultChannel := startApproval(approver, context.Background(), createTestRequest())
	approvalEvent := waitForApprovalEvent(t, emitter)
	if approvalEvent.AppName != "ssh" {
		t.Errorf("app name without ResolveRequester = %q", approvalEvent.AppName)
	}
	if result := waitForResult(t, resultChannel); result.result.IsAllowed {
		t.Error("timed out request allowed")
	}

	approver = createTestApprover(&fakeBiometric{}, emitter)
	ctx, cancel := context.WithCancel(context.Background())
	resultChannel = startApproval(approver, ctx, createTestRequest())
	waitForApprovalEvent(t, emitter)
	cancel()
	if result := waitForResult(t, resultChannel); result.result.IsAllowed {
		t.Error("cancelled request allowed")
	}

	firstChannel := startApproval(approver, context.Background(), createTestRequest())
	waitForApprovalEvent(t, emitter)
	secondChannel := startApproval(approver, context.Background(), createTestRequest())
	waitForApprovalEvent(t, emitter)
	approver.presenter.waitUntilIdle()
	_, hideCountBefore := emitter.getWindowCount()
	approver.DenyAll()
	if waitForResult(t, firstChannel).result.IsAllowed || waitForResult(t, secondChannel).result.IsAllowed {
		t.Error("DenyAll allowed a request")
	}
	approver.presenter.waitUntilIdle()
	if _, hideCountAfter := emitter.getWindowCount(); hideCountAfter != hideCountBefore+1 {
		t.Errorf("window hidden %d times for two requests, want once", hideCountAfter-hideCountBefore)
	}
}

func TestApproveWithoutWindowDenies(t *testing.T) {
	approver := NewApprover(ApproverConfig{})
	if result, err := approver.Approve(context.Background(), createTestRequest()); result.IsAllowed || err != nil {
		t.Errorf("no window: %+v %v", result, err)
	}
	if approver.config.Timeout != 60*time.Second {
		t.Errorf("default timeout = %v", approver.config.Timeout)
	}
}

type waitingBiometric struct {
	startedChannel chan struct{}
	resultChannel  chan error
}

func createWaitingBiometric() *waitingBiometric {
	return &waitingBiometric{startedChannel: make(chan struct{}, 10), resultChannel: make(chan error, 10)}
}

func (biometric *waitingBiometric) IsAvailable() bool { return true }

func (biometric *waitingBiometric) Authenticate(reason string) error {
	biometric.startedChannel <- struct{}{}
	return <-biometric.resultChannel
}

func (biometric *waitingBiometric) waitForStart(t *testing.T) {
	t.Helper()
	select {
	case <-biometric.startedChannel:
	case <-time.After(5 * time.Second):
		t.Fatal("Touch ID did not start")
	}
}

func TestDenyCancelsTouchIDInProgress(t *testing.T) {
	biometric := createWaitingBiometric()
	emitter := createFakeEmitter()
	cancelCount := 0
	approver := NewApprover(ApproverConfig{
		Biometric: biometric, Emit: emitter.emit,
		CancelBiometric: func() {
			cancelCount++
			biometric.resultChannel <- errors.New("cancelled")
		},
	})
	resultChannel := startApproval(approver, context.Background(), createTestRequest())
	approvalEvent := waitForApprovalEvent(t, emitter)
	approver.Respond(approvalEvent.RequestID, true, false)
	biometric.waitForStart(t)
	if !approver.Respond(approvalEvent.RequestID, false, false) {
		t.Fatal("Deny during Touch ID not accepted")
	}
	if result := waitForResult(t, resultChannel); result.result.IsAllowed {
		t.Error("Deny during Touch ID allowed the request")
	}
	if cancelCount != 1 {
		t.Errorf("Touch ID cancelled %d times", cancelCount)
	}
	approver.presenter.waitUntilIdle()
	for _, eventName := range emitter.listEventNames() {
		if eventName == ApprovalFailedEventName {
			t.Error("a denied request reported a failed Touch ID")
		}
	}
}

func TestRememberedChangedWhileTouchIDWaits(t *testing.T) {
	biometric := createWaitingBiometric()
	emitter := createFakeEmitter()
	approver := NewApprover(ApproverConfig{Biometric: biometric, Emit: emitter.emit})
	resultChannel := startApproval(approver, context.Background(), createTestRequest())
	approvalEvent := waitForApprovalEvent(t, emitter)
	approver.Respond(approvalEvent.RequestID, true, false)
	biometric.waitForStart(t)
	approver.SetRemembered(approvalEvent.RequestID, true)
	approver.SetRemembered("unknown", false)
	biometric.resultChannel <- nil
	if result := waitForResult(t, resultChannel); !result.result.IsAllowed || !result.result.IsRemembered {
		t.Errorf("result = %+v", result)
	}
}
