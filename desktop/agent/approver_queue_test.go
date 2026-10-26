package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// stuckBiometric waits like a Touch ID check nobody answers; a cancel sent while no check runs is lost, as with LAContext.
type stuckBiometric struct {
	mutex          sync.Mutex
	isActive       bool
	cancelChannel  chan struct{}
	startedChannel chan struct{}
	cancelCount    int
}

func createStuckBiometric() *stuckBiometric {
	return &stuckBiometric{startedChannel: make(chan struct{}, 10)}
}

func (biometric *stuckBiometric) IsAvailable() bool { return true }

func (biometric *stuckBiometric) Authenticate(string) error {
	biometric.mutex.Lock()
	biometric.isActive = true
	cancelChannel := make(chan struct{})
	biometric.cancelChannel = cancelChannel
	biometric.mutex.Unlock()
	biometric.startedChannel <- struct{}{}
	<-cancelChannel
	return errors.New("Touch ID was cancelled")
}

func (biometric *stuckBiometric) cancel() {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	biometric.cancelCount++
	if biometric.isActive {
		biometric.isActive = false
		close(biometric.cancelChannel)
	}
}

func (biometric *stuckBiometric) isWaiting() bool {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	return biometric.isActive
}

func (biometric *stuckBiometric) waitForStart(t *testing.T) {
	t.Helper()
	select {
	case <-biometric.startedChannel:
	case <-time.After(5 * time.Second):
		t.Fatal("Touch ID did not start")
	}
}

func waitForResultWithin(t *testing.T, resultChannel chan approveResult, limit time.Duration, name string) approveResult {
	t.Helper()
	select {
	case result := <-resultChannel:
		return result
	case <-time.After(limit):
		t.Fatalf("%s never ended", name)
		return approveResult{}
	}
}

func waitUntil(t *testing.T, condition func() bool, description string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal(description)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (phone *fakePhone) answerRequest(index int, isApproved bool) {
	phone.mutex.Lock()
	decide := phone.decisions[index]
	phone.mutex.Unlock()
	decide(createPhoneAnswer(isApproved, false))
}

func TestDeadlineEndsATouchIDCheckNobodyAnswers(t *testing.T) {
	biometric := createStuckBiometric()
	emitter := createFakeEmitter()
	phone := createFakePhone()
	phone.isAvailable = false
	approver := NewApprover(ApproverConfig{
		Biometric: biometric, CancelBiometric: biometric.cancel, Emit: emitter.emit, ShowApprovalWindow: emitter.showWindow,
		PhoneApprover: phone, Timeout: 200 * time.Millisecond,
	})
	resultChannel := startApproval(approver, context.Background(), createTestRequest())
	event := waitForApprovalEvent(t, emitter)
	approver.Respond(event.RequestID, true, false)
	biometric.waitForStart(t)
	if result := waitForResultWithin(t, resultChannel, 3*time.Second, "a request whose Touch ID check nobody answers"); result.result.IsAllowed {
		t.Errorf("result = %+v", result)
	}
	waitUntil(t, func() bool { return !biometric.isWaiting() }, "the Touch ID check was left open after the deadline")
	if len(approver.ListPendingApprovals()) != 0 {
		t.Error("the request is still pending")
	}
	if startCount, _ := phone.getStartAndFinishCount(); startCount != 0 {
		t.Errorf("an unavailable phone was asked %d times", startCount)
	}
}

func TestDeadlineDeniesBeforeHidingTheNotice(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		emitter := createFakeEmitter()
		approver := NewApprover(ApproverConfig{
			Emit: emitter.emit, ShowApprovalWindow: emitter.showWindow, HideApprovalWindow: emitter.hideWindow,
		})
		resultChannel := startApproval(approver, context.Background(), createTestRequest())
		event := waitForApprovalEvent(t, emitter)
		time.Sleep(time.Minute)
		synctest.Wait()
		if result := waitForResult(t, resultChannel); result.result.IsAllowed {
			t.Fatal("the expired request was approved")
		}
		if len(approver.ListPendingApprovals()) != 0 || approver.Respond(event.RequestID, true, true) {
			t.Fatal("the expired request can still be approved")
		}
		time.Sleep(2999 * time.Millisecond)
		synctest.Wait()
		if _, hidden := emitter.getWindowCount(); hidden != 0 {
			t.Fatal("the window hid before the denial could be read")
		}
		time.Sleep(time.Millisecond)
		synctest.Wait()
		if _, hidden := emitter.getWindowCount(); hidden != 1 {
			t.Fatalf("window hidden %d times, want once after the notice", hidden)
		}
	})
}

func TestExpiredNoticeDoesNotHideANewRequest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		emitter := createFakeEmitter()
		approver := NewApprover(ApproverConfig{
			Emit: emitter.emit, ShowApprovalWindow: emitter.showWindow, HideApprovalWindow: emitter.hideWindow,
		})
		first := startApproval(approver, context.Background(), createTestRequest())
		waitForApprovalEvent(t, emitter)
		time.Sleep(time.Minute)
		synctest.Wait()
		waitForResult(t, first)
		second := startApproval(approver, context.Background(), createTestRequest())
		event := waitForApprovalEvent(t, emitter)
		time.Sleep(3 * time.Second)
		synctest.Wait()
		if _, hidden := emitter.getWindowCount(); hidden != 0 {
			t.Fatal("an expired notice hid a newer request")
		}
		approver.Respond(event.RequestID, false, false)
		waitForResult(t, second)
		synctest.Wait()
		if _, hidden := emitter.getWindowCount(); hidden != 1 {
			t.Fatalf("window hidden %d times, want once after denying the newer request", hidden)
		}
	})
}

func TestDeadlineAndPhoneDoNotWaitForTheWindow(t *testing.T) {
	releaseChannel := make(chan struct{})
	t.Cleanup(func() { close(releaseChannel) })
	phone := createFakePhone()
	approver := NewApprover(ApproverConfig{
		Biometric:            &fakeBiometric{isAvailable: true},
		Emit:                 func(string, any) { <-releaseChannel },
		ShowApprovalWindow:   func(bool) { <-releaseChannel },
		HideApprovalWindow:   func() { <-releaseChannel },
		PhoneApprover:        phone,
		Timeout:              100 * time.Millisecond,
		PhoneApprovalTimeout: 300 * time.Millisecond,
	})
	resultChannel := startApproval(approver, context.Background(), createTestRequest())
	phone.waitForStart(t)
	startedAt := time.Now()
	if result := waitForResultWithin(t, resultChannel, 3*time.Second, "a request whose window never shows"); result.result.IsAllowed {
		t.Errorf("result = %+v", result)
	}
	if elapsed := time.Since(startedAt); elapsed > 2*time.Second {
		t.Errorf("the deadline came after %v", elapsed)
	}
	waitUntil(t, func() bool { _, finishCount := phone.getStartAndFinishCount(); return finishCount == 1 }, "the phone request was not withdrawn")

	resultChannel = startApproval(approver, context.Background(), createTestRequest())
	phone.waitForStart(t)
	phone.answer(true)
	if result := waitForResultWithin(t, resultChannel, 3*time.Second, "a request the phone approved"); !result.result.IsAllowed {
		t.Errorf("the phone's approval was lost while the window was stuck: %+v", result)
	}
}

func TestQueuedRequestsAreAnsweredOnThePhoneWhileTheFirstIsStuck(t *testing.T) {
	biometric := createStuckBiometric()
	emitter := createFakeEmitter()
	phone := createFakePhone()
	approver := NewApprover(ApproverConfig{
		Biometric: biometric, CancelBiometric: biometric.cancel, Emit: emitter.emit, PhoneApprover: phone,
		Timeout: 500 * time.Millisecond, PhoneApprovalTimeout: 1500 * time.Millisecond,
	})
	firstChannel := startApproval(approver, context.Background(), createTestRequest())
	firstEvent := waitForApprovalEvent(t, emitter)
	phone.waitForStart(t)
	approver.Respond(firstEvent.RequestID, true, false)
	biometric.waitForStart(t)

	secondChannel := startApproval(approver, context.Background(), createTestRequest())
	waitForApprovalEvent(t, emitter)
	phone.waitForStart(t)
	thirdChannel := startApproval(approver, context.Background(), createTestRequest())
	waitForApprovalEvent(t, emitter)
	phone.waitForStart(t)

	phone.answerRequest(2, false)
	phone.answerRequest(1, true)
	if result := waitForResultWithin(t, secondChannel, time.Second, "the second request"); !result.result.IsAllowed {
		t.Errorf("second = %+v", result)
	}
	if result := waitForResultWithin(t, thirdChannel, time.Second, "the third request"); result.result.IsAllowed {
		t.Errorf("third = %+v", result)
	}
	select {
	case result := <-firstChannel:
		t.Fatalf("the stuck request ended before its deadline: %+v", result)
	default:
	}
	if result := waitForResultWithin(t, firstChannel, 4*time.Second, "the stuck first request"); result.result.IsAllowed {
		t.Errorf("first = %+v", result)
	}
	waitUntil(t, func() bool { return !biometric.isWaiting() }, "the stuck Touch ID check was not cancelled")
	waitUntil(t, func() bool { _, finishCount := phone.getStartAndFinishCount(); return finishCount == 3 }, "phone requests were not withdrawn")

	fourthChannel := startApproval(approver, context.Background(), createTestRequest())
	fourthEvent := waitForApprovalEvent(t, emitter)
	phone.waitForStart(t)
	approver.Respond(fourthEvent.RequestID, true, false)
	biometric.waitForStart(t)
	biometric.mutex.Lock()
	close(biometric.cancelChannel)
	biometric.isActive = false
	biometric.mutex.Unlock()
	if result := waitForResultWithin(t, fourthChannel, 3*time.Second, "the request after the stuck one"); result.result.IsAllowed {
		t.Errorf("a cancelled Touch ID check approved: %+v", result)
	}
}

func TestClientDisconnectEndsTouchIDAndWithdrawsThePhoneRequest(t *testing.T) {
	biometric := createStuckBiometric()
	emitter := createFakeEmitter()
	phone := createFakePhone()
	approver := NewApprover(ApproverConfig{Biometric: biometric, CancelBiometric: biometric.cancel, Emit: emitter.emit, PhoneApprover: phone})
	ctx, cancel := context.WithCancel(context.Background())
	resultChannel := startApproval(approver, ctx, createTestRequest())
	event := waitForApprovalEvent(t, emitter)
	phone.waitForStart(t)
	approver.Respond(event.RequestID, true, false)
	biometric.waitForStart(t)
	cancel()
	if result := waitForResultWithin(t, resultChannel, 3*time.Second, "a request whose client disconnected"); result.result.IsAllowed {
		t.Errorf("result = %+v", result)
	}
	waitUntil(t, func() bool { return !biometric.isWaiting() }, "the Touch ID check was left open after the client disconnected")
	waitUntil(t, func() bool { _, finishCount := phone.getStartAndFinishCount(); return finishCount == 1 }, "the phone request was not withdrawn")
	waitUntil(t, func() bool {
		for _, eventName := range emitter.listEventNames() {
			if eventName == ApprovalClosedEventName {
				return true
			}
		}
		return false
	}, "the dialog was not closed")
}

func TestTouchIDOfAFinishedRequestNeverStarts(t *testing.T) {
	biometric := createStuckBiometric()
	emitter := createFakeEmitter()
	approver := NewApprover(ApproverConfig{Biometric: biometric, CancelBiometric: biometric.cancel, Emit: emitter.emit})
	firstChannel := startApproval(approver, context.Background(), createTestRequest())
	firstEvent := waitForApprovalEvent(t, emitter)
	approver.Respond(firstEvent.RequestID, true, false)
	biometric.waitForStart(t)
	secondContext, endSecond := context.WithCancel(context.Background())
	secondChannel := startApproval(approver, secondContext, createTestRequest())
	secondEvent := waitForApprovalEvent(t, emitter)
	approver.Respond(secondEvent.RequestID, true, false)
	// Once the request took its answer, its authorize goroutine waits behind the first check's biometricMutex.
	waitUntil(t, func() bool {
		approver.mutex.Lock()
		defer approver.mutex.Unlock()
		return len(approver.findPending(secondEvent.RequestID).responseChannel) == 0
	}, "the second request never took its answer")
	endSecond()
	if result := waitForResultWithin(t, secondChannel, 3*time.Second, "the second request"); result.result.IsAllowed {
		t.Fatalf("the ended request was allowed: %+v", result)
	}
	approver.Respond(firstEvent.RequestID, false, false)
	waitForResultWithin(t, firstChannel, 3*time.Second, "the first request")
	authorizeEnded := make(chan struct{})
	go func() {
		approver.authorizing.Wait()
		close(authorizeEnded)
	}()
	select {
	case <-authorizeEnded:
	case <-biometric.startedChannel:
		t.Error("Touch ID started for a request that had already ended")
		biometric.cancel()
		<-authorizeEnded
	}
	if biometric.isWaiting() {
		t.Error("a Touch ID check is still waiting")
	}
}
