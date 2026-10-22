package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func countQueryRow(t *testing.T, database *Store, query string, arguments ...any) int {
	t.Helper()
	var rowCount int
	if err := database.pool.QueryRow(context.Background(), query, arguments...).Scan(&rowCount); err != nil {
		t.Fatal(err)
	}
	return rowCount
}

func TestUpdateAccountCredentials(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	_, aliceSession := createTestAccount(t, database, "ALICE1", "alice@example.com")
	laptop := &Device{ID: uuid.NewString(), AccountID: "ALICE1", Name: "laptop", Platform: "p"}
	laptopSession := &Session{TokenHash: []byte("laptop"), AccountID: "ALICE1", DeviceID: laptop.ID, ExpiresAt: time.Now().Add(time.Hour)}
	if err := database.CreateDeviceSession(ctx, laptop, laptopSession, testRecordHash()); err != nil {
		t.Fatal(err)
	}
	update := &CredentialUpdate{
		AccountID: "ALICE1", TokenHash: aliceSession.TokenHash, ProvenRecordHash: HashOpaqueRecord([]byte{9}),
		Argon2Params: []byte(`{"t":30}`), Salt: []byte{2}, OpaqueRecord: []byte{3}, EncKeyset: []byte{4},
	}

	if _, err := database.UpdateAccountCredentials(ctx, update); !errors.Is(err, ErrConflict) {
		t.Errorf("proof against another record: %v", err)
	}
	update.ProvenRecordHash = HashOpaqueRecord([]byte{1})
	update.TokenHash = []byte("unknown")
	if _, err := database.UpdateAccountCredentials(ctx, update); !errors.Is(err, ErrNotFound) {
		t.Errorf("ended session: %v", err)
	}
	if account, _ := database.GetAccountByID(ctx, "ALICE1"); string(account.OpaqueRecord) != string([]byte{1}) {
		t.Fatal("a refused change was written")
	}

	update.TokenHash = aliceSession.TokenHash
	signedOutDeviceCount, err := database.UpdateAccountCredentials(ctx, update)
	if err != nil || signedOutDeviceCount != 1 {
		t.Fatalf("change: %d %v", signedOutDeviceCount, err)
	}
	account, err := database.GetAccountByID(ctx, "ALICE1")
	if err != nil || string(account.Salt) != string([]byte{2}) || string(account.OpaqueRecord) != string([]byte{3}) || string(account.EncKeyset) != string([]byte{4}) {
		t.Fatalf("account after change: %+v %v", account, err)
	}
	if _, err := database.GetSessionByHash(ctx, laptopSession.TokenHash, time.Hour); !errors.Is(err, ErrNotFound) {
		t.Errorf("other device still signed in: %v", err)
	}
	if _, err := database.GetSessionByHash(ctx, aliceSession.TokenHash, time.Hour); err != nil {
		t.Errorf("changing device signed out: %v", err)
	}
	if devices, _ := database.ListDevices(ctx, "ALICE1"); len(devices) != 1 || devices[0].ID != aliceSession.DeviceID {
		t.Errorf("devices after change: %+v", devices)
	}
	if _, err := database.UpdateAccountCredentials(ctx, update); !errors.Is(err, ErrConflict) {
		t.Errorf("second change with the same proof: %v", err)
	}
}

func TestDeleteAccount(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	_, aliceSession := createTestAccount(t, database, "ALICE1", "alice@example.com")
	createTestAccount(t, database, "BOB111", "bob@example.com")
	createTestAccount(t, database, "CAROL1", "carol@example.com")
	aliceID := "ALICE1"

	soleVault := createTestVault(t, database, aliceID)
	coOwnedVault := createTestVault(t, database, aliceID)
	bobVault := createTestVault(t, database, "BOB111")
	for _, grant := range []struct {
		vaultID string
		member  Member
	}{
		{soleVault, Member{AccountID: "BOB111", Role: "editor", WrappedKey: []byte{1}, GrantedBy: aliceID}},
		{soleVault, Member{AccountID: "CAROL1", Role: "reader", WrappedKey: []byte{1}, GrantedBy: aliceID}},
		{coOwnedVault, Member{AccountID: "CAROL1", Role: "owner", WrappedKey: []byte{1}, GrantedBy: aliceID}},
		{bobVault, Member{AccountID: aliceID, Role: "editor", WrappedKey: []byte{1}, GrantedBy: "BOB111"}},
	} {
		if err := database.SetAccess(ctx, grant.vaultID, grant.member); err != nil {
			t.Fatal(err)
		}
	}
	for _, vaultID := range []string{soleVault, coOwnedVault} {
		if _, err := database.PutItem(ctx, vaultID, "", 0, &Item{ID: uuid.NewString(), Revision: uuid.NewString(), Overview: []byte{1}, Details: []byte{1}, UpdatedBy: aliceID}); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.CreateShareLink(ctx, &ShareLink{ID: "link-alice", AccountID: aliceID, Ciphertext: []byte{1}, EncryptedLabel: []byte{1}}, time.Hour, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := createTestShareInvitation(t, database, ctx, &ShareInvitation{ID: uuid.NewString(), VaultID: soleVault, Email: "dave@example.com", Role: "reader", InvitedBy: aliceID}, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := createTestShareInvitation(t, database, ctx, &ShareInvitation{ID: uuid.NewString(), VaultID: bobVault, Email: "alice@example.com", Role: "reader", InvitedBy: "BOB111"}, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := database.CreateInvite(ctx, []byte("alice-sent"), "erin@example.com", "member", &aliceID, time.Hour); err != nil {
		t.Fatal(err)
	}

	preview, err := database.PreviewAccountDeletion(ctx, aliceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.OwnedVaults) != 1 || preview.OwnedVaults[0].ID != soleVault || preview.OwnedVaults[0].OtherMemberCount != 2 {
		t.Errorf("owned vaults: %+v", preview.OwnedVaults)
	}
	memberVaultIDs := []string{}
	for _, memberVault := range preview.MemberVaults {
		memberVaultIDs = append(memberVaultIDs, memberVault.ID)
	}
	slices.Sort(memberVaultIDs)
	expectedMemberVaultID := []string{coOwnedVault, bobVault}
	slices.Sort(expectedMemberVaultID)
	if !slices.Equal(memberVaultIDs, expectedMemberVaultID) {
		t.Errorf("member vaults: %+v", preview.MemberVaults)
	}
	if preview.ShareLinkCount != 1 || preview.SentInvitationCount != 2 || preview.ReceivedInvitationCount != 1 || preview.DeviceCount != 1 {
		t.Errorf("preview counts: %+v", preview)
	}

	if _, err := database.DeleteAccount(ctx, aliceID, aliceSession.TokenHash, HashOpaqueRecord([]byte{7}), reviewStoreDeletion(t, database, ctx, aliceID)); !errors.Is(err, ErrConflict) {
		t.Fatalf("delete with a stale proof: %v", err)
	}
	if _, err := database.GetAccountByID(ctx, aliceID); err != nil {
		t.Fatalf("refused deletion removed the account: %v", err)
	}
	deleted, err := database.DeleteAccount(ctx, aliceID, aliceSession.TokenHash, HashOpaqueRecord([]byte{1}), reviewStoreDeletion(t, database, ctx, aliceID))
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Email != "alice@example.com" || deleted.DeletedVaultCount != 1 {
		t.Errorf("deleted: %+v", deleted)
	}
	slices.Sort(deleted.VaultMemberAccountIDs)
	if !slices.Equal(deleted.VaultMemberAccountIDs, []string{"BOB111", "CAROL1"}) {
		t.Errorf("vault members to notify: %v", deleted.VaultMemberAccountIDs)
	}
	if !slices.Contains(deleted.InvitationAccountIDs, "BOB111") {
		t.Errorf("invitation accounts to notify: %v", deleted.InvitationAccountIDs)
	}

	for name, query := range map[string]string{
		"account":          `SELECT count(*) FROM accounts WHERE id = $1`,
		"devices":          `SELECT count(*) FROM devices WHERE account_id = $1`,
		"sessions":         `SELECT count(*) FROM sessions WHERE account_id = $1`,
		"access":           `SELECT count(*) FROM vault_access WHERE account_id = $1`,
		"share links":      `SELECT count(*) FROM share_links WHERE account_id = $1`,
		"sent invitations": `SELECT count(*) FROM share_invitations WHERE invited_by = $1 OR email = 'alice@example.com'`,
		"sign-up invites":  `SELECT count(*) FROM invites WHERE created_by = $1 OR email = 'alice@example.com'`,
	} {
		if rowCount := countQueryRow(t, database, query, aliceID); rowCount != 0 {
			t.Errorf("%s left after deletion: %d", name, rowCount)
		}
	}
	if countQueryRow(t, database, `SELECT count(*) FROM vaults WHERE id = $1`, soleVault) != 0 || countQueryRow(t, database, `SELECT count(*) FROM items WHERE vault_id = $1`, soleVault) != 0 {
		t.Error("the vault alice owned alone is still there")
	}
	if countQueryRow(t, database, `SELECT count(*) FROM items WHERE vault_id = $1`, coOwnedVault) != 1 {
		t.Error("the co-owned vault lost its items")
	}
	if role, err := database.GetVaultRole(ctx, coOwnedVault, "CAROL1"); err != nil || role != "owner" {
		t.Errorf("co-owner after deletion: %q %v", role, err)
	}
	if role, err := database.GetVaultRole(ctx, bobVault, "BOB111"); err != nil || role != "owner" {
		t.Errorf("bob's vault after deletion: %q %v", role, err)
	}
	if countQueryRow(t, database, `SELECT count(*) FROM invites WHERE email = 'erin@example.com'`) != 0 {
		t.Error("the pending sign-up invite alice sent is still there")
	}
}

func TestDeleteAccountForgetsItsSettingsSave(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	_, aliceSession := createTestAccount(t, database, "ALICE1", "alice@example.com")
	if err := database.UpdateInstanceSettings(ctx, "ALICE1", false, func(document []byte) ([]byte, error) { return document, nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := database.DeleteAccount(ctx, "ALICE1", aliceSession.TokenHash, HashOpaqueRecord([]byte{1}), reviewStoreDeletion(t, database, ctx, "ALICE1")); err != nil {
		t.Fatal(err)
	}
	if count := countQueryRow(t, database, `SELECT count(*) FROM instance_settings WHERE updated_by = 'ALICE1'`); count != 0 {
		t.Error("the settings row still names the deleted account")
	}
}

func TestDeleteAccountSeesOwnershipChangesThatCommitMeanwhile(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	_, aliceSession := createTestAccount(t, database, "ALICE1", "alice@example.com")
	createTestAccount(t, database, "BOB111", "bob@example.com")
	createTestAccount(t, database, "CAROL1", "carol@example.com")
	coOwnedVault := createTestVault(t, database, "ALICE1")
	soleVault := createTestVault(t, database, "ALICE1")
	for _, grant := range []struct {
		vaultID string
		member  Member
	}{
		{coOwnedVault, Member{AccountID: "BOB111", Role: "owner", WrappedKey: []byte{1}, GrantedBy: "ALICE1"}},
		{soleVault, Member{AccountID: "CAROL1", Role: "editor", WrappedKey: []byte{1}, GrantedBy: "ALICE1"}},
	} {
		if err := database.SetAccess(ctx, grant.vaultID, grant.member); err != nil {
			t.Fatal(err)
		}
	}

	err := runWhileHolding(t, database, func(transaction pgx.Tx) error {
		for _, statement := range []struct {
			query   string
			vaultID string
		}{
			{`SELECT 1 FROM vaults WHERE id = $1 FOR UPDATE`, coOwnedVault},
			{`DELETE FROM vault_access WHERE vault_id = $1 AND account_id = 'BOB111'`, coOwnedVault},
			{`SELECT 1 FROM vaults WHERE id = $1 FOR UPDATE`, soleVault},
			{`UPDATE vault_access SET role = 'owner' WHERE vault_id = $1 AND account_id = 'CAROL1'`, soleVault},
		} {
			if _, err := transaction.Exec(ctx, statement.query, statement.vaultID); err != nil {
				return err
			}
		}
		return nil
	}, func() error {
		_, err := database.DeleteAccount(ctx, "ALICE1", aliceSession.TokenHash, testRecordHash(), reviewStoreDeletion(t, database, ctx, "ALICE1"))
		return err
	})
	if !errors.Is(err, ErrDeletionPlanChanged) {
		t.Fatalf("a deletion reviewed before the ownership changes: %v", err)
	}
	if _, err := database.DeleteAccount(ctx, "ALICE1", aliceSession.TokenHash, testRecordHash(), reviewStoreDeletion(t, database, ctx, "ALICE1")); err != nil {
		t.Fatal(err)
	}
	if vaultCount := countQueryRow(t, database, `SELECT count(*) FROM vaults WHERE id = $1`, coOwnedVault); vaultCount != 0 {
		t.Errorf("the vault whose other owner left meanwhile stayed without an owner: %d members",
			countQueryRow(t, database, `SELECT count(*) FROM vault_access WHERE vault_id = $1`, coOwnedVault))
	}
	if role, err := database.GetVaultRole(ctx, soleVault, "CAROL1"); err != nil || role != "owner" {
		t.Errorf("the vault that got another owner meanwhile: %q %v", role, err)
	}
}

func TestProvenChangesAreCheckedAgainstARevocationThatCommitsMeanwhile(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	device, session := createTestAccount(t, database, "ALICE1", "alice@example.com")
	revokeMeanwhile := func(change func() error) error {
		t.Helper()
		return runWhileHolding(t, database, func(transaction pgx.Tx) error {
			_, err := revokeDevice(ctx, transaction, "ALICE1", device.ID)
			return err
		}, change)
	}

	update := &CredentialUpdate{AccountID: "ALICE1", TokenHash: session.TokenHash, ProvenRecordHash: testRecordHash(),
		Argon2Params: []byte(`{}`), Salt: []byte{2}, OpaqueRecord: []byte{2}, EncKeyset: []byte{2}}
	if err := revokeMeanwhile(func() error { _, err := database.UpdateAccountCredentials(ctx, update); return err }); !errors.Is(err, ErrNotFound) {
		t.Errorf("a credential change from a device revoked meanwhile: %v", err)
	}
	if account, _ := database.GetAccountByID(ctx, "ALICE1"); string(account.OpaqueRecord) != string([]byte{1}) {
		t.Error("the credential change of a revoked device was written")
	}

	if _, err := database.pool.Exec(ctx, `UPDATE accounts SET opaque_record = '\x01' WHERE id = 'ALICE1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.pool.Exec(ctx, `UPDATE devices SET revoked_at = NULL WHERE id = $1`, device.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.pool.Exec(ctx, `INSERT INTO sessions (token_hash, account_id, device_id, expires_at) VALUES ($1, 'ALICE1', $2, now() + interval '1 hour')`,
		session.TokenHash, device.ID); err != nil {
		t.Fatal(err)
	}
	if err := revokeMeanwhile(func() error {
		_, err := database.DeleteAccount(ctx, "ALICE1", session.TokenHash, testRecordHash(), reviewStoreDeletion(t, database, ctx, "ALICE1"))
		return err
	}); !errors.Is(err, ErrNotFound) {
		t.Errorf("an account deletion from a device revoked meanwhile: %v", err)
	}
	if _, err := database.GetAccountByID(ctx, "ALICE1"); err != nil {
		t.Errorf("the account deletion of a revoked device went through: %v", err)
	}
}
