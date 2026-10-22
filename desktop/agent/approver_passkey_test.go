package agent

import (
	"context"
	"testing"
)

func TestPasskeyWithoutTouchIDRequiresFreshMasterPassword(t *testing.T) {
	emitter := createFakeEmitter()
	unlocker := &fakeUnlocker{correctPassword: "ceremony-password"}
	approver := createCommandLineApprover(&fakeBiometric{isAvailable: false}, emitter, unlocker, 0)
	for range 2 {
		result := make(chan error, 1)
		go func() {
			result <- approver.VerifyPasskey(context.Background(), "verify you for a passkey at example.com")
		}()
		event := waitForApprovalEvent(t, emitter)
		if event.Kind != ApprovalKindPasskey || !event.IsPasswordUnlock || event.IsPhoneApproval || event.IsRememberable {
			t.Fatalf("unsafe passkey approval: %+v", event)
		}
		approver.Respond(event.RequestID, true, true)
		if failed := waitForFailedEvent(t, emitter); failed.Message != errPasswordRequired.Error() {
			t.Fatalf("unverified click: %+v", failed)
		}
		select {
		case err := <-result:
			t.Fatalf("click-only approval verified the user: %v", err)
		default:
		}
		approver.RespondWithPassword(event.RequestID, "ceremony-password", false)
		if err := waitForCommandLineResult(t, result); err != nil {
			t.Fatal(err)
		}
	}
	unlocker.mutex.Lock()
	defer unlocker.mutex.Unlock()
	if len(unlocker.passwords) != 2 {
		t.Fatalf("verification reused earlier evidence: %v", unlocker.passwords)
	}
}

// Locking ends the session the passkey ceremony belongs to, so its approval window must not unlock Hisar or prompt.
func TestPasskeyApprovalEndsWhenHisarLocks(t *testing.T) {
	for _, approve := range []string{"touch-id", "password"} {
		emitter := createFakeEmitter()
		biometric := &fakeBiometric{isAvailable: true}
		unlocker := &fakeUnlocker{correctPassword: "ceremony-password", isTouchIDUnlockReady: true}
		approver := createCommandLineApprover(biometric, emitter, unlocker, 0)
		result := make(chan error, 1)
		go func() {
			result <- approver.VerifyPasskey(context.Background(), "use your passkey for example.com")
		}()
		event := waitForApprovalEvent(t, emitter)
		unlocker.mutex.Lock()
		unlocker.isLocked = true
		unlocker.mutex.Unlock()
		if approve == "touch-id" {
			approver.Respond(event.RequestID, true, false)
		} else {
			approver.RespondWithPassword(event.RequestID, "ceremony-password", false)
		}
		if err := waitForCommandLineResult(t, result); err == nil {
			t.Fatalf("%s: a passkey was verified after Hisar locked", approve)
		}
		if !unlocker.IsLocked() || len(biometric.listReasons()) != 0 {
			t.Fatalf("%s: the passkey window unlocked Hisar or prompted for Touch ID: locked %v, prompts %q", approve, unlocker.IsLocked(), biometric.listReasons())
		}
	}
}
