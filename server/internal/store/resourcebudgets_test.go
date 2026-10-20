package store

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/tryhisar/hisar/core/apitypes"
	"testing"
)

func TestLegacyOverBudgetInventoryCanReadAndDeleteButNotGrow(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "owner", "owner@example.com")
	if _, err := database.RollbackMigration(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := database.pool.Exec(ctx, `INSERT INTO vaults(id,type,enc_name,created_by) SELECT gen_random_uuid(),'shared','x'::bytea,'owner' FROM generate_series(1,1025);
 INSERT INTO vault_access(vault_id,account_id,wrapped_key,role,granted_by) SELECT id,'owner','x'::bytea,'owner','owner' FROM vaults`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	cursor := ""
	count := 0
	pages := 0
	first := ""
	for {
		page, err := database.ReadCollectionPage(ctx, "owner", "device", "/vaults", cursor)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		count += len(page.Records)
		if first == "" && len(page.Records) > 0 {
			var vault apitypes.Vault
			if err := json.Unmarshal(page.Records[0].Value, &vault); err != nil {
				t.Fatal(err)
			}
			first = vault.ID
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if count != 1025 || pages < 2 {
		t.Fatalf("legacy traversal count=%d pages=%d", count, pages)
	}
	create := func() error {
		return database.CreateVault(ctx, &Vault{ID: uuid.NewString(), Type: apitypes.VaultShared, EncName: []byte{1}, CreatedBy: "owner"}, []byte{1})
	}
	if err := create(); !IsResourceBudget(err) {
		t.Fatalf("growth was not refused: %v", err)
	}
	if _, err := database.pool.Exec(ctx, `DELETE FROM vaults WHERE id=$1`, first); err != nil {
		t.Fatal(err)
	}
	if err := create(); !IsResourceBudget(err) {
		t.Fatalf("still-full legacy inventory grew: %v", err)
	}
	if _, err := database.pool.Exec(ctx, `DELETE FROM vaults WHERE id=(SELECT id FROM vaults LIMIT 1)`); err != nil {
		t.Fatal(err)
	}
	if err := create(); err != nil {
		t.Fatalf("freed budget was not reusable: %v", err)
	}
	var actual, charged int
	if err := database.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM vaults WHERE created_by='owner'),(SELECT used FROM resource_usage WHERE resource='vaults' AND scope='owner')`).Scan(&actual, &charged); err != nil {
		t.Fatal(err)
	}
	if actual != 1024 || charged != actual {
		t.Fatalf("rows=%d charged=%d", actual, charged)
	}
	t.Logf("legacy over-budget smoke traversed=%d pages=%d; refused growth, deleted safely, reused one freed slot", count, pages)
}

func TestCollectionCursorRejectsMutationAndCrossAccountReuse(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "owner", "owner@example.com")
	createTestAccount(t, database, "other", "other@example.com")
	for range 101 {
		createTestVault(t, database, "owner")
	}
	first, err := database.ReadCollectionPage(ctx, "owner", "device", "/vaults", "")
	if err != nil || first.NextCursor == "" {
		t.Fatalf("first page: %v", err)
	}
	// An unrelated account's private write does not invalidate this inventory.
	createTestVault(t, database, "other")
	if _, err := database.ReadCollectionPage(ctx, "owner", "device", "/vaults", first.NextCursor); err != nil {
		t.Fatalf("unrelated scope caused restart: %v", err)
	}
	createTestVault(t, database, "owner")
	if _, err := database.ReadCollectionPage(ctx, "owner", "device", "/vaults", first.NextCursor); !errors.Is(err, ErrCollectionChanged) {
		t.Fatalf("mixed snapshot accepted: %v", err)
	}
	if _, err := database.ReadCollectionPage(ctx, "other", "device", "/vaults", first.NextCursor); !errors.Is(err, ErrConflict) {
		t.Fatalf("cross-account cursor accepted: %v", err)
	}
}
