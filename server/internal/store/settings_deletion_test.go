package store

import (
	"context"
	"testing"
	"time"
)

func TestSettingsSaveDuringAdminDeletion(t *testing.T) {
	database := createTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	createTestAccount(t, database, "ADMIN1", "admin@example.com")
	createTestAccount(t, database, "ALICE1", "alice@example.com")
	createTestAccount(t, database, "CAROL1", "carol@example.com")
	if _, err := database.pool.Exec(ctx, `UPDATE accounts SET role = 'admin' WHERE id IN ('ADMIN1', 'CAROL1')`); err != nil {
		t.Fatal(err)
	}
	if err := database.UpdateInstanceSettings(ctx, "ALICE1", false, func(document []byte) ([]byte, error) { return document, nil }); err != nil {
		t.Fatal(err)
	}
	vaultID := createTestVault(t, database, "ALICE1")
	barrier, err := database.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer barrier.Rollback(context.Background())
	if _, err := barrier.Exec(ctx, `SELECT 1 FROM vaults WHERE id = $1 FOR UPDATE`, vaultID); err != nil {
		t.Fatal(err)
	}
	deleted := make(chan error, 1)
	go func() {
		_, err := database.DeleteAccountAsAdmin(ctx, AdminActor{AccountID: "ADMIN1"}, "ALICE1")
		deleted <- err
	}()
	waitForStoreLock(t, database, ctx, "%FROM vaults WHERE id IN (SELECT vault_id%")
	saved := make(chan error, 1)
	go func() {
		saved <- database.UpdateInstanceSettings(ctx, "CAROL1", true, func(document []byte) ([]byte, error) { return document, nil })
	}()
	waitForStoreLock(t, database, ctx, "%FROM accounts WHERE id = $1 FOR SHARE%")
	if err := barrier.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-deleted; err != nil {
		t.Errorf("admin deletion: %v", err)
	}
	if err := <-saved; err != nil {
		t.Errorf("settings save: %v", err)
	}
}
