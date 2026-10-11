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

	allResult, err := database.Migrate(ctx)
	if err != nil || len(allResult) != 0 {
		t.Fatalf("second migrate applied %d: %v", len(allResult), err)
	}
	allStatus, err := database.GetAllMigrationStatus(ctx)
	if err != nil || len(allStatus) == 0 {
		t.Fatalf("status: %v", err)
	}
	for _, status := range allStatus {
		if status.AppliedAt.IsZero() {
			t.Errorf("%s not applied", status.Source.Path)
		}
	}

	allRecoveryTable := []string{"recovery_keyset", "vault_recovery", "recovery_tokens", "account_recovery"}

	rolledBack, err := database.RollbackMigration(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, tableName := range allRecoveryTable {
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
	for _, tableName := range allRecoveryTable {
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

	if allResult, err = database.Migrate(ctx); err != nil || len(allResult) != 4 {
		t.Fatalf("re-apply: %d %v", len(allResult), err)
	}
	for _, tableName := range allRecoveryTable {
		if countTable(t, database, tableName) != 0 {
			t.Errorf("%s exists after re-applying", tableName)
		}
	}
	if countColumn(t, database, "vaults", "recovery_opt_out") != 0 || countTable(t, database, "share_invitations") != 1 {
		t.Error("re-applying left the wrong schema")
	}
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
	if _, err := database.RollbackMigration(ctx); err != nil {
		t.Fatal(err)
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

	if allResult, err := database.Migrate(ctx); err != nil || len(allResult) != 1 {
		t.Fatalf("applied %d: %v", len(allResult), err)
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
	allVault, err := database.ListAllVault(ctx, "AAAAAA")
	if err != nil || len(allVault) != 1 || allVault[0].ID != vaultID || allVault[0].Role != "owner" || len(allVault[0].WrappedKey) != 1 {
		t.Fatalf("vault after the migration: %+v %v", allVault, err)
	}

	if _, err := database.RollbackMigration(ctx); err != nil {
		t.Fatal(err)
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

	allItem, seq, err := database.ListAllChange(ctx, vaultID, 0, 10)
	if err != nil || seq != 2 || len(allItem) != 1 || allItem[0].Revision != revision2 {
		t.Errorf("changes: seq=%d items=%+v %v", seq, allItem, err)
	}
	if allItem, _, _ := database.ListAllChange(ctx, vaultID, 2, 10); len(allItem) != 0 {
		t.Errorf("changes since current: %d items", len(allItem))
	}
	if _, _, err := database.ListAllChange(ctx, uuid.NewString(), 0, 10); !errors.Is(err, ErrNotFound) {
		t.Errorf("changes on missing vault: %v", err)
	}
	if allHistory, _ := database.GetItemHistory(ctx, vaultID, itemID); len(allHistory) != 2 {
		t.Errorf("history: %d entries", len(allHistory))
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
		allItem, _, err := database.ListAllChange(ctx, vaultID, since, 2)
		if err != nil {
			t.Fatal(err)
		}
		total += len(allItem)
		if len(allItem) < 2 {
			break
		}
		since = allItem[len(allItem)-1].Seq
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
	allHistory, _ := database.GetItemHistory(ctx, vaultID, itemID)
	if len(allHistory) != 1 || allHistory[0].Revision != revision2 {
		t.Errorf("remaining history: %+v", allHistory)
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
	allVault, _ := database.ListAllVault(ctx, "BBBBBB")
	if len(allVault) != 1 || allVault[0].WrappedKey[0] != 2 {
		t.Errorf("bob vaults: %+v", allVault)
	}
	if allMember, _ := database.ListAllMember(ctx, vaultID); len(allMember) != 2 {
		t.Errorf("members: %d", len(allMember))
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
