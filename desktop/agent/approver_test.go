package agent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/sshagent"
)

type fakeBiometric struct {
	mutex             sync.Mutex
	isAvailable       bool
	authenticateError error
	allReason         []string
}

func (biometric *fakeBiometric) IsAvailable() bool { return biometric.isAvailable }

func (biometric *fakeBiometric) Authenticate(reason string) error {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	biometric.allReason = append(biometric.allReason, reason)
	return biometric.authenticateError
}

type emittedEvent struct {
	name string
	data any
}

type fakeEmitter struct {
	mutex                sync.Mutex
	allEvent             []emittedEvent
	approvalEventChannel chan ApprovalEvent
	showCount            int
}

func createFakeEmitter() *fakeEmitter {
	return &fakeEmitter{approvalEventChannel: make(chan ApprovalEvent, 10)}
}

func (emitter *fakeEmitter) emit(eventName string, data any) {
	emitter.mutex.Lock()
	emitter.allEvent = append(emitter.allEvent, emittedEvent{name: eventName, data: data})
	emitter.mutex.Unlock()
	if approvalEvent, ok := data.(ApprovalEvent); ok {
		emitter.approvalEventChannel <- approvalEvent
	}
}

func (emitter *fakeEmitter) showWindow() {
	emitter.mutex.Lock()
	defer emitter.mutex.Unlock()
	emitter.showCount++
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

type approveResult struct {
	isAllowed bool
	err       error
}

func startApproval(approver *Approver, ctx context.Context) chan approveResult {
	resultChannel := make(chan approveResult, 1)
	go func() {
		isAllowed, err := approver.Approve(ctx, createTestRequest())
		resultChannel <- approveResult{isAllowed: isAllowed, err: err}
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

func TestApproveWithTouchID(t *testing.T) {
	biometric := &fakeBiometric{isAvailable: true}
	emitter := createFakeEmitter()
	approver := CreateApprover(ApproverConfig{Biometric: biometric, Emit: emitter.emit, ShowWindow: emitter.showWindow})
	if !approver.IsTouchIDApproval() {
		t.Fatal("Touch ID approval not used")
	}

	isAllowed, err := approver.Approve(context.Background(), createTestRequest())
	if err != nil || !isAllowed {
		t.Fatalf("Touch ID passed: %v %v", isAllowed, err)
	}
	wantReason := `allow "ssh" to use the SSH key "Laptop" for github.com`
	if biometric.allReason[0] != wantReason {
		t.Errorf("reason = %q, want %q", biometric.allReason[0], wantReason)
	}

	biometric.authenticateError = errors.New("Touch ID was cancelled")
	request := createTestRequest()
	request.DestinationHost = ""
	request.IsForwarded = true
	isAllowed, err = approver.Approve(context.Background(), request)
	if err != nil || isAllowed {
		t.Errorf("cancelled Touch ID: %v %v", isAllowed, err)
	}
	if reason := biometric.allReason[1]; !strings.Contains(reason, "for SHA256:host") || !strings.HasSuffix(reason, "through a forwarded agent") {
		t.Errorf("reason without host = %q", reason)
	}
	if len(emitter.listAllEventName()) != 0 || emitter.showCount != 0 {
		t.Error("Touch ID approval used the in-app prompt")
	}
}

func TestBuildApprovalReasonFallsBackToKeyFingerprint(t *testing.T) {
	request := createTestRequest()
	request.DestinationHost, request.DestinationFingerprint = "", ""
	if reason := buildApprovalReason(request); !strings.HasSuffix(reason, "for SHA256:key") {
		t.Errorf("reason = %q", reason)
	}
}

func TestApproveInApp(t *testing.T) {
	for _, isAllowed := range []bool{true, false} {
		emitter := createFakeEmitter()
		approver := CreateApprover(ApproverConfig{Biometric: &fakeBiometric{isAvailable: false}, Emit: emitter.emit, ShowWindow: emitter.showWindow})
		if approver.IsTouchIDApproval() {
			t.Fatal("Touch ID approval used without Touch ID")
		}
		resultChannel := startApproval(approver, context.Background())
		approvalEvent := waitForApprovalEvent(t, emitter)
		if approvalEvent.RequestID == "" || approvalEvent.ClientName != "ssh" || approvalEvent.KeyTitle != "Laptop" || approvalEvent.DestinationHost != "github.com" || approvalEvent.ClientPath != "/usr/bin/ssh" {
			t.Errorf("event = %+v", approvalEvent)
		}
		if approver.Respond("unknown-request", !isAllowed) {
			t.Error("unknown request ID accepted")
		}
		if !approver.Respond(approvalEvent.RequestID, isAllowed) {
			t.Fatal("response not accepted")
		}
		result := waitForResult(t, resultChannel)
		if result.err != nil || result.isAllowed != isAllowed {
			t.Errorf("response %v: got %+v", isAllowed, result)
		}
		if approver.Respond(approvalEvent.RequestID, true) {
			t.Error("second response accepted")
		}
		if emitter.showCount != 1 {
			t.Errorf("window shown %d times", emitter.showCount)
		}
		if allName := emitter.listAllEventName(); len(allName) != 1 || allName[0] != ApprovalEventName {
			t.Errorf("events = %v", allName)
		}
	}
}

func TestApproveInAppTimeout(t *testing.T) {
	emitter := createFakeEmitter()
	approver := CreateApprover(ApproverConfig{Emit: emitter.emit, Timeout: 50 * time.Millisecond})
	resultChannel := startApproval(approver, context.Background())
	approvalEvent := waitForApprovalEvent(t, emitter)
	result := waitForResult(t, resultChannel)
	if result.err != nil || result.isAllowed {
		t.Errorf("timeout: %+v", result)
	}
	if approver.Respond(approvalEvent.RequestID, true) {
		t.Error("response after timeout accepted")
	}
	allName := emitter.listAllEventName()
	if len(allName) != 2 || allName[1] != ApprovalClosedEventName {
		t.Fatalf("events = %v", allName)
	}
	if closedID := emitter.allEvent[1].data; closedID != approvalEvent.RequestID {
		t.Errorf("closed event for %v, want %s", closedID, approvalEvent.RequestID)
	}
}

func TestApproveInAppCancelled(t *testing.T) {
	emitter := createFakeEmitter()
	approver := CreateApprover(ApproverConfig{Emit: emitter.emit})
	ctx, cancel := context.WithCancel(context.Background())
	resultChannel := startApproval(approver, ctx)
	waitForApprovalEvent(t, emitter)
	cancel()
	if result := waitForResult(t, resultChannel); result.isAllowed {
		t.Error("cancelled request allowed")
	}
}

func TestApproveWithoutPromptDenies(t *testing.T) {
	approver := CreateApprover(ApproverConfig{})
	if isAllowed, err := approver.Approve(context.Background(), createTestRequest()); isAllowed || err != nil {
		t.Errorf("no prompt: %v %v", isAllowed, err)
	}
}

func TestApproveInAppDefaultTimeout(t *testing.T) {
	if approver := CreateApprover(ApproverConfig{}); approver.config.Timeout != 60*time.Second {
		t.Errorf("timeout = %v", approver.config.Timeout)
	}
}
