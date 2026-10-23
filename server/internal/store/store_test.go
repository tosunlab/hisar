package store

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"slices"
	"strings"
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

// runWhileHolding opens a transaction and runs hold in it, then runs change in a goroutine. Once change has finished, or
// waits for a lock, it commits hold's transaction and returns change's error: a change that checks under the locks hold
// took sees hold's changes, and one that does not finishes before them.
func runWhileHolding(t *testing.T, database *Store, hold func(transaction pgx.Tx) error, change func() error) error {
	t.Helper()
	ctx := context.Background()
	transaction, err := database.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback(ctx)
	if err := hold(transaction); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- change() }()
	commit := func() {
		if err := transaction.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		select {
		case err := <-done:
			commit()
			return err
		default:
		}
		var waitingCount int
		if err := database.pool.QueryRow(ctx,
			`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&waitingCount); err != nil {
			t.Fatal(err)
		}
		if waitingCount > 0 {
			commit()
			return <-done
		}
	}
	t.Fatal("the change neither finished nor waited for a lock")
	return nil
}

func waitForStoreLock(t *testing.T, database *Store, ctx context.Context, queryPattern string) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		var waiting bool
		if err := database.pool.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM pg_stat_activity WHERE datname = current_database()
				AND wait_event_type = 'Lock' AND query LIKE $1)`, queryPattern).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
	}
	t.Fatalf("no lock wait for %q", queryPattern)
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
	if countTable(t, database, "account_preferences") != 1 {
		t.Error("account_preferences missing")
	}
	if countTable(t, database, "organizations") != 1 || countTable(t, database, "team_vaults") != 1 || countColumn(t, database, "vaults", "key_generation") != 1 {
		t.Error("the organization tables are missing")
	}
	if countTable(t, database, "instance_settings") != 1 {
		t.Error("instance_settings missing")
	}
	if countColumn(t, database, "accounts", "suspended_at") != 1 || countIndex(t, database, "devices_account_idx") != 1 {
		t.Error("the account suspension migration is missing")
	}
	for _, indexName := range []string{"audit_events_created_idx", "audit_events_account_idx", "audit_events_type_idx"} {
		if countIndex(t, database, indexName) != 1 {
			t.Errorf("%s missing", indexName)
		}
	}
	if countTable(t, database, "operational_secrets") != 1 {
		t.Fatal("operational secrets migration is missing")
	}
	rollbackTestMigrationsTo(t, database, 15)
	if countTable(t, database, "operational_secrets") != 0 {
		t.Fatal("rolling back unused operational secrets left its table")
	}
	if countTable(t, database, "vault_transition_items") != 1 {
		t.Fatal("staging migration is missing")
	}
	rollbackTestMigrationsTo(t, database, 13)
	if countTable(t, database, "vault_transition_items") != 0 || countColumn(t, database, "vaults", "metadata_revision") != 0 {
		t.Fatal("rolling back staging left transition state")
	}
	if countColumn(t, database, "vaults", "access_revision") != 1 {
		t.Fatal("rolling back staging removed the signed invitation migration's access revision")
	}
	if countColumn(t, database, "invites", "share_invitation_id") != 1 || countColumn(t, database, "invites", "organization_invitation_id") != 1 {
		t.Fatal("invitation source migration is missing")
	}
	sourceRollback, err := database.RollbackMigration(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if countColumn(t, database, "invites", "share_invitation_id") != 0 || countColumn(t, database, "invites", "organization_invitation_id") != 0 {
		t.Errorf("rolling back %s left invitation provenance", sourceRollback.Source.Path)
	}
	if countColumn(t, database, "share_invitations", "intent") != 1 || countTable(t, database, "retired_share_invitations") != 1 {
		t.Fatal("signed invitation migration is missing")
	}
	rollbackTestMigrationsTo(t, database, 11)
	if countColumn(t, database, "share_invitations", "intent") != 0 || countTable(t, database, "retired_share_invitations") != 0 {
		t.Fatal("signed invitation rollback kept its schema")
	}
	rolledBack, err := database.RollbackMigration(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, indexName := range []string{"audit_events_created_idx", "audit_events_account_idx", "audit_events_type_idx"} {
		if countIndex(t, database, indexName) != 0 {
			t.Errorf("rolling back %s left %s", rolledBack.Source.Path, indexName)
		}
	}
	if countColumn(t, database, "accounts", "suspended_at") != 1 {
		t.Errorf("rolling back %s left the wrong schema", rolledBack.Source.Path)
	}

	if rolledBack, err = database.RollbackMigration(ctx); err != nil {
		t.Fatal(err)
	}
	if countColumn(t, database, "accounts", "suspended_at") != 0 || countIndex(t, database, "devices_account_idx") != 0 || countTable(t, database, "instance_settings") != 1 {
		t.Errorf("rolling back %s left the wrong schema", rolledBack.Source.Path)
	}

	if rolledBack, err = database.RollbackMigration(ctx); err != nil {
		t.Fatal(err)
	}
	if countTable(t, database, "instance_settings") != 0 || countTable(t, database, "organizations") != 1 {
		t.Errorf("rolling back %s left the wrong schema", rolledBack.Source.Path)
	}

	if rolledBack, err = database.RollbackMigration(ctx); err != nil {
		t.Fatal(err)
	}
	if countTable(t, database, "organizations") != 0 || countTable(t, database, "team_vaults") != 0 || countColumn(t, database, "vaults", "organization_id") != 0 ||
		countColumn(t, database, "vaults", "needs_rotation") != 0 || countTable(t, database, "account_preferences") != 1 {
		t.Errorf("rolling back %s left the wrong schema", rolledBack.Source.Path)
	}

	if rolledBack, err = database.RollbackMigration(ctx); err != nil {
		t.Fatal(err)
	}
	if countTable(t, database, "account_preferences") != 0 {
		t.Errorf("rolling back %s left account_preferences", rolledBack.Source.Path)
	}

	if rolledBack, err = database.RollbackMigration(ctx); err != nil {
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

	if results, err = database.Migrate(ctx); err != nil || len(results) != len(statuses) {
		t.Fatalf("re-apply: %d %v", len(results), err)
	}
	for _, tableName := range recoveryTables {
		if countTable(t, database, tableName) != 0 {
			t.Errorf("%s exists after re-applying", tableName)
		}
	}
	if countColumn(t, database, "vaults", "recovery_opt_out") != 0 || countTable(t, database, "share_invitations") != 1 || countTable(t, database, "share_links") != 1 ||
		countTable(t, database, "account_preferences") != 1 || countTable(t, database, "organizations") != 1 || countTable(t, database, "instance_settings") != 1 ||
		countColumn(t, database, "accounts", "suspended_at") != 1 || countIndex(t, database, "audit_events_type_idx") != 1 {
		t.Error("re-applying left the wrong schema")
	}
}

func countIndex(t *testing.T, database *Store, indexName string) int {
	t.Helper()
	return countQueryRow(t, database, `SELECT count(*) FROM pg_indexes WHERE indexname = $1`, indexName)
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
	statuses, err := database.GetMigrationStatuses(ctx)
	if err != nil {
		t.Fatal(err)
	}
	vaultID := createTestVault(t, database, "AAAAAA")
	rollbackTestMigrationsTo(t, database, 3)

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

	if results, err := database.Migrate(ctx); err != nil || len(results) != len(statuses)-3 {
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

	rollbackTestMigrationsTo(t, database, 3)
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

	if _, err := database.RevokeDevice(ctx, "BBBBBB", device.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("revoke other's device: %v", err)
	}
	endedTokenHashes, err := database.RevokeDevice(ctx, "AAAAAA", device.ID)
	if err != nil || len(endedTokenHashes) != 1 || string(endedTokenHashes[0]) != string(session.TokenHash) {
		t.Fatalf("revoke: %q %v", endedTokenHashes, err)
	}
	if _, err := database.GetSessionByHash(ctx, session.TokenHash, time.Hour); !errors.Is(err, ErrNotFound) {
		t.Errorf("revoked session: %v", err)
	}

	secondDevice := &Device{ID: uuid.NewString(), AccountID: "AAAAAA", Name: "d2", Platform: "p"}
	expired := &Session{TokenHash: []byte("old"), AccountID: "AAAAAA", DeviceID: secondDevice.ID, ExpiresAt: time.Now().Add(-time.Minute)}
	if err := database.CreateDeviceSession(ctx, secondDevice, expired, testRecordHash()); err != nil {
		t.Fatal(err)
	}
	if _, err := database.GetSessionByHash(ctx, []byte("old"), time.Hour); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired session: %v", err)
	}
}

func TestSessionLookupRecordsLastSeenAtomicallyOrFails(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	device, session := createTestAccount(t, database, "AAAAAA", "a@example.com")
	readLastSeen := func() time.Time {
		t.Helper()
		var lastSeenAt time.Time
		if err := database.pool.QueryRow(ctx, `SELECT last_seen_at FROM devices WHERE id = $1`, device.ID).Scan(&lastSeenAt); err != nil {
			t.Fatal(err)
		}
		return lastSeenAt
	}
	if _, err := database.pool.Exec(ctx, `UPDATE devices SET last_seen_at = now() - interval '1 day' WHERE id = $1`, device.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.GetSessionByHash(ctx, session.TokenHash, time.Hour); err != nil {
		t.Fatal(err)
	}
	if time.Since(readLastSeen()) > time.Minute {
		t.Errorf("last seen not recorded: %v", readLastSeen())
	}

	var output bytes.Buffer
	previousWriter := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previousWriter) })
	beforeFailedWrite := readLastSeen()
	var beforeFailedExpiry time.Time
	if err := database.pool.QueryRow(ctx, `SELECT expires_at FROM sessions WHERE token_hash = $1`, session.TokenHash).Scan(&beforeFailedExpiry); err != nil {
		t.Fatal(err)
	}
	if _, err := database.pool.Exec(ctx, `
		CREATE FUNCTION refuse_last_seen() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'last seen refused'; END $$;
		CREATE TRIGGER refuse_last_seen BEFORE UPDATE OF last_seen_at ON devices FOR EACH ROW EXECUTE FUNCTION refuse_last_seen()`); err != nil {
		t.Fatal(err)
	}
	if authenticated, err := database.GetSessionByHash(ctx, session.TokenHash, 2*time.Hour); err == nil || authenticated != nil {
		t.Fatalf("last-seen failure returned an authenticated session: %+v %v", authenticated, err)
	}
	if !readLastSeen().Equal(beforeFailedWrite) {
		t.Error("the refused last-seen update changed the timestamp")
	}
	if !strings.Contains(output.String(), "device last-seen update failed") {
		t.Errorf("last-seen failure was not logged: %q", output.String())
	}
	var expiresAt time.Time
	if err := database.pool.QueryRow(ctx, `SELECT expires_at FROM sessions WHERE token_hash = $1`, session.TokenHash).Scan(&expiresAt); err != nil {
		t.Fatal(err)
	}
	if !expiresAt.Equal(beforeFailedExpiry) {
		t.Errorf("last-seen failure changed the session expiry: %v, was %v", expiresAt, beforeFailedExpiry)
	}
}

func TestLoginProvenAgainstAReplacedRecordCreatesNoSession(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	_, session := createTestAccount(t, database, "AAAAAA", "a@example.com")
	newDevice := func() (*Device, *Session) {
		device := &Device{ID: uuid.NewString(), AccountID: "AAAAAA", Name: "laptop", Platform: "p"}
		return device, &Session{TokenHash: []byte(device.ID), AccountID: "AAAAAA", DeviceID: device.ID, ExpiresAt: time.Now().Add(time.Hour)}
	}
	update := &CredentialUpdate{AccountID: "AAAAAA", TokenHash: session.TokenHash, ProvenRecordHash: HashOpaqueRecord([]byte{2}),
		Argon2Params: []byte(`{}`), Salt: []byte{3}, OpaqueRecord: []byte{3}, EncKeyset: []byte{3}}

	laptop, laptopSession := newDevice()
	err := runWhileHolding(t, database, func(transaction pgx.Tx) error {
		_, err := transaction.Exec(ctx, `UPDATE accounts SET opaque_record = $1 WHERE id = 'AAAAAA'`, []byte{2})
		return err
	}, func() error { return database.CreateDeviceSession(ctx, laptop, laptopSession, testRecordHash()) })
	if !errors.Is(err, ErrConflict) {
		t.Errorf("a login proven against the record a change replaced meanwhile: %v", err)
	}
	if _, err := database.GetSessionByHash(ctx, laptopSession.TokenHash, time.Hour); !errors.Is(err, ErrNotFound) {
		t.Errorf("the refused login's session: %v", err)
	}

	if _, err := database.UpdateAccountCredentials(ctx, update); err != nil {
		t.Fatal(err)
	}
	phone, phoneSession := newDevice()
	if err := database.CreateDeviceSession(ctx, phone, phoneSession, HashOpaqueRecord([]byte{2})); !errors.Is(err, ErrConflict) {
		t.Errorf("a login proven against the record before a credential change: %v", err)
	}
	if err := database.CreateDeviceSession(ctx, phone, phoneSession, HashOpaqueRecord([]byte{3})); err != nil {
		t.Errorf("a login proven against the current record: %v", err)
	}
}

// testRecordHash is the hash of the OPAQUE record createTestAccount gives its accounts.
func testRecordHash() []byte {
	return HashOpaqueRecord([]byte{1})
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

	if _, err := database.PutItem(ctx, vaultID, revision1, 0, createItem(revision2)); !errors.Is(err, ErrConflict) {
		t.Errorf("base for missing item: %v", err)
	}
	first := createItem(revision1)
	if _, err := database.PutItem(ctx, vaultID, "", 0, first); err != nil || first.Seq != 1 {
		t.Fatalf("create: seq=%d %v", first.Seq, err)
	}
	retry := createItem(revision1)
	if _, err := database.PutItem(ctx, vaultID, "", 0, retry); err != nil || retry.Seq != 1 {
		t.Errorf("retry: seq=%d %v", retry.Seq, err)
	}
	currentItem, err := database.PutItem(ctx, vaultID, "", 0, createItem(revision2))
	if !errors.Is(err, ErrConflict) || currentItem == nil || currentItem.Revision != revision1 {
		t.Errorf("create over existing: %v %+v", err, currentItem)
	}
	second := createItem(revision2)
	if _, err := database.PutItem(ctx, vaultID, revision1, 0, second); err != nil || second.Seq != 2 {
		t.Fatalf("update: seq=%d %v", second.Seq, err)
	}
	currentItem, err = database.PutItem(ctx, vaultID, revision1, 0, createItem(revision3))
	if !errors.Is(err, ErrConflict) || currentItem.Revision != revision2 || string(currentItem.Overview) != "o-"+revision2 {
		t.Errorf("stale base: %v %+v", err, currentItem)
	}

	items, seq, _, err := database.ListChanges(ctx, vaultID, 0, 10, 1<<20)
	if err != nil || seq != 2 || len(items) != 1 || items[0].Revision != revision2 {
		t.Errorf("changes: seq=%d items=%+v %v", seq, items, err)
	}
	if items, _, _, _ := database.ListChanges(ctx, vaultID, 2, 10, 1<<20); len(items) != 0 {
		t.Errorf("changes since current: %d items", len(items))
	}
	if _, _, _, err := database.ListChanges(ctx, uuid.NewString(), 0, 10, 1<<20); !errors.Is(err, ErrNotFound) {
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
		if _, err := database.PutItem(ctx, vaultID, "", 0, item); err != nil {
			t.Fatal(err)
		}
	}
	readPages := func(limit int, maxBytes int64) []int {
		t.Helper()
		var since int64
		var pageSizes []int
		for {
			items, _, isMore, err := database.ListChanges(ctx, vaultID, since, limit, maxBytes)
			if err != nil {
				t.Fatal(err)
			}
			pageSizes = append(pageSizes, len(items))
			if !isMore {
				return pageSizes
			}
			since = items[len(items)-1].Seq
		}
	}
	for _, test := range []struct {
		limit    int
		maxBytes int64
		want     []int
	}{
		{2, 1 << 20, []int{2, 2, 1}},
		{5, 1 << 20, []int{5, 0}},
		{10, 5, []int{3, 2}},
		{10, 1, []int{1, 1, 1, 1, 1}},
	} {
		if got := readPages(test.limit, test.maxBytes); !slices.Equal(got, test.want) {
			t.Errorf("limit %d, %d bytes: pages of %v, want %v", test.limit, test.maxBytes, got, test.want)
		}
	}
}

func TestPruneHistory(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	vaultID := createTestVault(t, database, "AAAAAA")
	itemID, revision1, revision2 := uuid.NewString(), uuid.NewString(), uuid.NewString()
	database.PutItem(ctx, vaultID, "", 0, &Item{ID: itemID, Revision: revision1, Overview: []byte{1}, Details: []byte{1}, UpdatedBy: "AAAAAA"})
	database.PutItem(ctx, vaultID, revision1, 0, &Item{ID: itemID, Revision: revision2, Overview: []byte{2}, Details: []byte{2}, UpdatedBy: "AAAAAA"})
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
	for _, ip := range []string{"192.0.2.1", "192.0.2.2"} {
		if err := database.RecordAudit(ctx, &accountID, nil, "login", nil, ip); err != nil {
			t.Fatal(err)
		}
	}
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
	createTestShareInvitation(t, database, ctx, &ShareInvitation{VaultID: vaultID, Email: "old@example.com", Role: "reader", InvitedBy: "AAAAAA"}, -40*24*time.Hour)
	createTestShareInvitation(t, database, ctx, &ShareInvitation{VaultID: vaultID, Email: "live@example.com", Role: "reader", InvitedBy: "AAAAAA"}, time.Hour)
	acceptedID, _ := createTestShareInvitation(t, database, ctx, &ShareInvitation{VaultID: vaultID, Email: "b@example.com", Role: "reader", InvitedBy: "AAAAAA"}, time.Hour)
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
	if err := database.RemoveAccess(ctx, vaultID, "BBBBBB", "AAAAAA"); !errors.Is(err, ErrNotOwner) {
		t.Errorf("an editor removes the owner: %v", err)
	}
	if err := database.RemoveAccess(ctx, vaultID, "AAAAAA", "BBBBBB"); err != nil {
		t.Fatal(err)
	}
	if err := database.RemoveAccess(ctx, vaultID, "AAAAAA", "BBBBBB"); !errors.Is(err, ErrNotFound) {
		t.Errorf("double remove: %v", err)
	}
	if err := database.CreateVault(ctx, &Vault{ID: vaultID, Type: "shared", EncName: []byte{1}, CreatedBy: "AAAAAA"}, []byte{1}); !errors.Is(err, ErrExists) {
		t.Errorf("duplicate vault: %v", err)
	}
}
