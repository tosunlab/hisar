package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGrantDuringDeletionLocksInvitationsBeforeAccountUpgrade(t *testing.T) {
	database := createTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, session := createTestAccount(t, database, "ALICE1", "alice@example.com")
	createTestAccount(t, database, "BOB111", "bob@example.com")
	vaultID := createTestVault(t, database, "BOB111")
	organizationID := uuid.NewString()
	if err := database.CreateOrganization(ctx, organizationID, "Acme", "BOB111"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := database.UpsertOrganizationInvitation(ctx, &OrganizationInvitation{OrganizationID: organizationID,
		Email: "alice@example.com", Role: "member", InvitedBy: "BOB111"}, time.Hour); err != nil {
		t.Fatal(err)
	}
	invitationID, err := database.UpsertShareInvitation(ctx, &ShareInvitation{VaultID: vaultID, Email: "alice@example.com",
		Role: "editor", InvitedBy: "BOB111"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := database.AcceptShareInvitation(ctx, invitationID, "ALICE1", "alice@example.com"); err != nil {
		t.Fatal(err)
	}
	barrier, err := database.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer barrier.Rollback(context.Background())
	if _, err := barrier.Exec(ctx, `SELECT 1 FROM organization_invitations WHERE email = 'alice@example.com' FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	deleted := make(chan error, 1)
	go func() {
		_, err := database.DeleteAccount(ctx, "ALICE1", session.TokenHash, testRecordHash())
		deleted <- err
	}()
	waitForStoreLock(t, database, ctx, "%organization_invitations WHERE invited_by%")
	granted := make(chan error, 1)
	go func() {
		_, err := database.GrantAccess(ctx, vaultID, Member{AccountID: "ALICE1", Role: "editor", WrappedKey: []byte{1}, GrantedBy: "BOB111"}, 0)
		granted <- err
	}()
	// Either the grant finishes under the weak account lock, or it reaches the foreign-key wait of the old order.
	var grantErr error
	grantFinished := false
	for !grantFinished {
		select {
		case grantErr = <-granted:
			grantFinished = true
		default:
		}
		var waiting bool
		if err := database.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock' AND query LIKE '%INSERT INTO vault_access%')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting || grantFinished {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := barrier.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-deleted; err != nil {
		t.Errorf("deletion: %v", err)
	}
	if !grantFinished {
		grantErr = <-granted
	}
	if grantErr != nil && !errors.Is(grantErr, ErrNoInvitation) {
		t.Errorf("key grant: %v", grantErr)
	}
}

func TestAdminDeletionSucceedsWhenTargetKeepsCreatingVaults(t *testing.T) {
	testAdminDeletionDuringCreations(t, func(ctx context.Context, database *Store) error {
		return database.CreateVault(ctx, &Vault{ID: uuid.NewString(), Type: "shared", EncName: []byte{1}, CreatedBy: "ALICE1"}, []byte{1})
	})
}

func TestAdminDeletionSucceedsWhenTargetKeepsCreatingOrganizations(t *testing.T) {
	testAdminDeletionDuringCreations(t, func(ctx context.Context, database *Store) error {
		return database.CreateOrganization(ctx, uuid.NewString(), "Acme", "ALICE1")
	})
}

func TestAdminDeletionRetriesAnOrganizationJoinedDuringDeletion(t *testing.T) {
	database := createTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	createTestAccount(t, database, "ADMIN1", "admin@example.com")
	createTestAccount(t, database, "ALICE1", "alice@example.com")
	createTestAccount(t, database, "BOB111", "bob@example.com")
	if _, err := database.pool.Exec(ctx, `UPDATE accounts SET role = 'admin' WHERE id = 'ADMIN1'`); err != nil {
		t.Fatal(err)
	}
	organizationID := uuid.NewString()
	if err := database.CreateOrganization(ctx, organizationID, "Acme", "BOB111"); err != nil {
		t.Fatal(err)
	}
	invitationID, _, err := database.UpsertOrganizationInvitation(ctx, &OrganizationInvitation{
		OrganizationID: organizationID, Email: "alice@example.com", Role: "member", InvitedBy: "BOB111",
	}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	accountBarrier, err := database.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer accountBarrier.Rollback(context.Background())
	if _, err := accountBarrier.Exec(ctx, `SELECT 1 FROM accounts WHERE id = 'ALICE1' FOR SHARE`); err != nil {
		t.Fatal(err)
	}
	deleted := make(chan error, 1)
	go func() {
		_, err := database.DeleteAccountAsAdmin(ctx, AdminActor{AccountID: "ADMIN1"}, "ALICE1")
		deleted <- err
	}()
	waitForStoreLock(t, database, ctx, "%ORDER BY id FOR NO KEY UPDATE%")
	organizationBarrier, err := database.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer organizationBarrier.Rollback(context.Background())
	if _, err := organizationBarrier.Exec(ctx, `SELECT 1 FROM organizations WHERE id = $1 FOR NO KEY UPDATE`, organizationID); err != nil {
		t.Fatal(err)
	}
	// Commit membership on the transaction that already holds the account share lock, ahead of deletion's waiter.
	result, err := accountBarrier.Exec(ctx, `
		WITH invitation AS (
			DELETE FROM organization_invitations WHERE id = $1 RETURNING organization_id, role, invited_by
		)
		INSERT INTO organization_members (organization_id, account_id, role, added_by)
		SELECT organization_id, 'ALICE1', role, invited_by FROM invitation`, invitationID)
	if err != nil || result.RowsAffected() != 1 {
		t.Fatalf("join organization during deletion: %v, result %v", err, result)
	}
	if err := accountBarrier.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	waitForStoreLock(t, database, ctx, "%FROM organizations o JOIN organization_members m%")
	if err := organizationBarrier.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-deleted; err != nil {
		t.Fatalf("deletion after a newly joined organization was locked: %v", err)
	}
	if _, err := database.GetAccountByID(ctx, "ALICE1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted account: %v", err)
	}
	if countQueryRow(t, database, `SELECT count(*) FROM organization_members WHERE organization_id = $1 AND account_id = 'BOB111'`, organizationID) != 1 {
		t.Error("deletion removed the other account's organization membership")
	}
}

func testAdminDeletionDuringCreations(t *testing.T, create func(context.Context, *Store) error) {
	t.Helper()
	database := createTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	createTestAccount(t, database, "ADMIN1", "admin@example.com")
	createTestAccount(t, database, "ALICE1", "alice@example.com")
	if _, err := database.pool.Exec(ctx, `UPDATE accounts SET role = 'admin' WHERE id = 'ADMIN1'`); err != nil {
		t.Fatal(err)
	}
	workersCtx, stop := context.WithCancel(ctx)
	var workers sync.WaitGroup
	ready := make(chan error, 8)
	finished := make(chan error, 8)
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			err := create(workersCtx, database)
			ready <- err
			for err == nil && workersCtx.Err() == nil {
				err = create(workersCtx, database)
			}
			finished <- err
		}()
	}
	defer func() {
		stop()
		workers.Wait()
	}()
	for range 8 {
		if err := <-ready; err != nil {
			t.Fatal(err)
		}
	}
	_, err := database.DeleteAccountAsAdmin(ctx, AdminActor{AccountID: "ADMIN1"}, "ALICE1")
	if err != nil {
		t.Fatalf("admin deletion while eight creation loops are running: %v", err)
	}
	stop()
	workers.Wait()
	for range 8 {
		if err := <-finished; err != nil && !errors.Is(err, ErrNotFound) && !errors.Is(err, context.Canceled) {
			t.Errorf("creation during deletion: %v", err)
		}
	}
	for _, table := range []string{"accounts", "vault_access", "organization_members"} {
		column := "account_id"
		if table == "accounts" {
			column = "id"
		}
		if countQueryRow(t, database, fmt.Sprintf("SELECT count(*) FROM %s WHERE %s = 'ALICE1'", table, column)) != 0 {
			t.Errorf("deleted account remains in %s", table)
		}
	}
}

func TestOrganizationAcceptanceWaitsForAccountDeletion(t *testing.T) {
	database := createTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, session := createTestAccount(t, database, "ALICE1", "alice@example.com")
	createTestAccount(t, database, "BOB111", "bob@example.com")
	vaultID := createTestVault(t, database, "ALICE1")
	organizationID := uuid.NewString()
	if err := database.CreateOrganization(ctx, organizationID, "Acme", "BOB111"); err != nil {
		t.Fatal(err)
	}
	invitationID, _, err := database.UpsertOrganizationInvitation(ctx, &OrganizationInvitation{
		OrganizationID: organizationID, Email: "alice@example.com", Role: "member", InvitedBy: "BOB111",
	}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
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
		_, err := database.DeleteAccount(ctx, "ALICE1", session.TokenHash, testRecordHash())
		deleted <- err
	}()
	waitForStoreLock(t, database, ctx, "%FROM vaults WHERE id IN (SELECT vault_id%")
	accepted := make(chan error, 1)
	go func() {
		_, _, err := database.AcceptOrganizationInvitation(ctx, invitationID, "ALICE1", "alice@example.com")
		accepted <- err
	}()
	for {
		var waiting bool
		if err := database.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock'
			AND (query LIKE '%SELECT id FROM accounts WHERE id = $1 FOR SHARE%'
				OR query LIKE '%INSERT INTO organization_members%'))`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case err := <-accepted:
			t.Fatalf("acceptance must wait for deletion, got %v", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(5 * time.Millisecond):
		}
	}
	if err := barrier.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-deleted; err != nil {
		t.Errorf("deletion: %v", err)
	}
	if err := <-accepted; !errors.Is(err, ErrNotFound) {
		t.Errorf("acceptance for deleted account: %v", err)
	}
	if countQueryRow(t, database, `SELECT count(*) FROM organization_members WHERE account_id = 'ALICE1'`) != 0 {
		t.Error("acceptance recreated the deleted account's membership")
	}
}

func TestDeletionRefusesInvitationInsertedAfterAccountUpgrade(t *testing.T) {
	database := createTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, session := createTestAccount(t, database, "ALICE1", "alice@example.com")
	createTestAccount(t, database, "BOB111", "bob@example.com")
	vaultID := createTestVault(t, database, "BOB111")
	organizationID := uuid.NewString()
	if err := database.CreateOrganization(ctx, organizationID, "Acme", "ALICE1"); err != nil {
		t.Fatal(err)
	}
	invitationID, _, err := database.UpsertOrganizationInvitation(ctx, &OrganizationInvitation{OrganizationID: organizationID,
		Email: "bob@example.com", Role: "member", InvitedBy: "ALICE1"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := database.AcceptOrganizationInvitation(ctx, invitationID, "BOB111", "bob@example.com"); err != nil {
		t.Fatal(err)
	}
	barrier, err := database.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer barrier.Rollback(context.Background())
	if _, err := barrier.Exec(ctx, `SELECT 1 FROM organization_members WHERE organization_id = $1 AND account_id = 'BOB111' FOR UPDATE`, organizationID); err != nil {
		t.Fatal(err)
	}
	deleted := make(chan error, 1)
	go func() {
		_, err := database.DeleteAccount(ctx, "ALICE1", session.TokenHash, testRecordHash())
		deleted <- err
	}()
	waitForStoreLock(t, database, ctx, "%UPDATE organization_members SET role = 'admin'%")
	invitationID, err = database.UpsertShareInvitation(ctx, &ShareInvitation{VaultID: vaultID, Email: "alice@example.com",
		Role: "editor", InvitedBy: "BOB111"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	accepted := make(chan error, 1)
	go func() {
		_, _, err := database.AcceptShareInvitation(ctx, invitationID, "ALICE1", "alice@example.com")
		accepted <- err
	}()
	waitForStoreLock(t, database, ctx, "%UPDATE share_invitations i SET state = 'accepted'%")
	if err := barrier.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-deleted; !errors.Is(err, ErrDeletionBusy) {
		t.Errorf("late invitation contention must refuse deletion without a deadlock, got %v", err)
	}
	if err := <-accepted; err != nil {
		t.Errorf("invitation acceptance after refused deletion: %v", err)
	}
	if countQueryRow(t, database, `SELECT count(*) FROM organization_members WHERE organization_id = $1 AND account_id = 'ALICE1' AND role = 'admin'`, organizationID) != 1 {
		t.Error("refused deletion changed organization membership")
	}
}
