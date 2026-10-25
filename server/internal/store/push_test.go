package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPushTargetsAreTheAccountsSignedInDevicesThatWantTheKind(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	firstDevice, _ := createTestAccount(t, database, "AAAAAA", "a@example.com")
	otherDevice, _ := createTestAccount(t, database, "BBBBBB", "b@example.com")
	if _, err := database.pool.Exec(ctx, `UPDATE devices SET platform = 'android/mobile'`); err != nil {
		t.Fatal(err)
	}
	secondDevice := &Device{ID: uuid.NewString(), AccountID: "AAAAAA", Name: "phone", Platform: "ios/mobile"}
	secondSession := &Session{TokenHash: []byte("token-second"), AccountID: "AAAAAA", DeviceID: secondDevice.ID, ExpiresAt: time.Now().Add(time.Hour)}
	if err := database.CreateDeviceSession(ctx, secondDevice, secondSession, HashOpaqueRecord([]byte{1})); err != nil {
		t.Fatal(err)
	}

	if err := database.SetDevicePush(ctx, "AAAAAA", PushTarget{DeviceID: firstDevice.ID, Handle: "handle-1"}, []string{"approval", "security"}); err != nil {
		t.Fatal(err)
	}
	if err := database.SetDevicePush(ctx, "AAAAAA", PushTarget{DeviceID: secondDevice.ID, Handle: "handle-2"}, []string{"security"}); err != nil {
		t.Fatal(err)
	}
	if err := database.SetDevicePush(ctx, "AAAAAA", PushTarget{DeviceID: otherDevice.ID, Handle: "handle-3"}, []string{"approval"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another account's device took a handle: %v", err)
	}
	macDevice := &Device{ID: uuid.NewString(), AccountID: "AAAAAA", Name: "mac", Platform: "darwin/desktop"}
	if err := database.CreateDeviceSession(ctx, macDevice, &Session{TokenHash: []byte("token-mac"), AccountID: "AAAAAA", DeviceID: macDevice.ID, ExpiresAt: time.Now().Add(time.Hour)}, HashOpaqueRecord([]byte{1})); err != nil {
		t.Fatal(err)
	}
	if err := database.SetDevicePush(ctx, "AAAAAA", PushTarget{DeviceID: macDevice.ID, Handle: "handle-mac"}, []string{"approval"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a Mac took a handle: %v", err)
	}
	if err := database.SetDevicePush(ctx, "AAAAAA", PushTarget{DeviceID: firstDevice.ID, Handle: "handle-1"}, []string{"marketing"}); err == nil {
		t.Fatal("an unknown kind was saved")
	}

	expectTargets := func(kind, exceptDeviceID string, want ...string) {
		t.Helper()
		allTarget, err := database.ListPushTargets(ctx, "AAAAAA", kind, exceptDeviceID)
		if err != nil {
			t.Fatal(err)
		}
		got := []string{}
		for _, target := range allTarget {
			if target.WebPush != nil {
				got = append(got, target.WebPush.Endpoint)
			} else {
				got = append(got, target.Handle)
			}
		}
		if len(got) != len(want) || (len(want) > 0 && !sameStrings(got, want)) {
			t.Fatalf("%s targets except %s = %v, want %v", kind, exceptDeviceID, got, want)
		}
	}
	expectTargets("approval", "", "handle-1")
	expectTargets("security", "", "handle-1", "handle-2")
	expectTargets("security", firstDevice.ID, "handle-2")
	expectTargets("sharing", "")

	if err := database.SetDevicePush(ctx, "AAAAAA", PushTarget{DeviceID: firstDevice.ID, Handle: "handle-1b"}, []string{"approval"}); err != nil {
		t.Fatal(err)
	}
	if err := database.DeleteStalePushTarget(ctx, PushTarget{DeviceID: firstDevice.ID, Handle: "handle-1"}); err != nil {
		t.Fatal(err)
	}
	expectTargets("approval", "", "handle-1b")

	// An Android phone's Web Push subscription replaces its handle, and goes stale the same way.
	webPush := &WebPushSubscription{Endpoint: "https://ntfy.example/upABC", P256DH: make([]byte, 65), Auth: make([]byte, 16)}
	if err := database.SetDevicePush(ctx, "AAAAAA", PushTarget{DeviceID: firstDevice.ID, WebPush: webPush}, []string{"approval"}); err != nil {
		t.Fatal(err)
	}
	expectTargets("approval", "", "https://ntfy.example/upABC")
	if allTarget, _ := database.ListPushTargets(ctx, "AAAAAA", "approval", ""); allTarget[0].Handle != "" || len(allTarget[0].WebPush.P256DH) != 65 {
		t.Fatalf("Web Push target = %+v", allTarget[0])
	}
	badWebPush := &WebPushSubscription{Endpoint: "https://ntfy.example/upABC", P256DH: make([]byte, 64), Auth: make([]byte, 16)}
	if err := database.SetDevicePush(ctx, "AAAAAA", PushTarget{DeviceID: firstDevice.ID, WebPush: badWebPush}, []string{"approval"}); err == nil {
		t.Fatal("a public key of the wrong size was saved")
	}
	if err := database.DeleteStalePushTarget(ctx, PushTarget{DeviceID: firstDevice.ID, Handle: "handle-1b"}); err != nil {
		t.Fatal(err)
	}
	expectTargets("approval", "", "https://ntfy.example/upABC")
	if err := database.DeleteStalePushTarget(ctx, PushTarget{DeviceID: firstDevice.ID, WebPush: webPush}); err != nil {
		t.Fatal(err)
	}
	expectTargets("approval", "")
	if err := database.SetDevicePush(ctx, "AAAAAA", PushTarget{DeviceID: firstDevice.ID, Handle: "handle-1c"}, []string{"approval"}); err != nil {
		t.Fatal(err)
	}

	if _, err := database.RevokeDevice(ctx, "AAAAAA", secondDevice.ID); err != nil {
		t.Fatal(err)
	}
	expectTargets("security", "")
	if err := database.DeleteDevicePush(ctx, firstDevice.ID); err != nil {
		t.Fatal(err)
	}
	expectTargets("approval", "")
}

func sameStrings(first, second []string) bool {
	seen := map[string]int{}
	for _, value := range first {
		seen[value]++
	}
	for _, value := range second {
		seen[value]--
	}
	for _, count := range seen {
		if count != 0 {
			return false
		}
	}
	return len(first) == len(second)
}
