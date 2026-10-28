package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/session"
)

func commitFixtureSecretKeyReplacement(t *testing.T, service *Service) CredentialChangeResult {
	t.Helper()
	key, err := crypt.GenerateSecretKey(service.state.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.changeCredentials(testMasterPassword, func(context.Context, *session.Session, []byte, crypt.SecretKey, session.StagePendingFactor) (*session.CredentialChange, error) {
		return &session.CredentialChange{SecretKey: key}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestMobileReplacementContinuationKeepsItsAccount(t *testing.T) {
	fixture := createVerifiableService(t)
	service := fixture.service
	result := commitFixtureSecretKeyReplacement(t, service)
	other := createVerifiableService(t)
	keyB, err := crypt.GenerateSecretKey(other.accountID)
	if err != nil {
		t.Fatal(err)
	}
	signInOtherAccount(t, service, other)
	service.mutex.Lock()
	service.setKitGrant("", keyB, nil)
	service.mutex.Unlock()
	directory := t.TempDir()
	kit, err := ExportNewSecretKeyEmergencyKitToDirectory(service, result, directory)
	if !errors.Is(err, errSessionEnded) || kit.Path != "" || kit.SecretKey != "" {
		t.Errorf("another account's kit was not refused: %+v, %v", kit, err)
	}
	if result.SecretKey == "" || result.SecretKey == keyB.Encode() {
		t.Error("the already committed replacement result was lost")
	}
	requireEmptyDirectory(t, directory)
}

func TestMobileReplacementContinuationKeepsItsKey(t *testing.T) {
	service := createVerifiableService(t).service
	first := commitFixtureSecretKeyReplacement(t, service)
	second := commitFixtureSecretKeyReplacement(t, service)
	directory := t.TempDir()
	if kit, err := ExportNewSecretKeyEmergencyKitToDirectory(service, first, directory); !errors.Is(err, errSessionEnded) || kit.Path != "" {
		t.Fatalf("an older replacement selected the current key: %+v, %v", kit, err)
	}
	requireEmptyDirectory(t, directory)
	kit, err := ExportNewSecretKeyEmergencyKitToDirectory(service, second, directory)
	if err != nil || kit.SecretKey != second.SecretKey || kit.Path == "" {
		t.Fatalf("the current replacement's kit is unavailable: %+v, %v", kit, err)
	}
	if document, err := os.ReadFile(kit.Path); err != nil || !strings.HasPrefix(string(document), "%PDF-") {
		t.Errorf("the current replacement did not save a PDF: %v", err)
	}
}

func TestMobileReplacementKitFailureKeepsCommittedCredentials(t *testing.T) {
	service := createVerifiableService(t).service
	result := commitFixtureSecretKeyReplacement(t, service)
	result.LocalError = "credentials changed but local state could not be saved"
	result.KeychainError = "write down the new Secret Key"
	service.Lock()
	directory := t.TempDir()
	if kit, err := ExportNewSecretKeyEmergencyKitToDirectory(service, result, directory); !errors.Is(err, ErrLocked) || kit.Path != "" {
		t.Fatalf("kit after lock: %+v, %v", kit, err)
	}
	requireEmptyDirectory(t, directory)
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"identity", "epoch", "accountId", "accountID"} {
		if _, isPresent := fields[name]; isPresent {
			t.Fatalf("the private continuation identity reached the JSON result: %s", encoded)
		}
	}
	var visible CredentialChangeResult
	if err := json.Unmarshal(encoded, &visible); err != nil {
		t.Fatal(err)
	}
	if visible.SecretKey != result.SecretKey || visible.KeychainError != result.KeychainError || visible.LocalError != result.LocalError || visible.Status.State != StateUnlocked {
		t.Fatalf("the committed result was lost when the optional kit failed: %s", encoded)
	}
}
