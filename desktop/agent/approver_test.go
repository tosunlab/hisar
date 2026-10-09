package agent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/sshagent"
	"github.com/tryhisar/hisar/desktop/appinfo"
)

type fakeBiometric struct {
	mutex                 sync.Mutex
	isAvailable           bool
	allAuthenticateResult []error
	allReason             []string
}

func (biometric *fakeBiometric) IsAvailable() bool { return biometric.isAvailable }

func (biometric *fakeBiometric) Authenticate(reason string) error {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	biometric.allReason = append(biometric.allReason, reason)
	if len(biometric.allAuthenticateResult) == 0 {
		return nil
	}
	result := biometric.allAuthenticateResult[0]
	biometric.allAuthenticateResult = biometric.allAuthenticateResult[1:]
	return result
}

func (biometric *fakeBiometric) listAllReason() []string {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	return append([]string{}, biometric.allReason...)
}

type emittedEvent struct {
	name string
	data any
}

type fakeEmitter struct {
	mutex                sync.Mutex
	allEvent             []emittedEvent
	approvalEventChannel chan ApprovalEvent
	failedEventChannel   chan ApprovalFailedEvent
	showCount            int
	hideCount            int
}

func createFakeEmitter() *fakeEmitter {
	return &fakeEmitter{approvalEventChannel: make(chan ApprovalEvent, 10), failedEventChannel: make(chan ApprovalFailedEvent, 10)}
}

func (emitter *fakeEmitter) emit(eventName string, data any) {
	emitter.mutex.Lock()
	emitter.allEvent = append(emitter.allEvent, emittedEvent{name: eventName, data: data})
	emitter.mutex.Unlock()
	switch event := data.(type) {
	case ApprovalEvent:
		emitter.approvalEventChannel <- event
	case ApprovalFailedEvent:
		emitter.failedEventChannel <- event
	}
}

func (emitter *fakeEmitter) showWindow() {
	emitter.mutex.Lock()
	defer emitter.mutex.Unlock()
	emitter.showCount++
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

func (emitter *fakeEmitter) listAllEventName() []string {
	emitter.mutex.Lock()
	defer emitter.mutex.Unlock()
	allName := []string{}
	for _, event := range emitter.allEvent {
		allName = append(allName, event.name)
	}
	return allName
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
	}
}

func createTestApprover(biometric Biometric, emitter *fakeEmitter) *Approver {
	return CreateApprover(ApproverConfig{
		Biometric: biometric, Emit: emitter.emit, ShowApprovalWindow: emitter.showWindow, HideApprovalWindow: emitter.hideWindow,
		FindApp: func(pid int) (appinfo.App, bool) {
			if pid != 42 {
				return appinfo.App{}, false
			}
			return appinfo.App{Name: "Terminal", Path: "/System/Applications/Utilities/Terminal.app", IconURL: "data:image/png;base64,AA=="}, true
		},
	})
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
	if allPending := approver.ListAllPendingApproval(); len(allPending) != 1 || allPending[0].RequestID != approvalEvent.RequestID {
		t.Errorf("pending = %+v", allPending)
	}
	if len(biometric.listAllReason()) != 0 {
		t.Error("Touch ID asked before Authorize")
	}
	if approver.Respond("unknown-request", true, false) {
		t.Error("unknown request ID accepted")
	}
	if !approver.Respond(approvalEvent.RequestID, true, false) {
		t.Fatal("Authorize not accepted")
	}
	result := waitForResult(t, resultChannel)
	if result.err != nil || !result.result.IsAllowed || result.result.IsForAllClient {
		t.Errorf("result = %+v", result)
	}
	if allReason := biometric.listAllReason(); len(allReason) != 1 || allReason[0] != `allow "Terminal" to use the SSH key "Laptop" for github.com` {
		t.Errorf("Touch ID reasons = %q", allReason)
	}
	if showCount, hideCount := emitter.getWindowCount(); showCount != 1 || hideCount != 1 {
		t.Errorf("window shown %d, hidden %d times", showCount, hideCount)
	}
	if allName := emitter.listAllEventName(); len(allName) != 2 || allName[1] != ApprovalClosedEventName {
		t.Errorf("events = %v", allName)
	}
	if approver.Respond(approvalEvent.RequestID, true, false) || len(approver.ListAllPendingApproval()) != 0 {
		t.Error("finished request still pending")
	}
}

func TestApproveRetriesAfterFailedTouchID(t *testing.T) {
	biometric := &fakeBiometric{isAvailable: true, allAuthenticateResult: []error{errors.New("cancelled")}}
	emitter := createFakeEmitter()
	approver := createTestApprover(biometric, emitter)
	resultChannel := startApproval(approver, context.Background(), createTestRequest())
	approvalEvent := waitForApprovalEvent(t, emitter)
	approver.Respond(approvalEvent.RequestID, true, true)
	select {
	case failed := <-emitter.failedEventChannel:
		if failed.RequestID != approvalEvent.RequestID || !strings.Contains(failed.Message, "Touch ID") {
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
	if result := waitForResult(t, resultChannel); !result.result.IsAllowed || !result.result.IsForAllClient {
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
	approver := CreateApprover(ApproverConfig{Emit: emitter.emit, Timeout: 50 * time.Millisecond})
	resultChannel := startApproval(approver, context.Background(), createTestRequest())
	approvalEvent := waitForApprovalEvent(t, emitter)
	if approvalEvent.AppName != "ssh" {
		t.Errorf("app name without FindApp = %q", approvalEvent.AppName)
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
	_, hideCountBefore := emitter.getWindowCount()
	approver.DenyAll()
	if waitForResult(t, firstChannel).result.IsAllowed || waitForResult(t, secondChannel).result.IsAllowed {
		t.Error("DenyAll allowed a request")
	}
	if _, hideCountAfter := emitter.getWindowCount(); hideCountAfter != hideCountBefore+1 {
		t.Errorf("window hidden %d times for two requests, want once", hideCountAfter-hideCountBefore)
	}
}

func TestApproveWithoutWindowDenies(t *testing.T) {
	approver := CreateApprover(ApproverConfig{})
	if result, err := approver.Approve(context.Background(), createTestRequest()); result.IsAllowed || err != nil {
		t.Errorf("no window: %+v %v", result, err)
	}
	if approver.config.Timeout != 60*time.Second {
		t.Errorf("default timeout = %v", approver.config.Timeout)
	}
}

func TestBuildApprovalReason(t *testing.T) {
	request := createTestRequest()
	request.DestinationHost, request.DestinationFingerprint = "", ""
	request.IsForwarded = true
	reason := buildApprovalReason("Terminal", request)
	if !strings.Contains(reason, "for SHA256:key") || !strings.HasSuffix(reason, "through a forwarded agent") {
		t.Errorf("reason = %q", reason)
	}
}
