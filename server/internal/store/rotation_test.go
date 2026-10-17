package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestRotateVault(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	createTestAccount(t, database, "BBBBBB", "b@example.com")
	vaultID := createTestVault(t, database, "AAAAAA")
	if err := database.SetAccess(ctx, vaultID, Member{AccountID: "BBBBBB", Role: "editor", WrappedKey: []byte{1}, GrantedBy: "AAAAAA"}); err != nil {
		t.Fatal(err)
	}
	firstItem := &Item{ID: uuid.NewString(), Revision: uuid.NewString(), Overview: []byte("o1"), Details: []byte("d1"), UpdatedBy: "AAAAAA"}
	secondItem := &Item{ID: uuid.NewString(), Revision: uuid.NewString(), Overview: []byte("o2"), Details: []byte("d2"), UpdatedBy: "BBBBBB"}
	for _, vaultItem := range []*Item{firstItem, secondItem} {
		if _, err := database.PutItem(ctx, vaultID, "", 0, vaultItem); err != nil {
			t.Fatal(err)
		}
	}

	createRotation := func(rotatedItems []RotatedItem) VaultRotation {
		return VaultRotation{
			VaultID:   vaultID,
			EncName:   []byte("new-name"),
			Accesses:  []Member{{AccountID: "AAAAAA", Role: "owner", WrappedKey: []byte{9}}},
			Items:     rotatedItems,
			RotatedBy: "AAAAAA",
		}
	}
	rotateItem := func(vaultItem *Item, baseRevision string) RotatedItem {
		return RotatedItem{ID: vaultItem.ID, BaseRevision: baseRevision, Revision: uuid.NewString(), Overview: []byte("new-o"), Details: []byte("new-d")}
	}

	conflictCases := map[string][]RotatedItem{
		"stale revision": {rotateItem(firstItem, uuid.NewString()), rotateItem(secondItem, secondItem.Revision)},
		"missing item":   {rotateItem(firstItem, firstItem.Revision)},
		"extra item": {
			rotateItem(firstItem, firstItem.Revision), rotateItem(secondItem, secondItem.Revision),
			{ID: uuid.NewString(), BaseRevision: uuid.NewString(), Revision: uuid.NewString(), Overview: []byte{1}, Details: []byte{1}},
		},
	}
	for name, rotatedItems := range conflictCases {
		if err := database.RotateVault(ctx, createRotation(rotatedItems)); !errors.Is(err, ErrConflict) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if role, _ := database.GetVaultRole(ctx, vaultID, "BBBBBB"); role != "editor" {
		t.Fatal("a failed rotation changed access")
	}

	rotatedItems := []RotatedItem{rotateItem(firstItem, firstItem.Revision), rotateItem(secondItem, secondItem.Revision)}
	if err := database.RotateVault(ctx, createRotation(rotatedItems)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.GetVaultRole(ctx, vaultID, "BBBBBB"); !errors.Is(err, ErrNotFound) {
		t.Errorf("removed member still has access: %v", err)
	}
	vaults, _ := database.ListVaults(ctx, "AAAAAA")
	if len(vaults) != 1 || string(vaults[0].EncName) != "new-name" || vaults[0].WrappedKey[0] != 9 {
		t.Errorf("owner vault after rotation: %+v", vaults)
	}
	changes, vaultSeq, err := database.ListChanges(ctx, vaultID, 2, 100)
	if err != nil || len(changes) != 2 || vaultSeq != 4 {
		t.Fatalf("changes after rotation: %d items, seq %d, %v", len(changes), vaultSeq, err)
	}
	for _, changed := range changes {
		if string(changed.Overview) != "new-o" || changed.UpdatedBy != "AAAAAA" {
			t.Errorf("item not rotated: %+v", changed)
		}
	}
	histories, _ := database.GetItemHistory(ctx, vaultID, firstItem.ID)
	if len(histories) != 1 || histories[0].Revision != rotatedItems[0].Revision {
		t.Errorf("history after rotation: %+v", histories)
	}
}
