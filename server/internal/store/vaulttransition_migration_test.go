package store

import (
	"context"
	"testing"
)

func TestStagingRollbackRespectsEarlierMigrationOwnership(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	vaultID := createTestVault(t, database, "AAAAAA")
	rollbackTestMigrationsTo(t, database, 14)
	result, err := database.RollbackMigration(ctx)
	if err != nil || result.Source.Version != 14 {
		t.Fatalf("staging rollback: %+v %v", result, err)
	}
	var before, after int64
	if err := database.pool.QueryRow(ctx, `SELECT access_revision FROM vaults WHERE id=$1`, vaultID).Scan(&before); err != nil {
		t.Fatalf("staging rollback removed invitation authority state: %v", err)
	}
	if _, err := database.pool.Exec(ctx, `UPDATE vault_access SET wrapped_key='new-key'::bytea WHERE vault_id=$1`, vaultID); err != nil {
		t.Fatal(err)
	}
	if err := database.pool.QueryRow(ctx, `SELECT access_revision FROM vaults WHERE id=$1`, vaultID).Scan(&after); err != nil || after != before+1 {
		t.Fatalf("earlier authority trigger stopped working: %d -> %d, %v", before, after, err)
	}
}
