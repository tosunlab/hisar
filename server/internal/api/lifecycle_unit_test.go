package api

import (
	"strings"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/server/internal/realtime"
)

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

func TestEventBrokerEndsAllStreamExceptTheKeptOne(t *testing.T) {
	broker := realtime.NewBroker()
	kept, _ := broker.Subscribe("ALICE1", []byte("kept"))
	ended, _ := broker.Subscribe("ALICE1", []byte("ended"))
	other, _ := broker.Subscribe("BOB111", []byte("bob"))
	broker.EndStreams("ALICE1", []byte("kept"))
	if broker.IsEnded(kept) || !broker.IsEnded(ended) || broker.IsEnded(other) {
		t.Errorf("ended: kept %v, ended %v, other account %v", broker.IsEnded(kept), broker.IsEnded(ended), broker.IsEnded(other))
	}
	broker.EndStreams("ALICE1", nil)
	if !broker.IsEnded(kept) {
		t.Error("ending every stream kept one")
	}
}
