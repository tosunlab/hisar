package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDeletionAsksForReviewOfAVaultGrantedDuringItsVaultWait(t *testing.T) {
	database := createTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, session := createTestAccount(t, database, "ALICE1", "alice@example.com")
	createTestAccount(t, database, "BOB111", "bob@example.com")
	firstVault := createTestVault(t, database, "ALICE1")
	laterVault := createTestVault(t, database, "BOB111")
	invitationID, err := createTestShareInvitation(t, database, ctx, &ShareInvitation{VaultID: laterVault, Email: "alice@example.com", Role: "owner", InvitedBy: "BOB111"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := database.AcceptShareInvitation(ctx, invitationID, "ALICE1", "alice@example.com"); err != nil {
		t.Fatal(err)
	}
	completion := authorizeTestShareCompletion(t, database, invitationID)
	firstHold, err := database.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer firstHold.Rollback(context.Background())
	if _, err := firstHold.Exec(ctx, `SELECT 1 FROM vaults WHERE id = $1 FOR UPDATE`, firstVault); err != nil {
		t.Fatal(err)
	}
	deleted := make(chan error, 1)
	go func() {
		_, err := database.DeleteAccount(ctx, "ALICE1", session.TokenHash, testRecordHash(), reviewStoreDeletion(t, database, ctx, "ALICE1"))
		deleted <- err
	}()
	waitForStoreLock(t, database, ctx, "%FROM vaults WHERE id IN (SELECT vault_id%")
	if err := database.CompleteSignedShare(ctx, "BOB111", "https://hisar.test", completion); err != nil {
		t.Fatal(err)
	}
	laterHold, err := database.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer laterHold.Rollback(context.Background())
	if _, err := laterHold.Exec(ctx, `SELECT 1 FROM vaults WHERE id = $1 FOR UPDATE`, laterVault); err != nil {
		t.Fatal(err)
	}
	if _, err := laterHold.Exec(ctx, `DELETE FROM vault_access WHERE vault_id = $1 AND account_id = 'BOB111'`, laterVault); err != nil {
		t.Fatal(err)
	}
	if err := firstHold.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := laterHold.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-deleted; !errors.Is(err, ErrDeletionPlanChanged) {
		t.Fatalf("deletion reviewed before the grant: %v", err)
	}
	if countQueryRow(t, database, `SELECT count(*) FROM vault_access WHERE vault_id = $1 AND account_id = 'ALICE1' AND role = 'owner'`, laterVault) != 1 {
		t.Fatal("a refused deletion removed the granted owner")
	}
	if _, err := database.DeleteAccount(ctx, "ALICE1", session.TokenHash, testRecordHash(), reviewStoreDeletion(t, database, ctx, "ALICE1")); err != nil {
		t.Fatalf("deletion reviewed after the grant: %v", err)
	}
	if countQueryRow(t, database, `SELECT count(*) FROM vaults WHERE id = $1`, laterVault) != 0 {
		t.Error("a vault granted during deletion was left without an owner")
	}
	if countQueryRow(t, database, `SELECT count(*) FROM accounts WHERE id = 'ALICE1'`) != 0 {
		t.Error("account deletion did not complete")
	}
}
