package session

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/tryhisar/hisar/core/apitypes"
)

func TestRemovedVaultsAreReportedOnce(t *testing.T) {
	account := createOfflineTestAccount(t)
	ctx := context.Background()
	unlockedSession, err := Unlock(ctx, account.apiClient, openTestCache(t), account.password, account.secretKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unlockedSession.ListVaults(ctx); err != nil {
		t.Fatal(err)
	}
	if removed := unlockedSession.TakeRemovedVaults(); len(removed) != 0 {
		t.Fatalf("removed before any change: %+v", removed)
	}

	account.server.mutex.Lock()
	familyVault := account.server.vaults[0]
	account.server.vaults = nil
	account.server.mutex.Unlock()
	if _, err := unlockedSession.ListVaults(ctx); err != nil {
		t.Fatal(err)
	}
	if removed := unlockedSession.TakeRemovedVaults(); !reflect.DeepEqual(removed, []RemovedVault{{ID: account.vaultID, Name: "Family"}}) {
		t.Errorf("removed: %+v", removed)
	}
	if removed := unlockedSession.TakeRemovedVaults(); len(removed) != 0 {
		t.Errorf("reported twice: %+v", removed)
	}

	account.server.mutex.Lock()
	account.server.vaults = []apitypes.Vault{familyVault}
	account.server.mutex.Unlock()
	if _, err := unlockedSession.ListVaults(ctx); err != nil {
		t.Fatal(err)
	}
	unlockedSession.noteOwnRemoval(account.vaultID)
	account.server.mutex.Lock()
	account.server.vaults = nil
	account.server.mutex.Unlock()
	if _, err := unlockedSession.ListVaults(ctx); err != nil {
		t.Fatal(err)
	}
	if removed := unlockedSession.TakeRemovedVaults(); len(removed) != 0 {
		t.Errorf("the session's own deletion was reported: %+v", removed)
	}
}

func TestVaultChecksBeforeAsking(t *testing.T) {
	account := createOfflineTestAccount(t)
	ctx := context.Background()
	unlockedSession, err := Unlock(ctx, account.apiClient, nil, account.password, account.secretKey)
	if err != nil {
		t.Fatal(err)
	}
	editorVault := &Vault{ID: account.vaultID, Type: apitypes.VaultShared, Name: "Family", Role: apitypes.VaultEditor, Key: make([]byte, 32)}
	if err := unlockedSession.RenameVault(ctx, editorVault, "Renamed"); !errors.Is(err, ErrNotVaultOwner) {
		t.Errorf("editor renames: %v", err)
	}
	ownerVault := *editorVault
	ownerVault.Role = apitypes.VaultOwner
	if err := unlockedSession.RenameVault(ctx, &ownerVault, "   "); !errors.Is(err, ErrVaultNameEmpty) {
		t.Errorf("blank name: %v", err)
	}
	if _, err := unlockedSession.DeleteVault(ctx, editorVault, VaultDeletion{IsDeletingItems: true}); !errors.Is(err, ErrNotVaultOwner) {
		t.Errorf("editor deletes: %v", err)
	}
	personalVault := ownerVault
	personalVault.Type = apitypes.VaultPersonal
	if _, err := unlockedSession.DeleteVault(ctx, &personalVault, VaultDeletion{}); !errors.Is(err, ErrPersonalVault) {
		t.Errorf("personal vault deleted: %v", err)
	}
	if err := unlockedSession.LeaveVault(ctx, &personalVault); !errors.Is(err, ErrLeavePersonalVault) {
		t.Errorf("personal vault left: %v", err)
	}
	readerTarget := &Vault{ID: "other", Role: apitypes.VaultReader}
	if _, err := unlockedSession.DeleteVault(ctx, &ownerVault, VaultDeletion{MoveTo: readerTarget}); !errors.Is(err, ErrTargetNotWritable) {
		t.Errorf("moving into a read-only vault: %v", err)
	}
	if _, err := unlockedSession.DeleteVault(ctx, &ownerVault, VaultDeletion{MoveTo: &ownerVault}); !errors.Is(err, ErrTargetNotWritable) {
		t.Errorf("moving into the same vault: %v", err)
	}
}
