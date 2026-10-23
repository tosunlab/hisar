package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
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
	invitationID, err := createTestShareInvitation(t, database, ctx, &ShareInvitation{VaultID: vaultID, Email: "alice@example.com",
		Role: "editor", InvitedBy: "BOB111"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := database.AcceptShareInvitation(ctx, invitationID, "ALICE1", "alice@example.com"); err != nil {
		t.Fatal(err)
	}
	completion := authorizeTestShareCompletion(t, database, invitationID)
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
		_, err := database.DeleteAccount(ctx, "ALICE1", session.TokenHash, testRecordHash(), reviewStoreDeletion(t, database, ctx, "ALICE1"))
		deleted <- err
	}()
	waitForStoreLock(t, database, ctx, "%organization_invitations WHERE invited_by%")
	granted := make(chan error, 1)
	go func() {
		err := database.CompleteSignedShare(ctx, "BOB111", "https://hisar.test", completion)
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
	deletionErr := <-deleted
	if !grantFinished {
		grantErr = <-granted
	}
	switch {
	case grantErr == nil:
		if !errors.Is(deletionErr, ErrDeletionPlanChanged) {
			t.Errorf("a grant that committed after the review must make the deletion ask for a new review, got %v", deletionErr)
		}
	case errors.Is(grantErr, ErrNoInvitation), errors.Is(grantErr, ErrNotFound):
		if deletionErr != nil {
			t.Errorf("deletion: %v", deletionErr)
		}
	default:
		t.Errorf("key grant: %v", grantErr)
	}
}

func TestAdminDeletionAsksForReviewWhileTargetKeepsCreatingVaults(t *testing.T) {
	testAdminDeletionDuringCreations(t, func(ctx context.Context, database *Store) error {
		return database.CreateVault(ctx, &Vault{ID: uuid.NewString(), Type: "shared", EncName: []byte{1}, CreatedBy: "ALICE1"}, []byte{1})
	})
}

func TestAdminDeletionAsksForReviewWhileTargetKeepsCreatingOrganizations(t *testing.T) {
	testAdminDeletionDuringCreations(t, func(ctx context.Context, database *Store) error {
		return database.CreateOrganization(ctx, uuid.NewString(), "Acme", "ALICE1")
	})
}

func TestAdminDeletionAsksForReviewOfAnOrganizationJoinedDuringDeletion(t *testing.T) {
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
		_, err := database.DeleteAccountAsAdmin(ctx, AdminActor{AccountID: "ADMIN1"}, "ALICE1", reviewStoreDeletion(t, database, ctx, "ALICE1"))
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
	if err := <-deleted; !errors.Is(err, ErrDeletionPlanChanged) {
		t.Fatalf("deletion reviewed before the organization was joined, while it is still locked: %v", err)
	}
	if err := organizationBarrier.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := database.DeleteAccountAsAdmin(ctx, AdminActor{AccountID: "ADMIN1"}, "ALICE1", reviewStoreDeletion(t, database, ctx, "ALICE1")); err != nil {
		t.Fatalf("deletion reviewed after joining the organization: %v", err)
	}
	if _, err := database.GetAccountByID(ctx, "ALICE1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted account: %v", err)
	}
	if countQueryRow(t, database, `SELECT count(*) FROM organization_members WHERE organization_id = $1 AND account_id = 'BOB111'`, organizationID) != 1 {
		t.Error("deletion removed the other account's organization membership")
	}
}

// The target keeps changing what its deletion removes. A plan reviewed before the latest change is refused without a
// deadlock or partial deletion, and a plan reviewed after the changes stop deletes everything.
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
	// Stop the loops between creations: a cancelled commit can still land after a later review.
	var stopping atomic.Bool
	var workers sync.WaitGroup
	var created atomic.Int64
	ready := make(chan error, 8)
	finished := make(chan error, 8)
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			err := create(ctx, database)
			ready <- err
			for err == nil && !stopping.Load() {
				created.Add(1)
				err = create(ctx, database)
			}
			finished <- err
		}()
	}
	stop := func() {
		stopping.Store(true)
		workers.Wait()
	}
	defer stop()
	for range 8 {
		if err := <-ready; err != nil {
			t.Fatal(err)
		}
	}
	planVersion := reviewStoreDeletion(t, database, ctx, "ALICE1")
	for reviewed := created.Load(); created.Load() <= reviewed+8; {
		if ctx.Err() != nil {
			t.Fatal(ctx.Err())
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := database.DeleteAccountAsAdmin(ctx, AdminActor{AccountID: "ADMIN1"}, "ALICE1", planVersion); !errors.Is(err, ErrDeletionPlanChanged) {
		t.Fatalf("admin deletion of a plan the eight creation loops changed: %v", err)
	}
	if countQueryRow(t, database, `SELECT count(*) FROM accounts WHERE id = 'ALICE1'`) != 1 {
		t.Fatal("a refused deletion removed the account")
	}
	stop()
	for range 8 {
		if err := <-finished; err != nil {
			t.Errorf("creation during deletion: %v", err)
		}
	}
	if _, err := database.DeleteAccountAsAdmin(ctx, AdminActor{AccountID: "ADMIN1"}, "ALICE1", reviewStoreDeletion(t, database, ctx, "ALICE1")); err != nil {
		t.Fatalf("admin deletion reviewed after the creations stopped: %v", err)
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
		_, err := database.DeleteAccount(ctx, "ALICE1", session.TokenHash, testRecordHash(), reviewStoreDeletion(t, database, ctx, "ALICE1"))
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

// An invitation to the account's email created after the deletion checked its plan is not in that plan. The deletion
// leaves it, as if it had been created just after, and an acceptance waiting for the deletion then finds no account.
func TestDeletionLeavesAnInvitationCreatedAfterItsPlanCheck(t *testing.T) {
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
		_, err := database.DeleteAccount(ctx, "ALICE1", session.TokenHash, testRecordHash(), reviewStoreDeletion(t, database, ctx, "ALICE1"))
		deleted <- err
	}()
	waitForStoreLock(t, database, ctx, "%UPDATE organization_members SET role = 'admin'%")
	invitationID, err = createTestShareInvitation(t, database, ctx, &ShareInvitation{VaultID: vaultID, Email: "alice@example.com",
		Role: "editor", InvitedBy: "BOB111"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	accepted := make(chan error, 1)
	go func() {
		_, _, err := database.AcceptShareInvitation(ctx, invitationID, "ALICE1", "alice@example.com")
		accepted <- err
	}()
	waitForStoreLock(t, database, ctx, "%SELECT id FROM accounts WHERE id = $1 FOR SHARE%")
	if err := barrier.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-deleted; err != nil {
		t.Errorf("deletion with an invitation created after its plan check: %v", err)
	}
	if err := <-accepted; !errors.Is(err, ErrNotFound) {
		t.Errorf("acceptance by the deleted account: %v", err)
	}
	if countQueryRow(t, database, `SELECT count(*) FROM share_invitations WHERE id = $1 AND state = 'pending'`, invitationID) != 1 {
		t.Error("deletion removed an invitation its reviewed plan did not include")
	}
	if countQueryRow(t, database, `SELECT count(*) FROM organization_members WHERE organization_id = $1 AND account_id = 'BOB111' AND role = 'admin'`, organizationID) != 1 {
		t.Error("the remaining member did not become the organization's admin")
	}
}
