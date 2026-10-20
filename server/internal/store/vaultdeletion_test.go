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
		if _, err := database.PutItem(ctx, onlyVaultID, "", 0, vaultItem); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.DeleteVault(ctx, VaultDeletion{VaultID: onlyVaultID, AccountID: "AAAAAA"}); !errors.Is(err, ErrVaultNotEmpty) {
		t.Errorf("deleting a vault with items without a choice: %v", err)
	}
	moveTo := func(baseRevision string) VaultDeletion {
		return VaultDeletion{VaultID: onlyVaultID, AccountID: "AAAAAA", MoveToVaultID: targetID, MovedItems: []MovedItem{
			{ID: liveItem.ID, BaseRevision: baseRevision, Revision: uuid.NewString(), Overview: []byte("new-o"), Details: []byte("new-d")},
		}}
	}
	if _, err := database.DeleteVault(ctx, moveTo(uuid.NewString())); !errors.Is(err, ErrConflict) {
		t.Errorf("moving a stale revision: %v", err)
	}
	if role, _ := database.GetVaultRole(ctx, onlyVaultID, "BBBBBB"); role != "editor" {
		t.Fatal("a failed deletion changed the vault")
	}
	formerMembers, err := database.DeleteVault(ctx, moveTo(liveItem.Revision))
	if err != nil || len(formerMembers) != 2 {
		t.Fatalf("move and delete: %v %v", formerMembers, err)
	}
	if _, err := database.GetVaultRole(ctx, onlyVaultID, "AAAAAA"); !errors.Is(err, ErrNotFound) {
		t.Errorf("the deleted vault still has access grants: %v", err)
	}
	movedItems, targetSeq, _, err := database.ListChanges(ctx, targetID, 0, 100, 1<<20)
	if err != nil || len(movedItems) != 1 || string(movedItems[0].Overview) != "new-o" || targetSeq != 1 {
		t.Errorf("target after the move: %+v seq %d %v", movedItems, targetSeq, err)
	}
	if histories, _ := database.GetItemHistory(ctx, onlyVaultID, liveItem.ID); len(histories) != 0 {
		t.Errorf("history of the deleted vault remains: %d", len(histories))
	}
}

// TestDeleteVaultMovesItemsOverTargetTombstones: an item that once left the target vault leaves a tombstone there, and
// moving it back with a vault deletion replaces that tombstone; only a live item with the same ID refuses the move.
func TestDeleteVaultMovesItemsOverTargetTombstones(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	sourceID := createTestVault(t, database, "AAAAAA")
	targetID := createTestVault(t, database, "AAAAAA")
	putItem := func(vaultID, baseRevision string, vaultItem *Item) {
		t.Helper()
		if _, err := database.PutItem(ctx, vaultID, baseRevision, 0, vaultItem); err != nil {
			t.Fatal(err)
		}
	}
	returningID, collidingID := uuid.NewString(), uuid.NewString()
	left := &Item{ID: returningID, Revision: uuid.NewString(), Overview: []byte("o"), Details: []byte("d"), UpdatedBy: "AAAAAA"}
	putItem(targetID, "", left)
	putItem(targetID, left.Revision, &Item{ID: returningID, Revision: uuid.NewString(), Deleted: true, Overview: []byte("t"), Details: []byte("t"), UpdatedBy: "AAAAAA"})
	returning := &Item{ID: returningID, Revision: uuid.NewString(), Overview: []byte("o"), Details: []byte("d"), UpdatedBy: "AAAAAA"}
	putItem(sourceID, "", returning)
	moveBack := VaultDeletion{VaultID: sourceID, AccountID: "AAAAAA", MoveToVaultID: targetID, MovedItems: []MovedItem{
		{ID: returningID, BaseRevision: returning.Revision, Revision: uuid.NewString(), Overview: []byte("moved-o"), Details: []byte("moved-d")},
	}}

	colliding := &Item{ID: collidingID, Revision: uuid.NewString(), Overview: []byte("o"), Details: []byte("d"), UpdatedBy: "AAAAAA"}
	putItem(targetID, "", &Item{ID: collidingID, Revision: uuid.NewString(), Overview: []byte("o"), Details: []byte("d"), UpdatedBy: "AAAAAA"})
	putItem(sourceID, "", colliding)
	withCollision := moveBack
	withCollision.MovedItems = append([]MovedItem{{ID: collidingID, BaseRevision: colliding.Revision, Revision: uuid.NewString(), Overview: []byte("o"), Details: []byte("d")}}, moveBack.MovedItems...)
	if _, err := database.DeleteVault(ctx, withCollision); !errors.Is(err, ErrExists) {
		t.Fatalf("moving over a live item: %v", err)
	}
	putItem(sourceID, colliding.Revision, &Item{ID: collidingID, Revision: uuid.NewString(), Deleted: true, Overview: []byte("t"), Details: []byte("t"), UpdatedBy: "AAAAAA"})

	if _, err := database.DeleteVault(ctx, moveBack); err != nil {
		t.Fatalf("moving over a tombstone: %v", err)
	}
	items, targetSeq, _, err := database.ListChanges(ctx, targetID, 0, 100, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	var moved *Item
	for index := range items {
		if items[index].ID == returningID {
			moved = &items[index]
		}
	}
	if moved == nil || moved.Deleted || string(moved.Overview) != "moved-o" || moved.Seq != targetSeq || targetSeq != 4 {
		t.Errorf("the returned item in the target: %+v, target seq %d", moved, targetSeq)
	}
	if histories, _ := database.GetItemHistory(ctx, targetID, returningID); len(histories) != 3 {
		t.Errorf("history of the returned item: %d entries", len(histories))
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
	if err := database.RenameVault(ctx, vaultID, []byte("renamed"), 0); err != nil {
		t.Fatal(err)
	}
	if vaults, _ := database.ListVaults(ctx, "BBBBBB"); len(vaults) != 1 || string(vaults[0].EncName) != "renamed" {
		t.Errorf("after leave and rename: %+v", vaults)
	}
	if err := database.RenameVault(ctx, uuid.NewString(), []byte("x"), 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("renaming a missing vault: %v", err)
	}
}
