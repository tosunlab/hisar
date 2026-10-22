package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/tryhisar/hisar/core/apitypes"
)

func TestGenerationOmissionRefusedOnEveryRotatedWrite(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	createTestAccount(t, database, "BBBBBB", "b@example.com")
	rotatedID := createTestVault(t, database, "AAAAAA")
	otherID := createTestVault(t, database, "AAAAAA")
	op, err := database.BeginVaultTransition(ctx, rotatedID, "AAAAAA", &apitypes.BeginVaultTransitionRequest{ID: uuid.NewString(), Kind: "rotate", KeyGeneration: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.StageVaultAccess(ctx, rotatedID, "AAAAAA", op.ID, []apitypes.RotatedAccess{{AccountID: "AAAAAA", Role: "owner", WrappedKey: []byte("new")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CommitVaultTransition(ctx, rotatedID, "AAAAAA", op.ID, []byte("new-name")); err != nil {
		t.Fatal(err)
	}
	newItem := func() *Item {
		return &Item{ID: uuid.NewString(), Revision: uuid.NewString(), Overview: []byte("o"), Details: []byte("d"), UpdatedBy: "AAAAAA"}
	}
	for _, deleted := range []bool{false, true} {
		item := newItem()
		item.Deleted = deleted
		if _, err := database.PutItem(ctx, rotatedID, "", 0, item); !errors.Is(err, ErrKeyGenerationMissing) {
			t.Fatalf("item deleted=%v with omitted generation: %v", deleted, err)
		}
	}
	if err := database.RenameVault(ctx, rotatedID, "AAAAAA", []byte("legacy"), 0); !errors.Is(err, ErrKeyGenerationMissing) {
		t.Fatalf("rename: %v", err)
	}
	if _, err := database.GrantAccess(ctx, rotatedID, Member{AccountID: "BBBBBB", Role: "reader", WrappedKey: []byte("legacy"), GrantedBy: "AAAAAA"}, 0); !errors.Is(err, ErrKeyGenerationMissing) {
		t.Fatalf("grant: %v", err)
	}
	if err := database.RotateVault(ctx, VaultRotation{VaultID: rotatedID, RotatedBy: "AAAAAA", EncName: []byte("legacy")}); !errors.Is(err, ErrKeyGenerationMissing) {
		t.Fatalf("rotation: %v", err)
	}
	if _, err := database.DeleteVault(ctx, VaultDeletion{VaultID: rotatedID, AccountID: "AAAAAA", IsDeletingItems: true}); !errors.Is(err, ErrKeyGenerationMissing) {
		t.Fatalf("delete: %v", err)
	}
	live := newItem()
	if _, err := database.PutItem(ctx, rotatedID, "", 2, live); err != nil {
		t.Fatal(err)
	}
	otherLive := newItem()
	if _, err := database.PutItem(ctx, otherID, "", 0, otherLive); err != nil {
		t.Fatalf("unrotated legacy compatibility: %v", err)
	}
	for _, sourceRotated := range []bool{true, false} {
		source, target, sourceGen, targetGen, item := otherID, rotatedID, int64(1), int64(0), otherLive
		if sourceRotated {
			source, target, sourceGen, targetGen, item = rotatedID, otherID, 0, 1, live
		}
		move := &ItemMove{SourceVaultID: source, TargetVaultID: target, AccountID: "AAAAAA", BaseRevision: item.Revision, SourceKeyGeneration: sourceGen, TargetKeyGeneration: targetGen,
			Tombstone: Item{ID: item.ID, Revision: uuid.NewString(), Deleted: true, Overview: []byte("o"), Details: []byte("d")},
			Moved:     Item{ID: item.ID, Revision: uuid.NewString(), Overview: []byte("o"), Details: []byte("d")}}
		if _, err := database.MoveItem(ctx, move); !errors.Is(err, ErrKeyGenerationMissing) {
			t.Fatalf("move sourceRotated=%v: %v", sourceRotated, err)
		}
	}
	if _, err := database.DeleteVault(ctx, VaultDeletion{VaultID: otherID, AccountID: "AAAAAA", MoveToVaultID: rotatedID, MoveToKeyGeneration: 0,
		MovedItems: []MovedItem{{ID: otherLive.ID, BaseRevision: otherLive.Revision, Revision: uuid.NewString(), Overview: []byte("o"), Details: []byte("d")}}}); !errors.Is(err, ErrKeyGenerationMissing) {
		t.Fatalf("delete move: %v", err)
	}
}
