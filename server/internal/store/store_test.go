package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Requires HISAR_TEST_DATABASE_URL pointing at a disposable database: the public schema is dropped.
func createTestStore(t *testing.T) *Store {
	t.Helper()
	databaseURL := os.Getenv("HISAR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("HISAR_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	connection.Close(ctx)
	database, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	if _, err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return database
}

func createTestAccount(t *testing.T, database *Store, accountID, email string) (*Device, *Session) {
	t.Helper()
	ctx := context.Background()
	inviteHash := []byte("invite-" + accountID)
	if err := database.CreateInvite(ctx, inviteHash, email, "member", nil, time.Hour); err != nil {
		t.Fatal(err)
	}
	account := &Account{ID: accountID, Email: email, Name: accountID, Argon2Params: []byte(`{}`), Salt: []byte{1}, OpaqueRecord: []byte{1}, EncKeyset: []byte{1}, KEMPub: []byte{1}, SignPub: []byte{1}, KEMSig: []byte{1}}
	device := &Device{ID: uuid.NewString(), AccountID: accountID, Name: "d", Platform: "p"}
	session := &Session{TokenHash: []byte("token-" + accountID), AccountID: accountID, DeviceID: device.ID, ExpiresAt: time.Now().Add(time.Hour)}
	if err := database.RedeemInvite(ctx, inviteHash, account, device, session); err != nil {
		t.Fatal(err)
	}
	return device, session
}

func createTestVault(t *testing.T, database *Store, owner string) string {
	t.Helper()
	vaultID := uuid.NewString()
	if err := database.CreateVault(context.Background(), &Vault{ID: vaultID, Type: "shared", EncName: []byte{1}, CreatedBy: owner}, []byte{1}); err != nil {
		t.Fatal(err)
	}
	return vaultID
}

func TestMigrations(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()

	results, err := database.Migrate(ctx)
	if err != nil || len(results) != 0 {
		t.Fatalf("second migrate applied %d: %v", len(results), err)
	}
	statuses, err := database.GetMigrationStatuses(ctx)
	if err != nil || len(statuses) == 0 {
		t.Fatalf("status: %v", err)
	}
	for _, status := range statuses {
		if status.AppliedAt.IsZero() {
			t.Errorf("%s not applied", status.Source.Path)
		}
	}

	recoveryTables := []string{"recovery_keyset", "vault_recovery", "recovery_tokens", "account_recovery"}

	if countConstraint(t, database, "items_updated_by_fkey") != 0 || countConstraint(t, database, "vaults_created_by_fkey") != 0 {
		t.Error("account deletion migration left the account foreign keys")
	}
	rolledBack, err := database.RollbackMigration(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if countConstraint(t, database, "items_updated_by_fkey") != 1 || countConstraint(t, database, "vault_access_granted_by_fkey") != 1 {
		t.Errorf("rolling back %s left the wrong schema", rolledBack.Source.Path)
	}

	if rolledBack, err = database.RollbackMigration(ctx); err != nil {
		t.Fatal(err)
	}
	if countTable(t, database, "share_links") != 0 || countTable(t, database, "share_invitations") != 1 {
		t.Errorf("rolling back %s left the wrong schema", rolledBack.Source.Path)
	}

	if rolledBack, err = database.RollbackMigration(ctx); err != nil {
		t.Fatal(err)
	}
	for _, tableName := range recoveryTables {
		if countTable(t, database, tableName) != 1 {
			t.Errorf("%s missing after rolling back %s", tableName, rolledBack.Source.Path)
		}
	}
	if countColumn(t, database, "vaults", "recovery_opt_out") != 1 || countTable(t, database, "share_invitations") != 1 {
		t.Errorf("rolling back %s left the wrong schema", rolledBack.Source.Path)
	}

	if rolledBack, err = database.RollbackMigration(ctx); err != nil {
		t.Fatal(err)
	}
	if countTable(t, database, "share_invitations") != 0 || countTable(t, database, "recovery_keyset") != 1 {
		t.Errorf("rolling back %s left the wrong schema", rolledBack.Source.Path)
	}

	if rolledBack, err = database.RollbackMigration(ctx); err != nil {
		t.Fatal(err)
	}
	for _, tableName := range recoveryTables {
		if countTable(t, database, tableName) != 0 {
			t.Errorf("%s still exists after rolling back %s", tableName, rolledBack.Source.Path)
		}
	}
	if countColumn(t, database, "vaults", "recovery_opt_out") != 0 || countTable(t, database, "accounts") != 1 {
		t.Errorf("rolling back %s left the wrong schema", rolledBack.Source.Path)
	}

	if rolledBack, err = database.RollbackMigration(ctx); err != nil {
		t.Fatal(err)
	}
	if countTable(t, database, "accounts") != 0 {
		t.Errorf("accounts table still exists after rolling back %s", rolledBack.Source.Path)
	}

	if results, err = database.Migrate(ctx); err != nil || len(results) != 6 {
		t.Fatalf("re-apply: %d %v", len(results), err)
	}
	for _, tableName := range recoveryTables {
		if countTable(t, database, tableName) != 0 {
			t.Errorf("%s exists after re-applying", tableName)
		}
	}
	if countColumn(t, database, "vaults", "recovery_opt_out") != 0 || countTable(t, database, "share_invitations") != 1 || countTable(t, database, "share_links") != 1 {
		t.Error("re-applying left the wrong schema")
	}
}

func countConstraint(t *testing.T, database *Store, constraintName string) int {
	t.Helper()
	var constraintCount int
	if err := database.pool.QueryRow(context.Background(), `SELECT count(*) FROM information_schema.table_constraints WHERE constraint_name = $1`, constraintName).Scan(&constraintCount); err != nil {
		t.Fatal(err)
	}
	return constraintCount
}

func countTable(t *testing.T, database *Store, tableName string) int {
	t.Helper()
	var tableCount int
	if err := database.pool.QueryRow(context.Background(), `SELECT count(*) FROM information_schema.tables WHERE table_name = $1`, tableName).Scan(&tableCount); err != nil {
		t.Fatal(err)
	}
	return tableCount
}

func countColumn(t *testing.T, database *Store, tableName, columnName string) int {
	t.Helper()
	var columnCount int
	if err := database.pool.QueryRow(context.Background(), `SELECT count(*) FROM information_schema.columns WHERE table_name = $1 AND column_name = $2`, tableName, columnName).Scan(&columnCount); err != nil {
		t.Fatal(err)
	}
	return columnCount
}

func countRow(t *testing.T, database *Store, tableName string) int {
	t.Helper()
	var rowCount int
	if err := database.pool.QueryRow(context.Background(), `SELECT count(*) FROM `+tableName).Scan(&rowCount); err != nil {
		t.Fatal(err)
	}
	return rowCount
}

func TestRemoveRecoveryMigrationDeletesRecoveryData(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	createTestAccount(t, database, "BBBBBB", "b@example.com")
	vaultID := createTestVault(t, database, "AAAAAA")
	for range 3 {
		if _, err := database.RollbackMigration(ctx); err != nil {
			t.Fatal(err)
		}
	}

	for _, statement := range []string{
		`INSERT INTO recovery_keyset (kem_pub, sign_pub, kem_sig, enc_keyset, created_by) VALUES ('\x01', '\x02', '\x03', '\x04', 'AAAAAA')`,
		`INSERT INTO vault_recovery (vault_id, wrapped_key) VALUES ('` + vaultID + `', '\x05')`,
		`UPDATE vaults SET recovery_opt_out = true`,
		`INSERT INTO recovery_tokens (account_id, token_hash, created_by, expires_at) VALUES ('BBBBBB', '\x06', 'AAAAAA', now() + interval '1 day')`,
		`INSERT INTO account_recovery (account_id, argon2_params, salt, opaque_record, enc_keyset, kem_pub, sign_pub, kem_sig)
			VALUES ('BBBBBB', '{}', '\x07', '\x08', '\x09', '\x0a', '\x0b', '\x0c')`,
	} {
		if _, err := database.pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}

	if results, err := database.Migrate(ctx); err != nil || len(results) != 3 {
		t.Fatalf("applied %d: %v", len(results), err)
	}
	for _, tableName := range []string{"recovery_keyset", "vault_recovery", "recovery_tokens", "account_recovery"} {
		if countTable(t, database, tableName) != 0 {
			t.Errorf("%s still exists", tableName)
		}
	}
	if countColumn(t, database, "vaults", "recovery_opt_out") != 0 {
		t.Error("vaults.recovery_opt_out still exists")
	}
	if countRow(t, database, "accounts") != 2 || countRow(t, database, "sessions") != 2 {
		t.Error("the migration changed accounts or sessions")
	}
	vaults, err := database.ListVaults(ctx, "AAAAAA")
	if err != nil || len(vaults) != 1 || vaults[0].ID != vaultID || vaults[0].Role != "owner" || len(vaults[0].WrappedKey) != 1 {
		t.Fatalf("vault after the migration: %+v %v", vaults, err)
	}

	for range 3 {
		if _, err := database.RollbackMigration(ctx); err != nil {
			t.Fatal(err)
		}
	}
	for _, tableName := range []string{"recovery_keyset", "vault_recovery", "recovery_tokens", "account_recovery"} {
		if countRow(t, database, tableName) != 0 {
			t.Errorf("%s has rows after rolling back", tableName)
		}
	}
	var isOptedOut bool
	if err := database.pool.QueryRow(ctx, `SELECT recovery_opt_out FROM vaults WHERE id = $1`, vaultID).Scan(&isOptedOut); err != nil || isOptedOut {
		t.Errorf("recovery_opt_out after rolling back: %v %v", isOptedOut, err)
	}
}

func TestInvites(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")

	account := &Account{ID: "BBBBBB", Email: "b@example.com", Argon2Params: []byte(`{}`)}
	device := &Device{ID: uuid.NewString(), AccountID: "BBBBBB", Name: "d", Platform: "p"}
	session := &Session{TokenHash: []byte("t"), AccountID: "BBBBBB", DeviceID: device.ID, ExpiresAt: time.Now().Add(time.Hour)}

	if err := database.RedeemInvite(ctx, []byte("invite-AAAAAA"), account, device, session); !errors.Is(err, ErrNotFound) {
		t.Errorf("used invite: %v", err)
	}
	database.CreateInvite(ctx, []byte("wrong-email"), "other@example.com", "member", nil, time.Hour)
	if err := database.RedeemInvite(ctx, []byte("wrong-email"), account, device, session); !errors.Is(err, ErrNotFound) {
		t.Errorf("wrong email: %v", err)
	}
	database.CreateInvite(ctx, []byte("expired"), "b@example.com", "member", nil, -time.Minute)
	if err := database.RedeemInvite(ctx, []byte("expired"), account, device, session); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired invite: %v", err)
	}
	if _, err := database.GetInviteByHash(ctx, []byte("expired")); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired InviteByHash: %v", err)
	}
	database.CreateInvite(ctx, []byte("dup"), "a@example.com", "member", nil, time.Hour)
	duplicateAccount := &Account{ID: "CCCCCC", Email: "a@example.com", Name: "c", Argon2Params: []byte(`{}`), Salt: []byte{1}, OpaqueRecord: []byte{1}, EncKeyset: []byte{1}, KEMPub: []byte{1}, SignPub: []byte{1}, KEMSig: []byte{1}}
	if err := database.RedeemInvite(ctx, []byte("dup"), duplicateAccount, device, session); !errors.Is(err, ErrExists) {
		t.Errorf("duplicate email: %v", err)
	}
	if _, err := database.GetInviteByHash(ctx, []byte("dup")); err != nil {
		t.Errorf("failed redeem consumed invite: %v", err)
	}
}

func TestGetInviteWithInviter(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	inviterID := "AAAAAA"
	database.CreateInvite(ctx, []byte("from-user"), "b@example.com", "member", &inviterID, time.Hour)
	database.CreateInvite(ctx, []byte("from-operator"), "c@example.com", "admin", nil, time.Hour)
	database.CreateInvite(ctx, []byte("expired"), "d@example.com", "member", &inviterID, -time.Minute)

	invite, err := database.GetInviteWithInviter(ctx, []byte("from-user"))
	if err != nil || invite.Email != "b@example.com" || invite.InviterName != "AAAAAA" || invite.InviterEmail != "a@example.com" || time.Until(invite.ExpiresAt) < 59*time.Minute {
		t.Errorf("invite from a user: %+v %v", invite, err)
	}
	invite, err = database.GetInviteWithInviter(ctx, []byte("from-operator"))
	if err != nil || invite.Email != "c@example.com" || invite.InviterName != "" || invite.InviterEmail != "" {
		t.Errorf("invite from the operator: %+v %v", invite, err)
	}
	for _, tokenHash := range []string{"expired", "invite-AAAAAA", "unknown"} {
		if invite, err := database.GetInviteWithInviter(ctx, []byte(tokenHash)); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: %+v %v", tokenHash, invite, err)
		}
	}
}

func TestSessions(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	device, session := createTestAccount(t, database, "AAAAAA", "a@example.com")

	foundSession, err := database.GetSessionByHash(ctx, session.TokenHash, 24*time.Hour)
	if err != nil || foundSession.AccountID != "AAAAAA" || foundSession.Role != "member" {
		t.Fatalf("session: %+v %v", foundSession, err)
	}
	if time.Until(foundSession.ExpiresAt) < 23*time.Hour {
		t.Errorf("expiry not extended: %v", foundSession.ExpiresAt)
	}

	if err := database.RevokeDevice(ctx, "BBBBBB", device.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("revoke other's device: %v", err)
	}
	if err := database.RevokeDevice(ctx, "AAAAAA", device.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.GetSessionByHash(ctx, session.TokenHash, time.Hour); !errors.Is(err, ErrNotFound) {
		t.Errorf("revoked session: %v", err)
	}

	secondDevice := &Device{ID: uuid.NewString(), AccountID: "AAAAAA", Name: "d2", Platform: "p"}
	expired := &Session{TokenHash: []byte("old"), AccountID: "AAAAAA", DeviceID: secondDevice.ID, ExpiresAt: time.Now().Add(-time.Minute)}
	if err := database.CreateDeviceSession(ctx, secondDevice, expired); err != nil {
		t.Fatal(err)
	}
	if _, err := database.GetSessionByHash(ctx, []byte("old"), time.Hour); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired session: %v", err)
	}
}

func TestPutItem(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	vaultID := createTestVault(t, database, "AAAAAA")
	itemID, revision1, revision2, revision3 := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()

	createItem := func(revision string) *Item {
		return &Item{ID: itemID, Revision: revision, Overview: []byte("o-" + revision), Details: []byte("d"), UpdatedBy: "AAAAAA"}
	}

	if _, err := database.PutItem(ctx, vaultID, revision1, createItem(revision2)); !errors.Is(err, ErrConflict) {
		t.Errorf("base for missing item: %v", err)
	}
	first := createItem(revision1)
	if _, err := database.PutItem(ctx, vaultID, "", first); err != nil || first.Seq != 1 {
		t.Fatalf("create: seq=%d %v", first.Seq, err)
	}
	retry := createItem(revision1)
	if _, err := database.PutItem(ctx, vaultID, "", retry); err != nil || retry.Seq != 1 {
		t.Errorf("retry: seq=%d %v", retry.Seq, err)
	}
	currentItem, err := database.PutItem(ctx, vaultID, "", createItem(revision2))
	if !errors.Is(err, ErrConflict) || currentItem == nil || currentItem.Revision != revision1 {
		t.Errorf("create over existing: %v %+v", err, currentItem)
	}
	second := createItem(revision2)
	if _, err := database.PutItem(ctx, vaultID, revision1, second); err != nil || second.Seq != 2 {
		t.Fatalf("update: seq=%d %v", second.Seq, err)
	}
	currentItem, err = database.PutItem(ctx, vaultID, revision1, createItem(revision3))
	if !errors.Is(err, ErrConflict) || currentItem.Revision != revision2 || string(currentItem.Overview) != "o-"+revision2 {
		t.Errorf("stale base: %v %+v", err, currentItem)
	}

	items, seq, err := database.ListChanges(ctx, vaultID, 0, 10)
	if err != nil || seq != 2 || len(items) != 1 || items[0].Revision != revision2 {
		t.Errorf("changes: seq=%d items=%+v %v", seq, items, err)
	}
	if items, _, _ := database.ListChanges(ctx, vaultID, 2, 10); len(items) != 0 {
		t.Errorf("changes since current: %d items", len(items))
	}
	if _, _, err := database.ListChanges(ctx, uuid.NewString(), 0, 10); !errors.Is(err, ErrNotFound) {
		t.Errorf("changes on missing vault: %v", err)
	}
	if histories, _ := database.GetItemHistory(ctx, vaultID, itemID); len(histories) != 2 {
		t.Errorf("history: %d entries", len(histories))
	}
}

func TestChangesPaging(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	vaultID := createTestVault(t, database, "AAAAAA")
	for range 5 {
		item := &Item{ID: uuid.NewString(), Revision: uuid.NewString(), Overview: []byte{1}, Details: []byte{1}, UpdatedBy: "AAAAAA"}
		if _, err := database.PutItem(ctx, vaultID, "", item); err != nil {
			t.Fatal(err)
		}
	}
	var since int64
	var total int
	for {
		items, _, err := database.ListChanges(ctx, vaultID, since, 2)
		if err != nil {
			t.Fatal(err)
		}
		total += len(items)
		if len(items) < 2 {
			break
		}
		since = items[len(items)-1].Seq
	}
	if total != 5 {
		t.Errorf("paged %d items, want 5", total)
	}
}

func TestPruneHistory(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	vaultID := createTestVault(t, database, "AAAAAA")
	itemID, revision1, revision2 := uuid.NewString(), uuid.NewString(), uuid.NewString()
	database.PutItem(ctx, vaultID, "", &Item{ID: itemID, Revision: revision1, Overview: []byte{1}, Details: []byte{1}, UpdatedBy: "AAAAAA"})
	database.PutItem(ctx, vaultID, revision1, &Item{ID: itemID, Revision: revision2, Overview: []byte{2}, Details: []byte{2}, UpdatedBy: "AAAAAA"})
	if _, err := database.pool.Exec(ctx, `UPDATE item_history SET updated_at = now() - interval '100 days'`); err != nil {
		t.Fatal(err)
	}
	prunedCount, err := database.PruneHistory(ctx, 90*24*time.Hour)
	if err != nil || prunedCount != 1 {
		t.Fatalf("pruned %d, %v", prunedCount, err)
	}
	histories, _ := database.GetItemHistory(ctx, vaultID, itemID)
	if len(histories) != 1 || histories[0].Revision != revision2 {
		t.Errorf("remaining history: %+v", histories)
	}
}

func TestPruneAudit(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	accountID := "AAAAAA"
	database.RecordAudit(ctx, &accountID, nil, "login", nil, "192.0.2.1")
	database.RecordAudit(ctx, &accountID, nil, "login", nil, "192.0.2.2")
	if _, err := database.pool.Exec(ctx, `UPDATE audit_events SET created_at = now() - interval '400 days' WHERE ip = '192.0.2.1'`); err != nil {
		t.Fatal(err)
	}
	prunedCount, err := database.PruneAudit(ctx, 365*24*time.Hour)
	if err != nil || prunedCount != 1 {
		t.Fatalf("pruned %d, %v", prunedCount, err)
	}
	var remainingIPs []string
	rows, _ := database.pool.Query(ctx, `SELECT ip FROM audit_events`)
	for rows.Next() {
		var ip string
		rows.Scan(&ip)
		remainingIPs = append(remainingIPs, ip)
	}
	if len(remainingIPs) != 1 || remainingIPs[0] != "192.0.2.2" {
		t.Errorf("remaining audit events: %v", remainingIPs)
	}
}

func TestPruneExpiredInvitation(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	createTestAccount(t, database, "BBBBBB", "b@example.com")
	vaultID := createTestVault(t, database, "AAAAAA")
	createdBy := "AAAAAA"
	for _, invite := range []struct {
		hash string
		ttl  time.Duration
	}{{"old", -40 * 24 * time.Hour}, {"recently-expired", -time.Hour}, {"live", time.Hour}} {
		if err := database.CreateInvite(ctx, []byte(invite.hash), invite.hash+"@example.com", "member", &createdBy, invite.ttl); err != nil {
			t.Fatal(err)
		}
	}
	database.UpsertShareInvitation(ctx, &ShareInvitation{VaultID: vaultID, Email: "old@example.com", Role: "reader", InvitedBy: "AAAAAA"}, -40*24*time.Hour)
	database.UpsertShareInvitation(ctx, &ShareInvitation{VaultID: vaultID, Email: "live@example.com", Role: "reader", InvitedBy: "AAAAAA"}, time.Hour)
	acceptedID, _ := database.UpsertShareInvitation(ctx, &ShareInvitation{VaultID: vaultID, Email: "b@example.com", Role: "reader", InvitedBy: "AAAAAA"}, time.Hour)
	if _, _, err := database.AcceptShareInvitation(ctx, acceptedID, "BBBBBB", "b@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.pool.Exec(ctx, `UPDATE share_invitations SET expires_at = now() - interval '40 days' WHERE id = $1`, acceptedID); err != nil {
		t.Fatal(err)
	}

	prunedCount, err := database.PruneExpiredInvitation(ctx, 30*24*time.Hour)
	if err != nil || prunedCount != 2 {
		t.Fatalf("pruned %d, %v", prunedCount, err)
	}
	var inviteCount, shareInvitationCount int
	database.pool.QueryRow(ctx, `SELECT count(*) FROM invites WHERE email IN ('old@example.com', 'recently-expired@example.com', 'live@example.com')`).Scan(&inviteCount)
	database.pool.QueryRow(ctx, `SELECT count(*) FROM share_invitations`).Scan(&shareInvitationCount)
	if inviteCount != 2 || shareInvitationCount != 2 {
		t.Errorf("left %d invites and %d share invitations", inviteCount, shareInvitationCount)
	}
	if waiting, _ := database.ListWaitingShareInvitations(ctx, "BBBBBB"); len(waiting) != 1 {
		t.Error("an accepted invitation was pruned")
	}
}

func TestVaultAccess(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	createTestAccount(t, database, "BBBBBB", "b@example.com")
	vaultID := createTestVault(t, database, "AAAAAA")

	if _, err := database.GetVaultRole(ctx, vaultID, "BBBBBB"); !errors.Is(err, ErrNotFound) {
		t.Errorf("non-member role: %v", err)
	}
	if err := database.SetAccess(ctx, vaultID, Member{AccountID: "BBBBBB", Role: "reader", WrappedKey: []byte{1}, GrantedBy: "AAAAAA"}); err != nil {
		t.Fatal(err)
	}
	if err := database.SetAccess(ctx, vaultID, Member{AccountID: "BBBBBB", Role: "editor", WrappedKey: []byte{2}, GrantedBy: "AAAAAA"}); err != nil {
		t.Fatal(err)
	}
	if role, _ := database.GetVaultRole(ctx, vaultID, "BBBBBB"); role != "editor" {
		t.Errorf("role after update = %q", role)
	}
	vaults, _ := database.ListVaults(ctx, "BBBBBB")
	if len(vaults) != 1 || vaults[0].WrappedKey[0] != 2 {
		t.Errorf("bob vaults: %+v", vaults)
	}
	if members, _ := database.ListMembers(ctx, vaultID); len(members) != 2 {
		t.Errorf("members: %d", len(members))
	}
	if err := database.RemoveAccess(ctx, vaultID, "BBBBBB"); err != nil {
		t.Fatal(err)
	}
	if err := database.RemoveAccess(ctx, vaultID, "BBBBBB"); !errors.Is(err, ErrNotFound) {
		t.Errorf("double remove: %v", err)
	}
	if err := database.CreateVault(ctx, &Vault{ID: vaultID, Type: "shared", EncName: []byte{1}, CreatedBy: "AAAAAA"}, []byte{1}); !errors.Is(err, ErrExists) {
		t.Errorf("duplicate vault: %v", err)
	}
}
