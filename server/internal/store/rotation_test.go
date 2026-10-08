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
		if _, err := database.PutItem(ctx, vaultID, "", vaultItem); err != nil {
			t.Fatal(err)
		}
	}

	createRotation := func(allRotatedItem []RotatedItem) VaultRotation {
		return VaultRotation{
			VaultID:   vaultID,
			EncName:   []byte("new-name"),
			AllAccess: []Member{{AccountID: "AAAAAA", Role: "owner", WrappedKey: []byte{9}}},
			AllItem:   allRotatedItem,
			RotatedBy: "AAAAAA",
		}
	}
	rotateItem := func(vaultItem *Item, baseRevision string) RotatedItem {
		return RotatedItem{ID: vaultItem.ID, BaseRevision: baseRevision, Revision: uuid.NewString(), Overview: []byte("new-o"), Details: []byte("new-d")}
	}

	allConflictCase := map[string][]RotatedItem{
		"stale revision": {rotateItem(firstItem, uuid.NewString()), rotateItem(secondItem, secondItem.Revision)},
		"missing item":   {rotateItem(firstItem, firstItem.Revision)},
		"extra item": {
			rotateItem(firstItem, firstItem.Revision), rotateItem(secondItem, secondItem.Revision),
			{ID: uuid.NewString(), BaseRevision: uuid.NewString(), Revision: uuid.NewString(), Overview: []byte{1}, Details: []byte{1}},
		},
	}
	for name, allRotatedItem := range allConflictCase {
		if err := database.RotateVault(ctx, createRotation(allRotatedItem)); !errors.Is(err, ErrConflict) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if role, _ := database.GetVaultRole(ctx, vaultID, "BBBBBB"); role != "editor" {
		t.Fatal("a failed rotation changed access")
	}

	allRotatedItem := []RotatedItem{rotateItem(firstItem, firstItem.Revision), rotateItem(secondItem, secondItem.Revision)}
	if err := database.RotateVault(ctx, createRotation(allRotatedItem)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.GetVaultRole(ctx, vaultID, "BBBBBB"); !errors.Is(err, ErrNotFound) {
		t.Errorf("removed member still has access: %v", err)
	}
	allVault, _ := database.ListAllVault(ctx, "AAAAAA")
	if len(allVault) != 1 || string(allVault[0].EncName) != "new-name" || allVault[0].WrappedKey[0] != 9 {
		t.Errorf("owner vault after rotation: %+v", allVault)
	}
	allChange, vaultSeq, err := database.ListAllChange(ctx, vaultID, 2, 100)
	if err != nil || len(allChange) != 2 || vaultSeq != 4 {
		t.Fatalf("changes after rotation: %d items, seq %d, %v", len(allChange), vaultSeq, err)
	}
	for _, changed := range allChange {
		if string(changed.Overview) != "new-o" || changed.UpdatedBy != "AAAAAA" {
			t.Errorf("item not rotated: %+v", changed)
		}
	}
	allHistory, _ := database.GetItemHistory(ctx, vaultID, firstItem.ID)
	if len(allHistory) != 1 || allHistory[0].Revision != allRotatedItem[0].Revision {
		t.Errorf("history after rotation: %+v", allHistory)
	}
}
