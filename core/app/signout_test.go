package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tryhisar/hisar/core/localstate"
)

type failingDeleteSecretKeyStore struct {
	SecretKeyStore
}

func (failingDeleteSecretKeyStore) DeleteSecretKey(string) error {
	return errors.New("keychain unavailable")
}

func requireSignedOutLocally(t *testing.T, service *Service) {
	t.Helper()
	if status := service.GetStatus(); status.State != StateSignedOut {
		t.Errorf("state after sign-out: %q", status.State)
	}
	service.mutex.Lock()
	isOpen, itemsByVaultID := service.unlockedSession != nil, service.itemsByVaultID
	service.mutex.Unlock()
	if isOpen || itemsByVaultID != nil {
		t.Error("the decrypted session outlived the sign-out")
	}
	saved, err := localstate.Load(service.config.StatePath)
	if err != nil {
		t.Fatal(err)
	}
	if saved.IsLoggedIn() {
		t.Error("the saved state still holds the session token")
	}
}

func TestSignOutWhenTheKeychainFailsStillSignsOut(t *testing.T) {
	fixture := createUnlockedTestService(t)
	service := fixture.service
	if _, err := service.ListItems(fixture.personalID); err != nil {
		t.Fatal(err)
	}
	service.config.SecretKeyStore = failingDeleteSecretKeyStore{service.config.SecretKeyStore}
	if _, err := service.SignOut(); err == nil {
		t.Error("the keychain failure was not reported")
	}
	requireSignedOutLocally(t, service)
}

func TestSignOutReportsTheServerSessionItCouldNotEnd(t *testing.T) {
	fixture := createUnlockedTestService(t)
	service := fixture.service
	fixture.apiServer.setUnreachable(true)
	if _, err := service.SignOut(); err == nil {
		t.Error("a sign-out the server never heard of reported success")
	}
	requireSignedOutLocally(t, service)
}

func TestSignOutSucceedsWhenTheServerEndsTheSession(t *testing.T) {
	fixture := createUnlockedTestService(t)
	if _, err := fixture.service.SignOut(); err != nil {
		t.Errorf("sign-out: %v", err)
	}
	requireSignedOutLocally(t, fixture.service)
}

func TestSignOutReportsAnOfflineCopyItCouldNotDelete(t *testing.T) {
	fixture := createUnlockedTestService(t)
	service := fixture.service
	undeletable := service.config.CachePath + "-journal"
	if err := os.MkdirAll(filepath.Join(undeletable, "kept"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SignOut(); err == nil {
		t.Error("the sign-out reported success while the offline copy stayed on disk")
	}
	requireSignedOutLocally(t, service)
}
