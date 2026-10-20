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
