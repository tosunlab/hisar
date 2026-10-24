package service

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/server/internal/ratelimit"
	"github.com/tryhisar/hisar/server/internal/realtime"
	"github.com/tryhisar/hisar/server/internal/settings"
	"github.com/tryhisar/hisar/server/internal/store"
	"golang.org/x/time/rate"
)

func TestExpiredApprovalLeavesMemoryWithoutANewApproval(t *testing.T) {
	current := &atomic.Pointer[settings.Settings]{}
	current.Store(&settings.Settings{})
	service := &Service{Approvals: newApprovalRelay(ratelimit.New(rate.Every(approvalRateEvery), ApprovalRateBurst)), events: realtime.NewBroker(), instanceSettings: InstanceSettings{Current: current}}
	start := time.Now()
	var now atomic.Int64
	now.Store(start.UnixNano())
	service.Approvals.SetClock(func() time.Time { return time.Unix(0, now.Load()) })
	requestID := strings.Repeat("ab", approvalRequestIDByteCount)
	request := &apitypes.CreateApprovalRequest{RequestID: requestID, SealedRequest: []byte{1}, ExpiresAt: start.Add(time.Minute)}
	if err := service.CreateApproval(Actor{AccountID: "A", DeviceID: "D"}, request); err != nil {
		t.Fatal(err)
	}
	now.Store(start.Add(time.Minute + ApprovalRetention + time.Second).UnixNano())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go service.Approvals.Run(ctx, 10*time.Millisecond)
	for deadline := time.Now().Add(2 * time.Second); service.Approvals.IsKept("A", requestID); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the sealed request stayed in memory past its retention")
		}
	}
}

func TestLoginCache(t *testing.T) {
	cache := newLoginCache(50*time.Millisecond, 0)
	loginID := cache.put(&pendingLogin{accountID: "A"})
	if pending := cache.take(loginID); pending == nil || pending.accountID != "A" {
		t.Fatal("take failed")
	}
	if cache.take(loginID) != nil {
		t.Error("login state reusable")
	}
	loginID = cache.put(&pendingLogin{accountID: "A"})
	time.Sleep(60 * time.Millisecond)
	if cache.take(loginID) != nil {
		t.Error("expired login state returned")
	}
	if cache.take("nope") != nil {
		t.Error("unknown id returned state")
	}
}

func TestManyPendingLoginsDoNotSlowEveryLogin(t *testing.T) {
	cache := newLoginCache(time.Hour, 0)
	deadline := time.Now().Add(5 * time.Second)
	var firstLoginID string
	for index := range 200_000 {
		loginID := cache.put(&pendingLogin{accountID: "A"})
		if index == 0 {
			firstLoginID = loginID
		}
		if index%1000 == 0 && time.Now().After(deadline) {
			t.Fatalf("%d pending logins took over 5 s: each login walks every pending one", index)
		}
	}
	if cache.take(firstLoginID) == nil {
		t.Error("a pending login was dropped before it expired")
	}
}

func TestIsValidDevice(t *testing.T) {
	if isValidDevice(apitypes.DeviceInfo{}) || isValidDevice(apitypes.DeviceInfo{Name: string(make([]byte, 101)), Platform: "x"}) {
		t.Error("invalid device accepted")
	}
}

func TestComposeCredentialsChangedEmail(t *testing.T) {
	changedAt := time.Date(2026, 10, 11, 9, 30, 0, 0, time.UTC)
	for _, testCase := range []struct {
		change               string
		signedOutDeviceCount int
		subject              string
		expectedValues       []string
	}{
		{apitypes.CredentialChangeMasterPassword, 0, "Your Hisar master password was changed", []string{"No other devices were signed in.", "11 October 2026 at 09:30 UTC"}},
		{apitypes.CredentialChangeMasterPassword, 1, "Your Hisar master password was changed", []string{"Your other device was signed out.", "Your Secret Key and Emergency Kit stay the same."}},
		{apitypes.CredentialChangeSecretKey, 3, "Your Hisar Secret Key was replaced", []string{"Your 3 other devices were signed out.", "Your old Secret Key and Emergency Kit no longer work."}},
	} {
		message := composeCredentialsChangedEmail("alice@example.com", testCase.change, testCase.signedOutDeviceCount, changedAt)
		if message.To != "alice@example.com" || message.Subject != testCase.subject {
			t.Errorf("%s: %q to %q", testCase.change, message.Subject, message.To)
		}
		for _, expected := range testCase.expectedValues {
			if !strings.Contains(message.Body, expected) {
				t.Errorf("%s with %d devices: no %q in %s", testCase.change, testCase.signedOutDeviceCount, expected, message.Body)
			}
		}
	}
}

func TestComposeAccountDeletedEmail(t *testing.T) {
	deletedAt := time.Date(2026, 10, 11, 9, 30, 0, 0, time.UTC)
	message := composeAccountDeletedEmail("alice@example.com", deletedAt, false)
	if message.Subject != "Your Hisar account was deleted" || !strings.Contains(message.Body, "alice@example.com was deleted on 11 October 2026") ||
		!strings.Contains(message.Body, "kept for 30 days") || !strings.Contains(message.Body, "If you did not do this, contact support@tryhisar.com.") {
		t.Errorf("email: %+v", message)
	}
	message = composeAccountDeletedEmail("alice@example.com", deletedAt, true)
	if !strings.Contains(message.Body, "alice@example.com was deleted by an admin of the Hisar server on 11 October 2026") ||
		!strings.Contains(message.Body, "contact the admin of the Hisar server.") || strings.Contains(message.Body, "If you did not do this") {
		t.Errorf("email after an admin deletion: %+v", message)
	}
}

func TestInviteState(t *testing.T) {
	expiresAt := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	usedAt := expiresAt.Add(-time.Hour)
	for _, testCase := range []struct {
		usedAt   *time.Time
		now      time.Time
		expected string
	}{
		{nil, expiresAt.Add(-time.Nanosecond), invitePending},
		{nil, expiresAt, inviteExpired},
		{nil, expiresAt.Add(time.Nanosecond), inviteExpired},
		{&usedAt, expiresAt.Add(-time.Hour), inviteUsed},
		{&usedAt, expiresAt.Add(time.Hour), inviteUsed},
	} {
		if state := inviteState(&store.AdminInvite{ExpiresAt: expiresAt, UsedAt: testCase.usedAt}, testCase.now); state != testCase.expected {
			t.Errorf("used %v at %v: %s, want %s", testCase.usedAt != nil, testCase.now, state, testCase.expected)
		}
	}
}
