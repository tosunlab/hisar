package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/desktop/appinfo"
)

type fakeUnlocker struct {
	mutex                sync.Mutex
	isLocked             bool
	isTouchIDUnlockReady bool
	correctPassword      string
	passwords            []string
	touchIDUnlockCount   int
}

func (unlocker *fakeUnlocker) IsLocked() bool {
	unlocker.mutex.Lock()
	defer unlocker.mutex.Unlock()
	return unlocker.isLocked
}

func (unlocker *fakeUnlocker) IsTouchIDUnlockReady() bool { return unlocker.isTouchIDUnlockReady }

func (unlocker *fakeUnlocker) UnlockWithTouchID(authenticate func(reason string) error) error {
	if err := authenticate("unlock Hisar"); err != nil {
		return err
	}
	unlocker.mutex.Lock()
	defer unlocker.mutex.Unlock()
	unlocker.touchIDUnlockCount++
	unlocker.isLocked = false
	return nil
}

func (unlocker *fakeUnlocker) UnlockWithPassword(password string) error {
	unlocker.mutex.Lock()
	defer unlocker.mutex.Unlock()
	unlocker.passwords = append(unlocker.passwords, password)
	if password != unlocker.correctPassword {
		return errors.New("Incorrect master password.")
	}
	unlocker.isLocked = false
	return nil
}

func (unlocker *fakeUnlocker) VerifyPassword(password string) error {
	unlocker.mutex.Lock()
	defer unlocker.mutex.Unlock()
	unlocker.passwords = append(unlocker.passwords, password)
	if password != unlocker.correctPassword {
		return errors.New("Incorrect master password.")
	}
	return nil
}

func createCommandLineApprover(biometric Biometric, emitter *fakeEmitter, unlocker Unlocker, timeout time.Duration) *Approver {
	return NewApprover(ApproverConfig{
		Biometric: biometric, Emit: emitter.emit, ShowApprovalWindow: emitter.showWindow, HideApprovalWindow: emitter.hideWindow,
		ResolveRequester: func(pid int) appinfo.Requester {
			if pid != 77 {
				return appinfo.Requester{Kind: appinfo.RequesterKindApp, Name: "Hisar", App: appinfo.App{Name: "Hisar"}}
			}
			return createTerminalRequester(nil)
		},
		Unlocker: unlocker,
		Timeout:  timeout,
	})
}

func createCommandLineRequest() clisocket.ApprovalRequest {
	return clisocket.ApprovalRequest{ClientPID: 77, ParentPID: 42, ClientPath: "/Applications/Hisar.app/Contents/Helpers/hisar", TerminalName: "ttys003"}
}

func startCommandLineApproval(approver *Approver, ctx context.Context) chan error {
	resultChannel := make(chan error, 1)
	go func() { resultChannel <- approver.ApproveCommandLine(ctx, createCommandLineRequest()) }()
	return resultChannel
}

func waitForCommandLineResult(t *testing.T, resultChannel chan error) error {
	t.Helper()
	select {
	case err := <-resultChannel:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("approval did not finish")
		return nil
	}
}

func waitForFailedEvent(t *testing.T, emitter *fakeEmitter) ApprovalFailedEvent {
	t.Helper()
	select {
	case failedEvent := <-emitter.failedEventChannel:
		return failedEvent
	case <-time.After(5 * time.Second):
		t.Fatal("no failure event")
		return ApprovalFailedEvent{}
	}
}

func TestCommandLineApprovalNamesTheTerminalAndUsesTouchID(t *testing.T) {
	biometric := &fakeBiometric{isAvailable: true}
	emitter := createFakeEmitter()
	approver := createCommandLineApprover(biometric, emitter, &fakeUnlocker{}, 0)
	resultChannel := startCommandLineApproval(approver, context.Background())
	event := waitForApprovalEvent(t, emitter)
	if event.Kind != ApprovalKindCommandLine || event.AppName != "Terminal" || event.AppIconURL == "" || event.ClientName != "hisar" ||
		event.TerminalName != "ttys003" || event.ClientPath != "/Applications/Hisar.app/Contents/Helpers/hisar" || event.IsUnlockRequired || event.KeyTitle != "" {
		t.Errorf("event = %+v", event)
	}
	approver.Respond(event.RequestID, true, true)
	if err := waitForCommandLineResult(t, resultChannel); err != nil {
		t.Fatalf("approval: %v", err)
	}
	if reasons := biometric.listReasons(); len(reasons) != 1 || reasons[0] != `allow "hisar" in Terminal to use Hisar` {
		t.Errorf("Touch ID reasons = %q", reasons)
	}
}

func TestCommandLineApprovalDenyAndTimeout(t *testing.T) {
	emitter := createFakeEmitter()
	approver := createCommandLineApprover(&fakeBiometric{isAvailable: true}, emitter, &fakeUnlocker{}, 0)
	resultChannel := startCommandLineApproval(approver, context.Background())
	event := waitForApprovalEvent(t, emitter)
	approver.Respond(event.RequestID, false, false)
	if err := waitForCommandLineResult(t, resultChannel); !errors.Is(err, clisocket.ErrDenied) {
		t.Errorf("deny: %v", err)
	}

	timeoutApprover := createCommandLineApprover(&fakeBiometric{isAvailable: true}, createFakeEmitter(), &fakeUnlocker{}, 20*time.Millisecond)
	if err := timeoutApprover.ApproveCommandLine(context.Background(), createCommandLineRequest()); !errors.Is(err, clisocket.ErrDenied) {
		t.Errorf("timeout: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	disconnectEmitter := createFakeEmitter()
	disconnectApprover := createCommandLineApprover(&fakeBiometric{isAvailable: true}, disconnectEmitter, &fakeUnlocker{}, 0)
	resultChannel = startCommandLineApproval(disconnectApprover, ctx)
	waitForApprovalEvent(t, disconnectEmitter)
	cancel()
	if err := waitForCommandLineResult(t, resultChannel); !errors.Is(err, clisocket.ErrDenied) {
		t.Errorf("client gone: %v", err)
	}
}

func TestCommandLineApprovalWithoutTouchIDNeedsAuthorize(t *testing.T) {
	biometric := &fakeBiometric{isAvailable: false}
	emitter := createFakeEmitter()
	approver := createCommandLineApprover(biometric, emitter, &fakeUnlocker{}, 0)
	resultChannel := startCommandLineApproval(approver, context.Background())
	event := waitForApprovalEvent(t, emitter)
	if event.IsTouchIDApproval {
		t.Error("Touch ID offered without Touch ID")
	}
	approver.Respond(event.RequestID, true, false)
	if err := waitForCommandLineResult(t, resultChannel); err != nil || len(biometric.listReasons()) != 0 {
		t.Errorf("approval %v, Touch ID %v", err, biometric.listReasons())
	}
}

func TestCommandLineApprovalUnlocksWithTouchID(t *testing.T) {
	biometric := &fakeBiometric{isAvailable: true, authenticateResults: []error{errors.New("no match")}}
	emitter := createFakeEmitter()
	unlocker := &fakeUnlocker{isLocked: true, isTouchIDUnlockReady: true}
	approver := createCommandLineApprover(biometric, emitter, unlocker, 0)
	resultChannel := startCommandLineApproval(approver, context.Background())
	event := waitForApprovalEvent(t, emitter)
	if !event.IsUnlockRequired || event.IsPasswordUnlock {
		t.Fatalf("event = %+v", event)
	}
	approver.Respond(event.RequestID, true, false)
	if failedEvent := waitForFailedEvent(t, emitter); failedEvent.Message != errTouchIDFailed.Error() {
		t.Errorf("failure %q", failedEvent.Message)
	}
	approver.Respond(event.RequestID, true, false)
	if err := waitForCommandLineResult(t, resultChannel); err != nil {
		t.Fatalf("approval: %v", err)
	}
	if unlocker.IsLocked() || unlocker.touchIDUnlockCount != 1 {
		t.Errorf("unlocked %v, Touch ID unlocks %d", !unlocker.IsLocked(), unlocker.touchIDUnlockCount)
	}
	if reasons := biometric.listReasons(); len(reasons) != 2 || reasons[1] != `unlock Hisar for "hisar" in Terminal` {
		t.Errorf("Touch ID reasons = %q", reasons)
	}
}

func TestCommandLineApprovalUnlocksWithPassword(t *testing.T) {
	biometric := &fakeBiometric{isAvailable: true}
	emitter := createFakeEmitter()
	unlocker := &fakeUnlocker{isLocked: true, correctPassword: "correct horse battery"}
	approver := createCommandLineApprover(biometric, emitter, unlocker, 0)
	resultChannel := startCommandLineApproval(approver, context.Background())
	event := waitForApprovalEvent(t, emitter)
	if !event.IsUnlockRequired || !event.IsPasswordUnlock {
		t.Fatalf("event = %+v", event)
	}
	approver.Respond(event.RequestID, true, false)
	if failedEvent := waitForFailedEvent(t, emitter); failedEvent.Message != "Enter your master password to unlock Hisar." {
		t.Errorf("failure without password %q", failedEvent.Message)
	}
	if approver.RespondWithPassword(event.RequestID, "", false) {
		t.Error("empty password accepted")
	}
	approver.RespondWithPassword(event.RequestID, "wrong password", false)
	if failedEvent := waitForFailedEvent(t, emitter); failedEvent.Message != "Incorrect master password." {
		t.Errorf("failure with a wrong password %q", failedEvent.Message)
	}
	approver.RespondWithPassword(event.RequestID, "correct horse battery", false)
	if err := waitForCommandLineResult(t, resultChannel); err != nil {
		t.Fatalf("approval: %v", err)
	}
	if unlocker.IsLocked() || len(biometric.listReasons()) != 0 {
		t.Errorf("locked %v, Touch ID %v", unlocker.IsLocked(), biometric.listReasons())
	}
}

func TestCommandLineApprovalWithPasswordWhileUnlocked(t *testing.T) {
	biometric := &fakeBiometric{isAvailable: true}
	emitter := createFakeEmitter()
	unlocker := &fakeUnlocker{correctPassword: "correct horse battery"}
	approver := createCommandLineApprover(biometric, emitter, unlocker, 0)
	resultChannel := startCommandLineApproval(approver, context.Background())
	event := waitForApprovalEvent(t, emitter)
	approver.RespondWithPassword(event.RequestID, "correct horse battery", true)
	if err := waitForCommandLineResult(t, resultChannel); err != nil {
		t.Fatalf("approval: %v", err)
	}
	if len(unlocker.passwords) != 1 || len(biometric.listReasons()) != 0 {
		t.Errorf("passwords %v, Touch ID %v", unlocker.passwords, biometric.listReasons())
	}
	if approver.RespondWithPassword("unknown", "correct horse battery", false) {
		t.Error("a password answered an unknown request")
	}
}
