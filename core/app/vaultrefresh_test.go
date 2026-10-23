package app

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/tryhisar/hisar/core/apitypes"
)

func TestVaultRefreshDropsTheItemsOfARemovedVault(t *testing.T) {
	fixture := createUnlockedTestService(t)
	service := fixture.service
	saved, err := service.SaveItem(fixture.familyID, ItemDetail{Title: "Bank", Password: "bank-password"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ListItems(fixture.familyID); err != nil {
		t.Fatal(err)
	}
	fixture.apiServer.mutex.Lock()
	fixture.apiServer.vaults = slices.DeleteFunc(fixture.apiServer.vaults, func(vault apitypes.Vault) bool { return vault.ID == fixture.familyID })
	fixture.apiServer.mutex.Unlock()
	service.mutex.Lock()
	err = service.loadVaults(context.Background())
	service.mutex.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if detail, err := service.GetItem(fixture.familyID, saved.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("an item of the removed vault is still readable: %q %v", detail.Password, err)
	}
}
