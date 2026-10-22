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
	go func() { _, err := database.UpsertShareInvitation(ctx, invitation); replayed <- err }()
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

func TestRetiredShareInvitationStaysRetiredAfterItsTombstoneIsPruned(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "owner@example.com")
	vaultID := createTestVault(t, database, "AAAAAA")
	invitation := &ShareInvitation{VaultID: vaultID, Email: "recipient@example.com", Role: "reader", InvitedBy: "AAAAAA"}
	id, err := createTestShareInvitation(t, database, ctx, invitation, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := database.CancelShareInvitation(ctx, id, "AAAAAA"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.UpsertShareInvitation(ctx, invitation); !errors.Is(err, ErrConflict) {
		t.Fatalf("a cancelled operation was recreated: %v", err)
	}
	if _, err := database.PruneExpiredInvitation(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if countQueryRow(t, database, `SELECT count(*) FROM retired_share_invitations WHERE id=$1`, id) != 1 {
		t.Fatal("a retired ID was pruned before its signed expiry")
	}

	time.Sleep(time.Until(time.Unix(invitation.Intent.Context.ExpiresAt, 0)) + 100*time.Millisecond)
	if _, err := database.PruneExpiredInvitation(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if countQueryRow(t, database, `SELECT count(*) FROM retired_share_invitations`) != 0 {
		t.Fatal("an expired retired ID was kept")
	}
	if _, err := database.UpsertShareInvitation(ctx, invitation); !errors.Is(err, ErrConflict) {
		t.Fatalf("an expired operation was recreated after its tombstone was pruned: %v", err)
	}
	if countQueryRow(t, database, `SELECT count(*) FROM share_invitations WHERE id=$1`, id) != 0 {
		t.Fatal("the retired invitation reappeared")
	}
}
