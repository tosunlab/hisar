package service

import (
	"strings"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/server/internal/store"
)

func TestLoginCache(t *testing.T) {
	cache := newLoginCache(50 * time.Millisecond)
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
	message := composeAccountDeletedEmail("alice@example.com", time.Date(2026, 10, 11, 9, 30, 0, 0, time.UTC))
	if message.Subject != "Your Hisar account was deleted" || !strings.Contains(message.Body, "alice@example.com was deleted on 11 October 2026") || !strings.Contains(message.Body, "kept for 30 days") {
		t.Errorf("email: %+v", message)
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
