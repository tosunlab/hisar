package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestMoveItemRules(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	createTestAccount(t, database, "BBBBBB", "b@example.com")
	sourceID := createTestVault(t, database, "AAAAAA")
	targetID := createTestVault(t, database, "AAAAAA")
	liveItem := &Item{ID: uuid.NewString(), Revision: uuid.NewString(), Overview: []byte("o"), Details: []byte("d"), UpdatedBy: "AAAAAA"}
	if _, err := database.PutItem(ctx, sourceID, "", 0, liveItem); err != nil {
		t.Fatal(err)
	}
	if err := database.SetAccess(ctx, sourceID, Member{AccountID: "BBBBBB", Role: "editor", WrappedKey: []byte{1}, GrantedBy: "AAAAAA"}); err != nil {
		t.Fatal(err)
	}
	buildMove := func(accountID, baseRevision string) *ItemMove {
		return &ItemMove{
			SourceVaultID: sourceID, TargetVaultID: targetID, AccountID: accountID, BaseRevision: baseRevision,
			Tombstone: Item{Revision: uuid.NewString(), Deleted: true, Overview: []byte("t"), Details: []byte("t")},
			Moved:     Item{ID: liveItem.ID, Revision: uuid.NewString(), Overview: []byte("moved-o"), Details: []byte("moved-d")},
		}
	}

	if current, err := database.MoveItem(ctx, buildMove("AAAAAA", uuid.NewString())); !errors.Is(err, ErrConflict) || current == nil || current.Revision != liveItem.Revision {
		t.Errorf("stale move: %+v %v", current, err)
	}
	if _, err := database.MoveItem(ctx, buildMove("BBBBBB", liveItem.Revision)); !errors.Is(err, ErrNotFound) {
		t.Errorf("a stranger moves into the target: %v", err)
	}
	if err := database.SetAccess(ctx, targetID, Member{AccountID: "BBBBBB", Role: "reader", WrappedKey: []byte{1}, GrantedBy: "AAAAAA"}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.MoveItem(ctx, buildMove("BBBBBB", liveItem.Revision)); !errors.Is(err, ErrTargetNotWritable) {
		t.Errorf("a reader moves into the target: %v", err)
	}
	if err := database.SetAccess(ctx, sourceID, Member{AccountID: "BBBBBB", Role: "reader", WrappedKey: []byte{1}, GrantedBy: "AAAAAA"}); err != nil {
		t.Fatal(err)
	}
	if err := database.SetAccess(ctx, targetID, Member{AccountID: "BBBBBB", Role: "editor", WrappedKey: []byte{1}, GrantedBy: "AAAAAA"}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.MoveItem(ctx, buildMove("BBBBBB", liveItem.Revision)); !errors.Is(err, ErrReadOnly) {
		t.Errorf("a reader moves out of the source: %v", err)
	}

	// A live item with the same ID in the target stops the move.
	clash := &Item{ID: liveItem.ID, Revision: uuid.NewString(), Overview: []byte("o"), Details: []byte("d"), UpdatedBy: "AAAAAA"}
	if _, err := database.PutItem(ctx, targetID, "", 0, clash); err != nil {
		t.Fatal(err)
	}
	if _, err := database.MoveItem(ctx, buildMove("AAAAAA", liveItem.Revision)); !errors.Is(err, ErrExists) {
		t.Errorf("moving onto a live item: %v", err)
	}
	if _, err := database.PutItem(ctx, targetID, clash.Revision, 0, &Item{ID: clash.ID, Revision: uuid.NewString(), Deleted: true, Overview: []byte("t"), Details: []byte("t"), UpdatedBy: "AAAAAA"}); err != nil {
		t.Fatal(err)
	}

	move := buildMove("AAAAAA", liveItem.Revision)
	if _, err := database.MoveItem(ctx, move); err != nil {
		t.Fatalf("move onto a tombstone: %v", err)
	}
	sourceItems, sourceSeq, _, err := database.ListChanges(ctx, sourceID, 0, 100, 1<<20)
	if err != nil || len(sourceItems) != 1 || !sourceItems[0].Deleted || sourceSeq != move.Tombstone.Seq {
		t.Errorf("source after the move: %+v seq %d %v", sourceItems, sourceSeq, err)
	}
	targetItems, targetSeq, _, err := database.ListChanges(ctx, targetID, 0, 100, 1<<20)
	if err != nil || len(targetItems) != 1 || targetItems[0].Deleted || string(targetItems[0].Overview) != "moved-o" || targetSeq != move.Moved.Seq {
		t.Errorf("target after the move: %+v seq %d %v", targetItems, targetSeq, err)
	}
	if histories, _ := database.GetItemHistory(ctx, targetID, liveItem.ID); len(histories) != 3 {
		t.Errorf("target history = %d revisions", len(histories))
	}
}

func TestVaultChangesAreCheckedAgainstAccessRemovedMeanwhile(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	createTestAccount(t, database, "BBBBBB", "b@example.com")
	vaultID := createTestVault(t, database, "AAAAAA")
	otherVaultID := createTestVault(t, database, "BBBBBB")
	liveItem := &Item{ID: uuid.NewString(), Revision: uuid.NewString(), Overview: []byte("o"), Details: []byte("d"), UpdatedBy: "AAAAAA"}
	if _, err := database.PutItem(ctx, vaultID, "", 0, liveItem); err != nil {
		t.Fatal(err)
	}
	grantBob := func(role string) {
		t.Helper()
		if err := database.SetAccess(ctx, vaultID, Member{AccountID: "BBBBBB", Role: role, WrappedKey: []byte{1}, GrantedBy: "AAAAAA"}); err != nil {
			t.Fatal(err)
		}
	}
	removeBobMeanwhile := func(change func() error) error {
		t.Helper()
		return runWhileHolding(t, database, func(transaction pgx.Tx) error {
			if _, err := lockVault(ctx, transaction, vaultID); err != nil {
				return err
			}
			_, err := transaction.Exec(ctx, `DELETE FROM vault_access WHERE vault_id = $1 AND account_id = 'BBBBBB'`, vaultID)
			return err
		}, change)
	}

	grantBob("editor")
	written := &Item{ID: uuid.NewString(), Revision: uuid.NewString(), Overview: []byte("o"), Details: []byte("d"), UpdatedBy: "BBBBBB"}
	if err := removeBobMeanwhile(func() error { _, err := database.PutItem(ctx, vaultID, "", 0, written); return err }); !errors.Is(err, ErrNotFound) {
		t.Errorf("an item write by an editor removed meanwhile: %v", err)
	}
	grantBob("editor")
	move := &ItemMove{
		SourceVaultID: vaultID, TargetVaultID: otherVaultID, AccountID: "BBBBBB", BaseRevision: liveItem.Revision,
		Tombstone: Item{Revision: uuid.NewString(), Deleted: true, Overview: []byte("t"), Details: []byte("t")},
		Moved:     Item{ID: liveItem.ID, Revision: uuid.NewString(), Overview: []byte("moved-o"), Details: []byte("moved-d")},
	}
	if err := removeBobMeanwhile(func() error { _, err := database.MoveItem(ctx, move); return err }); !errors.Is(err, ErrNotFound) {
		t.Errorf("an item move by an editor removed meanwhile: %v", err)
	}
	grantBob("owner")
	if err := removeBobMeanwhile(func() error { return database.RenameVault(ctx, vaultID, "BBBBBB", []byte("renamed"), 0) }); !errors.Is(err, ErrNotFound) {
		t.Errorf("a rename by an owner removed meanwhile: %v", err)
	}
	grantBob("owner")
	if err := removeBobMeanwhile(func() error { return database.RemoveAccess(ctx, vaultID, "BBBBBB", "AAAAAA") }); !errors.Is(err, ErrNotFound) {
		t.Errorf("an owner removed meanwhile removes the other owner: %v", err)
	}

	if items, _, _, err := database.ListChanges(ctx, vaultID, 0, 100, 1<<20); err != nil || len(items) != 1 || items[0].Revision != liveItem.Revision {
		t.Errorf("the vault's items: %+v %v", items, err)
	}
	if vaults, _ := database.ListVaults(ctx, "AAAAAA"); len(vaults) != 1 || string(vaults[0].EncName) != string([]byte{1}) || vaults[0].Role != "owner" {
		t.Errorf("the remaining owner's vault: %+v", vaults)
	}
}
