package store

import (
	"context"
	"testing"
	"time"
)

func TestDeletionLocksVaultGrantedDuringInitialVaultWait(t *testing.T) {
	database := createTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, session := createTestAccount(t, database, "ALICE1", "alice@example.com")
	createTestAccount(t, database, "BOB111", "bob@example.com")
	firstVault := createTestVault(t, database, "ALICE1")
	laterVault := createTestVault(t, database, "BOB111")
	invitationID, err := database.UpsertShareInvitation(ctx, &ShareInvitation{VaultID: laterVault, Email: "alice@example.com", Role: "owner", InvitedBy: "BOB111"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := database.AcceptShareInvitation(ctx, invitationID, "ALICE1", "alice@example.com"); err != nil {
		t.Fatal(err)
	}
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
		_, err := database.DeleteAccount(ctx, "ALICE1", session.TokenHash, testRecordHash())
		deleted <- err
	}()
	waitForStoreLock(t, database, ctx, "%FROM vaults WHERE id IN (SELECT vault_id%")
	if _, err := database.GrantAccess(ctx, laterVault, Member{AccountID: "ALICE1", Role: "owner", WrappedKey: []byte{1}, GrantedBy: "BOB111"}, 0); err != nil {
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
	finished := false
	for {
		select {
		case err := <-deleted:
			if err != nil {
				t.Fatal(err)
			}
			finished = true
		default:
		}
		var waiting bool
		if err := database.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE datname = current_database() AND $1 = ANY(pg_blocking_pids(pid)))`, int32(laterHold.Conn().PgConn().PID())).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if finished || waiting {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(5 * time.Millisecond):
		}
	}
	if err := laterHold.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if !finished {
		if err := <-deleted; err != nil {
			t.Fatal(err)
		}
	}
	if countQueryRow(t, database, `SELECT count(*) FROM vaults WHERE id = $1`, laterVault) != 0 {
		t.Error("a vault granted during deletion was left without an owner")
	}
	if countQueryRow(t, database, `SELECT count(*) FROM accounts WHERE id = 'ALICE1'`) != 0 {
		t.Error("account deletion did not complete")
	}
}
