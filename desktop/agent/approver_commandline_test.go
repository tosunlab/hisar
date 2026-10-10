package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/tryhisar/hisar/desktop/appinfo"
	"github.com/tryhisar/hisar/desktop/commandline"
)

type fakeUnlocker struct {
	mutex                sync.Mutex
	isLocked             bool
	isTouchIDUnlockReady bool
	correctPassword      string
	allPassword          []string
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
	unlocker.allPassword = append(unlocker.allPassword, password)
	if password != unlocker.correctPassword {
		return errors.New("Incorrect master password.")
	}
	unlocker.isLocked = false
	return nil
}

func createCommandLineApprover(biometric Biometric, emitter *fakeEmitter, unlocker Unlocker, timeout time.Duration) *Approver {
	return CreateApprover(ApproverConfig{
		Biometric: biometric, Emit: emitter.emit, ShowApprovalWindow: emitter.showWindow, HideApprovalWindow: emitter.hideWindow,
		FindApp: func(pid int) (appinfo.App, bool) {
			if pid != 42 {
				return appinfo.App{Name: "Hisar"}, true
			}
			return appinfo.App{Name: "Terminal", IconURL: "data:image/png;base64,AA=="}, true
		},
		Unlocker: unlocker,
		Timeout:  timeout,
	})
}

func createCommandLineRequest() commandline.ApprovalRequest {
	return commandline.ApprovalRequest{ClientPID: 77, ParentPID: 42, ClientPath: "/Applications/Hisar.app/Contents/Helpers/hisar", TerminalName: "ttys003"}
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
	if allReason := biometric.listAllReason(); len(allReason) != 1 || allReason[0] != `allow "hisar" in Terminal to use Hisar` {
		t.Errorf("Touch ID reasons = %q", allReason)
	}
}

func TestCommandLineApprovalDenyAndTimeout(t *testing.T) {
	emitter := createFakeEmitter()
	approver := createCommandLineApprover(&fakeBiometric{isAvailable: true}, emitter, &fakeUnlocker{}, 0)
	resultChannel := startCommandLineApproval(approver, context.Background())
	event := waitForApprovalEvent(t, emitter)
	approver.Respond(event.RequestID, false, false)
	if err := waitForCommandLineResult(t, resultChannel); !errors.Is(err, commandline.ErrDenied) {
		t.Errorf("deny: %v", err)
	}

	timeoutApprover := createCommandLineApprover(&fakeBiometric{isAvailable: true}, createFakeEmitter(), &fakeUnlocker{}, 20*time.Millisecond)
	if err := timeoutApprover.ApproveCommandLine(context.Background(), createCommandLineRequest()); !errors.Is(err, commandline.ErrDenied) {
		t.Errorf("timeout: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	disconnectEmitter := createFakeEmitter()
	disconnectApprover := createCommandLineApprover(&fakeBiometric{isAvailable: true}, disconnectEmitter, &fakeUnlocker{}, 0)
	resultChannel = startCommandLineApproval(disconnectApprover, ctx)
	waitForApprovalEvent(t, disconnectEmitter)
	cancel()
	if err := waitForCommandLineResult(t, resultChannel); !errors.Is(err, commandline.ErrDenied) {
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
	if err := waitForCommandLineResult(t, resultChannel); err != nil || len(biometric.listAllReason()) != 0 {
		t.Errorf("approval %v, Touch ID %v", err, biometric.listAllReason())
	}
}

func TestCommandLineApprovalUnlocksWithTouchID(t *testing.T) {
	biometric := &fakeBiometric{isAvailable: true, allAuthenticateResult: []error{errors.New("no match")}}
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
	if allReason := biometric.listAllReason(); len(allReason) != 2 || allReason[1] != `unlock Hisar for "hisar" in Terminal` {
		t.Errorf("Touch ID reasons = %q", allReason)
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
	if approver.RespondWithPassword(event.RequestID, "") {
		t.Error("empty password accepted")
	}
	approver.RespondWithPassword(event.RequestID, "wrong password")
	if failedEvent := waitForFailedEvent(t, emitter); failedEvent.Message != "Incorrect master password." {
		t.Errorf("failure with a wrong password %q", failedEvent.Message)
	}
	approver.RespondWithPassword(event.RequestID, "correct horse battery")
	if err := waitForCommandLineResult(t, resultChannel); err != nil {
		t.Fatalf("approval: %v", err)
	}
	if unlocker.IsLocked() || len(biometric.listAllReason()) != 0 {
		t.Errorf("locked %v, Touch ID %v", unlocker.IsLocked(), biometric.listAllReason())
	}
}

func TestRespondWithPasswordIgnoresSSHRequests(t *testing.T) {
	emitter := createFakeEmitter()
	approver := createTestApprover(&fakeBiometric{isAvailable: true}, emitter)
	resultChannel := startApproval(approver, context.Background(), createTestRequest())
	event := waitForApprovalEvent(t, emitter)
	if event.Kind != ApprovalKindSSHKey {
		t.Errorf("SSH event kind %q", event.Kind)
	}
	if approver.RespondWithPassword(event.RequestID, "password") {
		t.Error("a password answered an SSH request")
	}
	approver.Respond(event.RequestID, false, false)
	waitForResult(t, resultChannel)
}
