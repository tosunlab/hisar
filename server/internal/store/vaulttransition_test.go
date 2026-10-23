package store

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/tryhisar/hisar/core/apitypes"
)

func TestStagedRotationPublishesOnlyCompleteSnapshot(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	createTestAccount(t, database, "BBBBBB", "b@example.com")
	vaultID := createTestVault(t, database, "AAAAAA")
	if err := database.SetAccess(ctx, vaultID, Member{AccountID: "BBBBBB", Role: "editor", WrappedKey: []byte("old-b"), GrantedBy: "AAAAAA"}); err != nil {
		t.Fatal(err)
	}
	first := &Item{ID: uuid.NewString(), Revision: uuid.NewString(), Overview: []byte("old-o"), Details: []byte("old-d"), UpdatedBy: "AAAAAA"}
	second := &Item{ID: uuid.NewString(), Revision: uuid.NewString(), Deleted: true, Overview: []byte("old-tomb-o"), Details: []byte("old-tomb-d"), UpdatedBy: "AAAAAA"}
	for _, item := range []*Item{first, second} {
		if _, err := database.PutItem(ctx, vaultID, "", 1, item); err != nil {
			t.Fatal(err)
		}
	}
	begin := func() *apitypes.VaultTransition {
		t.Helper()
		op, err := database.BeginVaultTransition(ctx, vaultID, "AAAAAA", &apitypes.BeginVaultTransitionRequest{ID: uuid.NewString(), Kind: "rotate", KeyGeneration: 1})
		if err != nil {
			t.Fatal(err)
		}
		return op
	}
	stage := func(op *apitypes.VaultTransition, item *Item) apitypes.RotatedItem {
		t.Helper()
		sealed := apitypes.RotatedItem{ID: item.ID, BaseRevision: item.Revision, Revision: uuid.NewString(), Deleted: item.Deleted, Overview: []byte("new-o"), Details: []byte("new-d")}
		if err := database.StageVaultItems(ctx, vaultID, "AAAAAA", op.ID, []apitypes.RotatedItem{sealed}); err != nil {
			t.Fatal(err)
		}
		return sealed
	}
	op := begin()
	sealed := stage(op, first)
	if err := database.StageVaultItems(ctx, vaultID, "AAAAAA", op.ID, []apitypes.RotatedItem{sealed}); err != nil {
		t.Fatalf("identical upload retry: %v", err)
	}
	altered := sealed
	altered.Details = []byte("different")
	if err := database.StageVaultItems(ctx, vaultID, "AAAAAA", op.ID, []apitypes.RotatedItem{altered}); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed retry: %v", err)
	}
	if _, err := database.CommitVaultTransition(ctx, vaultID, "AAAAAA", op.ID, []byte("new-name")); !errors.Is(err, ErrConflict) {
		t.Fatalf("incomplete set committed: %v", err)
	}
	current, _, _, err := database.ListChanges(ctx, vaultID, 0, 100, 1<<20)
	if err != nil || len(current) != 2 {
		t.Fatalf("interrupted stage changed live records: %v", err)
	}
	for _, item := range current {
		if !bytes.HasPrefix(item.Overview, []byte("old")) {
			t.Fatal("staging published ciphertext")
		}
	}
	if err := database.AbortVaultTransition(ctx, vaultID, "AAAAAA", op.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CommitVaultTransition(ctx, vaultID, "AAAAAA", op.ID, []byte("new-name")); !errors.Is(err, ErrConflict) {
		t.Fatalf("aborted stage committed: %v", err)
	}

	op = begin()
	stage(op, first)
	stage(op, second)
	if err := database.StageVaultAccess(ctx, vaultID, "AAAAAA", op.ID, []apitypes.RotatedAccess{{AccountID: "AAAAAA", Role: "owner", WrappedKey: []byte("new-a")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CommitVaultTransition(ctx, vaultID, "AAAAAA", op.ID, []byte("new-name")); !errors.Is(err, ErrRotationMembers) {
		t.Fatalf("omitted current member: %v", err)
	}
	if err := database.StageVaultAccess(ctx, vaultID, "AAAAAA", op.ID, []apitypes.RotatedAccess{{AccountID: "BBBBBB", Role: "editor", WrappedKey: []byte("new-b")}}); err != nil {
		t.Fatal(err)
	}
	receipt, err := database.CommitVaultTransition(ctx, vaultID, "AAAAAA", op.ID, []byte("new-name"))
	if err != nil || receipt.State != "committed" || receipt.ItemCount != 2 {
		t.Fatalf("complete commit: %+v %v", receipt, err)
	}
	if _, err := database.CommitVaultTransition(ctx, vaultID, "AAAAAA", op.ID, []byte("new-name")); err != nil {
		t.Fatalf("commit retry: %v", err)
	}
	if err := database.AbortVaultTransition(ctx, vaultID, "AAAAAA", op.ID); err != nil {
		t.Fatal(err)
	}
	vaults, err := database.ListVaults(ctx, "BBBBBB")
	if err != nil || len(vaults) != 1 || vaults[0].KeyGeneration != 2 || string(vaults[0].WrappedKey) != "new-b" {
		t.Fatalf("member generation: %+v %v", vaults, err)
	}
	for _, item := range []*Item{first, second} {
		history, err := database.GetItemHistory(ctx, vaultID, item.ID)
		if err != nil || len(history) != 1 || string(history[0].Details) != "new-d" {
			t.Fatalf("history retained old key: %+v %v", history, err)
		}
	}
}

func TestStagedRotationRejectsConcurrentChanges(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	createTestAccount(t, database, "BBBBBB", "b@example.com")
	for _, change := range []string{"item", "name", "access", "owner"} {
		t.Run(change, func(t *testing.T) {
			vaultID := createTestVault(t, database, "AAAAAA")
			if err := database.SetAccess(ctx, vaultID, Member{AccountID: "BBBBBB", Role: "owner", WrappedKey: []byte("old-b"), GrantedBy: "AAAAAA"}); err != nil {
				t.Fatal(err)
			}
			item := &Item{ID: uuid.NewString(), Revision: uuid.NewString(), Overview: []byte("old"), Details: []byte("old"), UpdatedBy: "AAAAAA"}
			if _, err := database.PutItem(ctx, vaultID, "", 1, item); err != nil {
				t.Fatal(err)
			}
			op, err := database.BeginVaultTransition(ctx, vaultID, "AAAAAA", &apitypes.BeginVaultTransitionRequest{ID: uuid.NewString(), Kind: "rotate", KeyGeneration: 1})
			if err != nil {
				t.Fatal(err)
			}
			staged := apitypes.RotatedItem{ID: item.ID, BaseRevision: item.Revision, Revision: uuid.NewString(), Overview: []byte("stage"), Details: []byte("stage")}
			if err := database.StageVaultItems(ctx, vaultID, "AAAAAA", op.ID, []apitypes.RotatedItem{staged}); err != nil {
				t.Fatal(err)
			}
			if err := database.StageVaultAccess(ctx, vaultID, "AAAAAA", op.ID, []apitypes.RotatedAccess{{AccountID: "AAAAAA", Role: "owner", WrappedKey: []byte("new-a")}, {AccountID: "BBBBBB", Role: "owner", WrappedKey: []byte("new-b")}}); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "item":
				base := item.Revision
				item.Revision = uuid.NewString()
				item.Details = []byte("concurrent edit")
				_, err = database.PutItem(ctx, vaultID, base, 1, item)
			case "name":
				err = database.RenameVault(ctx, vaultID, "AAAAAA", []byte("concurrent name"), 1, nil)
			case "access":
				_, err = database.GrantAccess(ctx, vaultID, Member{AccountID: "BBBBBB", Role: "editor", WrappedKey: []byte("old-b"), GrantedBy: "AAAAAA"}, 1)
			case "owner":
				err = database.RemoveAccess(ctx, vaultID, "BBBBBB", "AAAAAA")
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := database.CommitVaultTransition(ctx, vaultID, "AAAAAA", op.ID, []byte("stage-name")); err == nil {
				t.Fatal("stale snapshot committed")
			}
			if _, _, err := database.ListVaultTransitionItems(ctx, vaultID, "AAAAAA", op.ID, ""); err == nil {
				t.Fatal("stale snapshot continued")
			}
			items, _, _, err := database.ListChanges(ctx, vaultID, 0, 100, 1<<20)
			if err != nil || len(items) != 1 || items[0].Revision == staged.Revision {
				t.Fatalf("failed transition published data: %+v %v", items, err)
			}
			if change == "item" && string(items[0].Details) != "concurrent edit" {
				t.Fatal("lost concurrent edit")
			}
			if err := database.AbortVaultTransition(ctx, vaultID, "AAAAAA", op.ID); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOwnerRemovalBlocksExposedGenerationUntilRotation(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	createTestAccount(t, database, "BBBBBB", "b@example.com")
	createTestAccount(t, database, "CCCCCC", "c@example.com")
	vaultID := createTestVault(t, database, "AAAAAA")
	for _, accountID := range []string{"BBBBBB", "CCCCCC"} {
		if err := database.SetAccess(ctx, vaultID, Member{AccountID: accountID, Role: "editor", WrappedKey: []byte("old"), GrantedBy: "AAAAAA"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.RemoveAccess(ctx, vaultID, "AAAAAA", "BBBBBB"); err != nil {
		t.Fatal(err)
	}
	vaults, err := database.ListVaults(ctx, "AAAAAA")
	if err != nil || !vaults[0].NeedsRotation {
		t.Fatalf("removal did not mark rotation: %+v %v", vaults, err)
	}
	item := &Item{ID: uuid.NewString(), Revision: uuid.NewString(), Overview: []byte("new"), Details: []byte("new"), UpdatedBy: "AAAAAA"}
	if _, err := database.PutItem(ctx, vaultID, "", 1, item); !errors.Is(err, ErrRotationPending) {
		t.Fatalf("exposed key write: %v", err)
	}
	if err := database.RenameVault(ctx, vaultID, "AAAAAA", []byte("new"), 1, nil); !errors.Is(err, ErrRotationPending) {
		t.Fatalf("exposed name: %v", err)
	}
	if _, err := database.GrantAccess(ctx, vaultID, Member{AccountID: "CCCCCC", Role: "reader", WrappedKey: []byte("old"), GrantedBy: "AAAAAA"}, 1); !errors.Is(err, ErrRotationPending) {
		t.Fatalf("remaining member's grant under the exposed key: %v", err)
	}
	if _, err := database.GrantAccess(ctx, vaultID, Member{AccountID: "BBBBBB", Role: "editor", WrappedKey: []byte("old"), GrantedBy: "AAAAAA"}, 1); !errors.Is(err, ErrNoInvitation) {
		t.Fatalf("removed member's grant without an invitation: %v", err)
	}
	if role, err := database.GetVaultRole(ctx, vaultID, "BBBBBB"); !errors.Is(err, ErrNotFound) || role != "" {
		t.Fatalf("removed member restored: %q %v", role, err)
	}
	if role, err := database.GetVaultRole(ctx, vaultID, "CCCCCC"); err != nil || role != "editor" {
		t.Fatalf("refused grant changed a remaining member: %q %v", role, err)
	}
}
