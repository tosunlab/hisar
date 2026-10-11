package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestDeleteVaultRules(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	createTestAccount(t, database, "BBBBBB", "b@example.com")
	onlyVaultID := createTestVault(t, database, "AAAAAA")
	if _, err := database.DeleteVault(ctx, VaultDeletion{VaultID: onlyVaultID, AccountID: "AAAAAA"}); !errors.Is(err, ErrOnlyVault) {
		t.Errorf("deleting the only vault: %v", err)
	}

	targetID := createTestVault(t, database, "AAAAAA")
	if err := database.SetAccess(ctx, onlyVaultID, Member{AccountID: "BBBBBB", Role: "editor", WrappedKey: []byte{1}, GrantedBy: "AAAAAA"}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.DeleteVault(ctx, VaultDeletion{VaultID: onlyVaultID, AccountID: "BBBBBB", IsDeletingItems: true}); !errors.Is(err, ErrNotOwner) {
		t.Errorf("an editor deletes: %v", err)
	}
	liveItem := &Item{ID: uuid.NewString(), Revision: uuid.NewString(), Overview: []byte("o"), Details: []byte("d"), UpdatedBy: "AAAAAA"}
	deletedItem := &Item{ID: uuid.NewString(), Revision: uuid.NewString(), Deleted: true, Overview: []byte("o"), Details: []byte("d"), UpdatedBy: "AAAAAA"}
	for _, vaultItem := range []*Item{liveItem, deletedItem} {
		if _, err := database.PutItem(ctx, onlyVaultID, "", vaultItem); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.DeleteVault(ctx, VaultDeletion{VaultID: onlyVaultID, AccountID: "AAAAAA"}); !errors.Is(err, ErrVaultNotEmpty) {
		t.Errorf("deleting a vault with items without a choice: %v", err)
	}
	moveTo := func(baseRevision string) VaultDeletion {
		return VaultDeletion{VaultID: onlyVaultID, AccountID: "AAAAAA", MoveToVaultID: targetID, AllMovedItem: []MovedItem{
			{ID: liveItem.ID, BaseRevision: baseRevision, Revision: uuid.NewString(), Overview: []byte("new-o"), Details: []byte("new-d")},
		}}
	}
	if _, err := database.DeleteVault(ctx, moveTo(uuid.NewString())); !errors.Is(err, ErrConflict) {
		t.Errorf("moving a stale revision: %v", err)
	}
	if role, _ := database.GetVaultRole(ctx, onlyVaultID, "BBBBBB"); role != "editor" {
		t.Fatal("a failed deletion changed the vault")
	}
	allFormerMember, err := database.DeleteVault(ctx, moveTo(liveItem.Revision))
	if err != nil || len(allFormerMember) != 2 {
		t.Fatalf("move and delete: %v %v", allFormerMember, err)
	}
	if _, err := database.GetVaultRole(ctx, onlyVaultID, "AAAAAA"); !errors.Is(err, ErrNotFound) {
		t.Errorf("the deleted vault still has access grants: %v", err)
	}
	allMoved, targetSeq, err := database.ListAllChange(ctx, targetID, 0, 100)
	if err != nil || len(allMoved) != 1 || string(allMoved[0].Overview) != "new-o" || targetSeq != 1 {
		t.Errorf("target after the move: %+v seq %d %v", allMoved, targetSeq, err)
	}
	if allHistory, _ := database.GetItemHistory(ctx, onlyVaultID, liveItem.ID); len(allHistory) != 0 {
		t.Errorf("history of the deleted vault remains: %d", len(allHistory))
	}
}

func TestLeaveVault(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	createTestAccount(t, database, "BBBBBB", "b@example.com")
	vaultID := createTestVault(t, database, "AAAAAA")
	if err := database.LeaveVault(ctx, vaultID, "AAAAAA"); !errors.Is(err, ErrOnlyOwner) {
		t.Errorf("the only owner leaves: %v", err)
	}
	if err := database.LeaveVault(ctx, vaultID, "BBBBBB"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a stranger leaves: %v", err)
	}
	if err := database.SetAccess(ctx, vaultID, Member{AccountID: "BBBBBB", Role: "owner", WrappedKey: []byte{1}, GrantedBy: "AAAAAA"}); err != nil {
		t.Fatal(err)
	}
	if err := database.LeaveVault(ctx, vaultID, "AAAAAA"); err != nil {
		t.Errorf("an owner leaves while another remains: %v", err)
	}
	if err := database.RenameVault(ctx, vaultID, []byte("renamed")); err != nil {
		t.Fatal(err)
	}
	if allVault, _ := database.ListAllVault(ctx, "BBBBBB"); len(allVault) != 1 || string(allVault[0].EncName) != "renamed" {
		t.Errorf("after leave and rename: %+v", allVault)
	}
	if err := database.RenameVault(ctx, uuid.NewString(), []byte("x")); !errors.Is(err, ErrNotFound) {
		t.Errorf("renaming a missing vault: %v", err)
	}
}
