package store

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/tryhisar/hisar/core/apitypes"
)

func TestPersonalCreationSerializesAndLegacyExtrasRemainRecoverable(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	const owner = "AAAAAAAAAA"
	createTestAccount(t, database, owner, "personal@example.com")
	var workers sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		workers.Go(func() {
			results <- database.CreateVault(ctx, &Vault{ID: uuid.NewString(), Type: "personal", EncName: []byte("sealed"), CreatedBy: owner}, []byte("wrapped"))
		})
	}
	workers.Wait()
	close(results)
	success, conflicts := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrExists) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("concurrent primary creation: %d success %d conflicts", success, conflicts)
	}
	vaults, err := database.ListVaults(ctx, owner)
	if err != nil || len(vaults) != 1 || vaults[0].IsPersonalExtra {
		t.Fatalf("primary: %+v %v", vaults, err)
	}
	primaryID := vaults[0].ID
	extraID := uuid.NewString()
	if err := database.runInTransaction(ctx, func(transaction pgx.Tx) error {
		return insertVault(ctx, transaction, &Vault{ID: extraID, Type: "personal", EncName: []byte("unreadable legacy"), CreatedBy: owner}, []byte("unreadable legacy"))
	}); err != nil {
		t.Fatal(err)
	}
	vaults, err = database.ListVaults(ctx, owner)
	if err != nil || len(vaults) != 2 {
		t.Fatalf("legacy data disappeared: %+v %v", vaults, err)
	}
	extras := 0
	for _, vault := range vaults {
		if vault.IsPersonalExtra {
			extras++
			if vault.ID != extraID {
				t.Fatal("primary designation changed")
			}
		}
	}
	if extras != 1 {
		t.Fatalf("legacy recovery designation: %+v", vaults)
	}
	page, err := database.ReadCollectionPage(ctx, owner, "device", "/vaults", "")
	if err != nil || len(page.Records) != 2 {
		t.Fatalf("paged vault list: %+v %v", page, err)
	}
	for _, record := range page.Records {
		var listed apitypes.Vault
		if err := json.Unmarshal(record.Value, &listed); err != nil {
			t.Fatal(err)
		}
		if listed.IsPersonalExtra != (listed.ID == extraID) {
			t.Errorf("paged vault list designates %s as extra = %v", listed.ID, listed.IsPersonalExtra)
		}
	}
	otherID := uuid.NewString()
	if err := database.CreateVault(ctx, &Vault{ID: otherID, Type: "shared", EncName: []byte("sealed"), CreatedBy: owner}, []byte("wrapped")); err != nil {
		t.Fatal(err)
	}
	for vaultID, wantErr := range map[string]error{primaryID: ErrPersonalVault, extraID: nil} {
		_, err := database.BeginVaultTransition(ctx, vaultID, owner, &apitypes.BeginVaultTransitionRequest{ID: uuid.NewString(), Kind: "delete-move", KeyGeneration: 1, TargetID: otherID, TargetKeyGeneration: 1})
		if !errors.Is(err, wantErr) {
			t.Fatalf("delete-move of %s: %v, want %v", vaultID, err, wantErr)
		}
	}
	if _, err := database.DeleteVault(ctx, VaultDeletion{VaultID: primaryID, AccountID: owner, IsDeletingItems: true}); !errors.Is(err, ErrPersonalVault) {
		t.Fatalf("primary deleted: %v", err)
	}
	if _, err := database.DeleteVault(ctx, VaultDeletion{VaultID: extraID, AccountID: owner, IsDeletingItems: true}); err != nil {
		t.Fatalf("explicit legacy recovery refused: %v", err)
	}
	vaults, err = database.ListVaults(ctx, owner)
	if err != nil || len(vaults) != 2 || vaults[0].ID != primaryID {
		t.Fatalf("recovery damaged primary: %+v %v", vaults, err)
	}
}
