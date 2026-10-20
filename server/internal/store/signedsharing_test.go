package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCancelledShareCannotReappearAfterSourceLockWait(t *testing.T) {
	database := createTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	createTestAccount(t, database, "AAAAAA", "owner@example.com")
	vaultID := createTestVault(t, database, "AAAAAA")
	invitation := &ShareInvitation{VaultID: vaultID, Email: "recipient@example.com", Role: "reader", InvitedBy: "AAAAAA"}
	id, err := createTestShareInvitation(t, database, ctx, invitation, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	sourceDelete, err := database.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer sourceDelete.Rollback(context.Background())
	if _, err := sourceDelete.Exec(ctx, `DELETE FROM share_invitations WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	replayed := make(chan error, 1)
	go func() { _, err := database.UpsertShareInvitation(ctx, invitation, time.Hour); replayed <- err }()
	waitForStoreLock(t, database, ctx, "%SELECT intent FROM share_invitations WHERE id=%")
	if err := sourceDelete.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-replayed; !errors.Is(err, ErrConflict) {
		t.Fatalf("cancellation resurrected by concurrent replay: %v", err)
	}
	if countQueryRow(t, database, `SELECT count(*) FROM share_invitations WHERE id=$1`, id) != 0 {
		t.Fatal("retired invitation reappeared")
	}
}
