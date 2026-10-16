package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestMoveItemRules(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	createTestAccount(t, database, "BBBBBB", "b@example.com")
	sourceID := createTestVault(t, database, "AAAAAA")
	targetID := createTestVault(t, database, "AAAAAA")
	liveItem := &Item{ID: uuid.NewString(), Revision: uuid.NewString(), Overview: []byte("o"), Details: []byte("d"), UpdatedBy: "AAAAAA"}
	if _, err := database.PutItem(ctx, sourceID, "", liveItem); err != nil {
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

	// A live item with the same ID in the target stops the move.
	clash := &Item{ID: liveItem.ID, Revision: uuid.NewString(), Overview: []byte("o"), Details: []byte("d"), UpdatedBy: "AAAAAA"}
	if _, err := database.PutItem(ctx, targetID, "", clash); err != nil {
		t.Fatal(err)
	}
	if _, err := database.MoveItem(ctx, buildMove("AAAAAA", liveItem.Revision)); !errors.Is(err, ErrExists) {
		t.Errorf("moving onto a live item: %v", err)
	}
	if _, err := database.PutItem(ctx, targetID, clash.Revision, &Item{ID: clash.ID, Revision: uuid.NewString(), Deleted: true, Overview: []byte("t"), Details: []byte("t"), UpdatedBy: "AAAAAA"}); err != nil {
		t.Fatal(err)
	}

	move := buildMove("AAAAAA", liveItem.Revision)
	if _, err := database.MoveItem(ctx, move); err != nil {
		t.Fatalf("move onto a tombstone: %v", err)
	}
	sourceItems, sourceSeq, err := database.ListChanges(ctx, sourceID, 0, 100)
	if err != nil || len(sourceItems) != 1 || !sourceItems[0].Deleted || sourceSeq != move.Tombstone.Seq {
		t.Errorf("source after the move: %+v seq %d %v", sourceItems, sourceSeq, err)
	}
	targetItems, targetSeq, err := database.ListChanges(ctx, targetID, 0, 100)
	if err != nil || len(targetItems) != 1 || targetItems[0].Deleted || string(targetItems[0].Overview) != "moved-o" || targetSeq != move.Moved.Seq {
		t.Errorf("target after the move: %+v seq %d %v", targetItems, targetSeq, err)
	}
	if histories, _ := database.GetItemHistory(ctx, targetID, liveItem.ID); len(histories) != 3 {
		t.Errorf("target history = %d revisions", len(histories))
	}
}
