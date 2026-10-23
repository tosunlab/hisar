package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func setTestRole(t *testing.T, database *Store, accountID, role string) {
	t.Helper()
	if _, err := database.pool.Exec(context.Background(), `UPDATE accounts SET role = $2 WHERE id = $1`, accountID, role); err != nil {
		t.Fatal(err)
	}
}

// grantTestVaultKey gives the account a wrapped key to the vault, as an owner's app does for a team member.
func grantTestVaultKey(t *testing.T, database *Store, vaultID, accountID string) {
	t.Helper()
	if _, err := database.pool.Exec(context.Background(), `INSERT INTO vault_access (vault_id, account_id, wrapped_key, role, granted_by) VALUES ($1, $2, '\x01', 'editor', 'AAAAAA')`,
		vaultID, accountID); err != nil {
		t.Fatal(err)
	}
}

func listTestAccountEmails(t *testing.T, database *Store, query, afterEmail string, limit int) []string {
	t.Helper()
	accounts, err := database.ListAdminAccounts(context.Background(), query, afterEmail, limit)
	if err != nil {
		t.Fatal(err)
	}
	emails := []string{}
	for _, account := range accounts {
		emails = append(emails, account.Email)
	}
	return emails
}

func TestAdminAccountsPagingAndSearch(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	for index := range 5 {
		createTestAccount(t, database, fmt.Sprintf("ACCT%02d", index), fmt.Sprintf("user%d@example.com", 4-index))
	}
	createTestAccount(t, database, "ZOE001", "zoe@other.example")
	if _, err := database.pool.Exec(ctx, `UPDATE accounts SET name = 'Zoë Yılmaz' WHERE id = 'ZOE001'`); err != nil {
		t.Fatal(err)
	}

	firstPage := listTestAccountEmails(t, database, "", "", 4)
	if !slices.Equal(firstPage, []string{"user0@example.com", "user1@example.com", "user2@example.com", "user3@example.com"}) {
		t.Errorf("first page: %v", firstPage)
	}
	if secondPage := listTestAccountEmails(t, database, "", firstPage[len(firstPage)-1], 4); !slices.Equal(secondPage, []string{"user4@example.com", "zoe@other.example"}) {
		t.Errorf("second page: %v", secondPage)
	}
	if found := listTestAccountEmails(t, database, "USER3", "", 50); !slices.Equal(found, []string{"user3@example.com"}) {
		t.Errorf("email search: %v", found)
	}
	if found := listTestAccountEmails(t, database, "yılmaz", "", 50); !slices.Equal(found, []string{"zoe@other.example"}) {
		t.Errorf("name search: %v", found)
	}
	if found := listTestAccountEmails(t, database, "%", "", 50); len(found) != 0 {
		t.Errorf("a wildcard matched: %v", found)
	}
	if found := listTestAccountEmails(t, database, "example.com", "user2@example.com", 50); !slices.Equal(found, []string{"user3@example.com", "user4@example.com"}) {
		t.Errorf("search after a cursor: %v", found)
	}

	device, _ := createTestAccount(t, database, "DEVICE", "devices@example.com")
	second := &Device{ID: uuid.NewString(), AccountID: "DEVICE", Name: "laptop", Platform: "darwin/desktop"}
	if err := database.CreateDeviceSession(ctx, second, &Session{TokenHash: []byte("laptop"), AccountID: "DEVICE", DeviceID: second.ID, ExpiresAt: time.Now().Add(time.Hour)}, testRecordHash()); err != nil {
		t.Fatal(err)
	}
	if _, err := database.RevokeDevice(ctx, "DEVICE", device.ID); err != nil {
		t.Fatal(err)
	}
	createTestVault(t, database, "DEVICE")
	account, err := database.GetAdminAccount(ctx, "DEVICE")
	if err != nil || account.DeviceCount != 1 || account.VaultCount != 1 || account.LastSeenAt == nil || account.SuspendedAt != nil {
		t.Errorf("account: %+v %v", account, err)
	}
	devices, err := database.ListAdminDevices(ctx, "DEVICE")
	if err != nil || len(devices) != 2 || devices[0].ID != second.ID || devices[0].RevokedAt != nil || devices[1].RevokedAt == nil {
		t.Errorf("devices: %+v %v", devices, err)
	}
	if _, err := database.GetAdminAccount(ctx, "NOBODY"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown account: %v", err)
	}
}

func TestSuspendAccount(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "admin@example.com")
	setTestRole(t, database, "AAAAAA", "admin")
	bobDevice, bobSession := createTestAccount(t, database, "BOBBBB", "bob@example.com")
	organizationID := createTestOrganization(t, database, "BOBBBB")
	vaultID := createTestOrganizationVault(t, database, organizationID, createTestTeam(t, database, organizationID, "AAAAAA", "BOBBBB"))
	grantTestVaultKey(t, database, vaultID, "BOBBBB")
	createTestShareLink(t, database, "bob-link", "BOBBBB", 0, time.Hour)

	if suspended, err := database.SuspendAccount(ctx, "AAAAAA", "BOBBBB"); err != nil || !suspended {
		t.Fatal(suspended, err)
	}
	if suspended, err := database.SuspendAccount(ctx, "AAAAAA", "BOBBBB"); err != nil || suspended {
		t.Errorf("suspending a suspended account: %v %v", suspended, err)
	}
	if _, err := database.GetSessionByHash(ctx, bobSession.TokenHash, time.Hour); !errors.Is(err, ErrAccountSuspended) {
		t.Errorf("a suspended account's session: %v", err)
	}
	laptop := &Device{ID: uuid.NewString(), AccountID: "BOBBBB", Name: "laptop", Platform: "p"}
	if err := database.CreateDeviceSession(ctx, laptop, &Session{TokenHash: []byte("bob laptop"), AccountID: "BOBBBB", DeviceID: laptop.ID, ExpiresAt: time.Now().Add(time.Hour)}, testRecordHash()); !errors.Is(err, ErrAccountSuspended) {
		t.Errorf("a suspended account signed in: %v", err)
	}
	if countQueryRow(t, database, `SELECT count(*) FROM devices WHERE id = $1 AND revoked_at IS NULL`, bobDevice.ID) != 1 ||
		countQueryRow(t, database, `SELECT count(*) FROM sessions WHERE token_hash = $1`, bobSession.TokenHash) != 1 {
		t.Error("suspension signed out a device")
	}
	if _, err := database.ViewShareLink(ctx, "bob-link"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a suspended account's share link opened: %v", err)
	}
	if countQueryRow(t, database, `SELECT count(*) FROM organization_members WHERE organization_id = $1 AND account_id = 'BOBBBB'`, organizationID) != 1 ||
		countQueryRow(t, database, `SELECT count(*) FROM vault_access WHERE vault_id = $1 AND account_id = 'BOBBBB'`, vaultID) != 1 || queryNeedsRotation(t, database, vaultID) {
		t.Error("suspension changed the organization membership or the vault access")
	}
	if account, err := database.GetAdminAccount(ctx, "BOBBBB"); err != nil || account.SuspendedAt == nil {
		t.Errorf("suspended account: %+v %v", account, err)
	}

	if reactivated, err := database.ReactivateAccount(ctx, "AAAAAA", "BOBBBB"); err != nil || !reactivated {
		t.Fatal(reactivated, err)
	}
	if reactivated, err := database.ReactivateAccount(ctx, "AAAAAA", "BOBBBB"); err != nil || reactivated {
		t.Errorf("reactivating an active account: %v %v", reactivated, err)
	}
	if err := database.CreateDeviceSession(ctx, laptop, &Session{TokenHash: []byte("bob laptop"), AccountID: "BOBBBB", DeviceID: laptop.ID, ExpiresAt: time.Now().Add(time.Hour)}, testRecordHash()); err != nil {
		t.Errorf("reactivated account: %v", err)
	}
	if _, err := database.GetSessionByHash(ctx, []byte("bob laptop"), time.Hour); err != nil {
		t.Errorf("reactivated account's new session: %v", err)
	}
	if _, err := database.GetSessionByHash(ctx, bobSession.TokenHash, time.Hour); err != nil {
		t.Errorf("the session from before the suspension after reactivation: %v", err)
	}
	if _, err := database.ViewShareLink(ctx, "bob-link"); err != nil {
		t.Errorf("a reactivated account's share link: %v", err)
	}
	if _, err := database.ReactivateAccount(ctx, "AAAAAA", "NOBODY"); !errors.Is(err, ErrNotFound) {
		t.Errorf("reactivating an unknown account: %v", err)
	}
}

func TestServerKeepsAnActiveAdmin(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	createTestAccount(t, database, "BBBBBB", "b@example.com")
	setTestRole(t, database, "AAAAAA", "admin")

	for name, change := range map[string]func() error{
		"demote": func() error { return database.SetAccountRole(ctx, AdminActor{AccountID: "AAAAAA"}, "AAAAAA", "member") },
		"suspend": func() error {
			_, err := database.SuspendAccount(ctx, "AAAAAA", "AAAAAA")
			return err
		},
		"delete": func() error {
			_, err := database.DeleteAccountAsAdmin(ctx, AdminActor{AccountID: "AAAAAA"}, "AAAAAA")
			return err
		},
	} {
		if err := change(); !errors.Is(err, ErrLastServerAdmin) {
			t.Errorf("%s the last admin: %v", name, err)
		}
	}
	if err := database.SetAccountRole(ctx, AdminActor{AccountID: "AAAAAA"}, "AAAAAA", "admin"); err != nil {
		t.Errorf("keeping the last admin's role: %v", err)
	}

	setTestRole(t, database, "BBBBBB", "admin")
	if _, err := database.SuspendAccount(ctx, "AAAAAA", "BBBBBB"); err != nil {
		t.Fatal(err)
	}
	if err := database.SetAccountRole(ctx, AdminActor{AccountID: "AAAAAA"}, "AAAAAA", "member"); !errors.Is(err, ErrLastServerAdmin) {
		t.Errorf("a suspended admin counted as active: %v", err)
	}
	if err := database.SetAccountRole(ctx, AdminActor{AccountID: "AAAAAA"}, "BBBBBB", "member"); err != nil {
		t.Errorf("demoting a suspended admin: %v", err)
	}
	if _, err := database.ReactivateAccount(ctx, "AAAAAA", "BBBBBB"); err != nil {
		t.Fatal(err)
	}

	// Two admins who demote each other at once: one change waits for the other and then finds no other admin.
	setTestRole(t, database, "BBBBBB", "admin")
	for range 10 {
		var changes sync.WaitGroup
		results := make([]error, 2)
		for index, accountID := range []string{"AAAAAA", "BBBBBB"} {
			otherAccountID := map[string]string{"AAAAAA": "BBBBBB", "BBBBBB": "AAAAAA"}[accountID]
			changes.Go(func() {
				results[index] = database.SetAccountRole(ctx, AdminActor{AccountID: otherAccountID}, accountID, "member")
			})
		}
		changes.Wait()
		if countQueryRow(t, database, `SELECT count(*) FROM accounts WHERE role = 'admin' AND suspended_at IS NULL`) != 1 ||
			(results[0] == nil) == (results[1] == nil) || !errors.Is(errors.Join(results...), ErrLastServerAdmin) {
			t.Fatalf("mutual demotion: %v", results)
		}
		setTestRole(t, database, "AAAAAA", "admin")
		setTestRole(t, database, "BBBBBB", "admin")
	}
}

func TestDeleteAccountAsAdmin(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "admin@example.com")
	setTestRole(t, database, "AAAAAA", "admin")
	createTestAccount(t, database, "BOBBBB", "bob@example.com")
	createTestAccount(t, database, "CAROL1", "carol@example.com")

	sharedVaultID := createTestVault(t, database, "BOBBBB")
	if _, err := database.pool.Exec(ctx, `INSERT INTO vault_access (vault_id, account_id, wrapped_key, role, granted_by) VALUES ($1, 'AAAAAA', '\x01', 'owner', 'BOBBBB')`, sharedVaultID); err != nil {
		t.Fatal(err)
	}
	ownVaultID := createTestVault(t, database, "BOBBBB")
	// Bob is the organization's admin, Carol its other member: she becomes admin. Bob alone is in the second one.
	bobOrganizationID := uuid.NewString()
	if err := database.CreateOrganization(ctx, bobOrganizationID, "Bob's", "BOBBBB"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.pool.Exec(ctx, `INSERT INTO organization_members (organization_id, account_id, role, added_by) VALUES ($1, 'CAROL1', 'member', 'BOBBBB')`, bobOrganizationID); err != nil {
		t.Fatal(err)
	}
	soloOrganizationID := uuid.NewString()
	if err := database.CreateOrganization(ctx, soloOrganizationID, "Solo", "BOBBBB"); err != nil {
		t.Fatal(err)
	}
	acmeID := createTestOrganization(t, database, "BOBBBB")
	acmeVaultID := createTestOrganizationVault(t, database, acmeID, createTestTeam(t, database, acmeID, "AAAAAA", "BOBBBB"))
	grantTestVaultKey(t, database, acmeVaultID, "BOBBBB")

	deleted, err := database.DeleteAccountAsAdmin(ctx, AdminActor{AccountID: "AAAAAA"}, "BOBBBB")
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Email != "bob@example.com" || deleted.DeletedVaultCount != 1 || !slices.Contains(deleted.VaultMemberAccountIDs, "AAAAAA") || !slices.Contains(deleted.OrganizationAccountIDs, "CAROL1") {
		t.Errorf("deleted: %+v", deleted)
	}
	for name, count := range map[string]int{
		"account":              countQueryRow(t, database, `SELECT count(*) FROM accounts WHERE id = 'BOBBBB'`),
		"own vault":            countQueryRow(t, database, `SELECT count(*) FROM vaults WHERE id = $1`, ownVaultID),
		"vault access":         countQueryRow(t, database, `SELECT count(*) FROM vault_access WHERE account_id = 'BOBBBB'`),
		"solo organization":    countQueryRow(t, database, `SELECT count(*) FROM organizations WHERE id = $1`, soloOrganizationID),
		"organization members": countQueryRow(t, database, `SELECT count(*) FROM organization_members WHERE account_id = 'BOBBBB'`),
		"team members":         countQueryRow(t, database, `SELECT count(*) FROM team_members WHERE account_id = 'BOBBBB'`),
	} {
		if count != 0 {
			t.Errorf("%s left: %d", name, count)
		}
	}
	if countQueryRow(t, database, `SELECT count(*) FROM vaults WHERE id = $1`, sharedVaultID) != 1 {
		t.Error("the vault the admin co-owns was deleted")
	}
	if countQueryRow(t, database, `SELECT count(*) FROM organization_members WHERE organization_id = $1 AND account_id = 'CAROL1' AND role = 'admin'`, bobOrganizationID) != 1 {
		t.Error("the remaining member did not become the organization's admin")
	}
	if !queryNeedsRotation(t, database, acmeVaultID) {
		t.Error("the organization vault Bob held a key to is not marked for rotation")
	}
	if _, err := database.DeleteAccountAsAdmin(ctx, AdminActor{AccountID: "AAAAAA"}, "BOBBBB"); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting again: %v", err)
	}
}

func TestSignOutAccount(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "admin@example.com")
	setTestRole(t, database, "AAAAAA", "admin")
	_, session := createTestAccount(t, database, "BOBBBB", "bob@example.com")
	laptop := &Device{ID: uuid.NewString(), AccountID: "BOBBBB", Name: "laptop", Platform: "p"}
	if err := database.CreateDeviceSession(ctx, laptop, &Session{TokenHash: []byte("laptop"), AccountID: "BOBBBB", DeviceID: laptop.ID, ExpiresAt: time.Now().Add(time.Hour)}, testRecordHash()); err != nil {
		t.Fatal(err)
	}
	if revokedDeviceCount, err := database.SignOutAccount(ctx, "AAAAAA", "BOBBBB"); err != nil || revokedDeviceCount != 2 {
		t.Fatalf("sign out: %d %v", revokedDeviceCount, err)
	}
	for _, tokenHash := range [][]byte{session.TokenHash, []byte("laptop")} {
		if _, err := database.GetSessionByHash(ctx, tokenHash, time.Hour); !errors.Is(err, ErrNotFound) {
			t.Errorf("session %s after sign out: %v", tokenHash, err)
		}
	}
	if countQueryRow(t, database, `SELECT count(*) FROM devices WHERE account_id = 'BOBBBB' AND revoked_at IS NULL`) != 0 {
		t.Error("a device stayed signed in")
	}
	if _, err := database.SignOutAccount(ctx, "AAAAAA", "NOBODY"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown account: %v", err)
	}
}

func TestAdminChangesNeedAnActiveAdmin(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	createTestAccount(t, database, "BBBBBB", "b@example.com")
	cDevice, _ := createTestAccount(t, database, "CCCCCC", "c@example.com")
	setTestRole(t, database, "AAAAAA", "admin")
	setTestRole(t, database, "BBBBBB", "admin")
	if _, err := database.SuspendAccount(ctx, "AAAAAA", "BBBBBB"); err != nil {
		t.Fatal(err)
	}

	// BBBBBB is a suspended admin and CCCCCC a member: neither may change an account.
	for _, actorAccountID := range []string{"BBBBBB", "CCCCCC"} {
		for name, change := range map[string]func() error{
			"role": func() error {
				return database.SetAccountRole(ctx, AdminActor{AccountID: actorAccountID}, "CCCCCC", "admin")
			},
			"suspend": func() error {
				_, err := database.SuspendAccount(ctx, actorAccountID, "CCCCCC")
				return err
			},
			"delete": func() error {
				_, err := database.DeleteAccountAsAdmin(ctx, AdminActor{AccountID: actorAccountID}, "CCCCCC")
				return err
			},
			"reactivate": func() error {
				_, err := database.ReactivateAccount(ctx, actorAccountID, "CCCCCC")
				return err
			},
			"sign-out": func() error {
				_, err := database.SignOutAccount(ctx, actorAccountID, "CCCCCC")
				return err
			},
			"device sign-out": func() error {
				_, err := database.RevokeAccountDevice(ctx, actorAccountID, "CCCCCC", cDevice.ID)
				return err
			},
		} {
			if err := change(); !errors.Is(err, ErrActorNotAdmin) {
				t.Errorf("%s by %s: %v", name, actorAccountID, err)
			}
		}
	}
	if countQueryRow(t, database, `SELECT count(*) FROM accounts WHERE id = 'CCCCCC' AND role = 'member' AND suspended_at IS NULL`) != 1 ||
		countQueryRow(t, database, `SELECT count(*) FROM devices WHERE id = $1 AND revoked_at IS NULL`, cDevice.ID) != 1 {
		t.Error("a change by an account that is not an active admin went through")
	}
}

func TestProvenAdminChangesCheckTheProof(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	_, adminSession := createTestAccount(t, database, "AAAAAA", "a@example.com")
	setTestRole(t, database, "AAAAAA", "admin")
	createTestAccount(t, database, "CCCCCC", "c@example.com")
	var opaqueRecord []byte
	if err := database.pool.QueryRow(ctx, `SELECT opaque_record FROM accounts WHERE id = 'AAAAAA'`).Scan(&opaqueRecord); err != nil {
		t.Fatal(err)
	}
	proven := AdminActor{AccountID: "AAAAAA", TokenHash: adminSession.TokenHash, ProvenRecordHash: HashOpaqueRecord(opaqueRecord)}
	changedRecord := proven
	changedRecord.ProvenRecordHash = HashOpaqueRecord([]byte("an older record"))
	endedSession := proven
	endedSession.TokenHash = []byte("a signed-out session")

	for name, test := range map[string]struct {
		actor AdminActor
		want  error
	}{"a changed record": {changedRecord, ErrConflict}, "an ended session": {endedSession, ErrSessionEnded}} {
		if err := database.SetAccountRole(ctx, test.actor, "CCCCCC", "admin"); !errors.Is(err, test.want) {
			t.Errorf("role change with %s: %v", name, err)
		}
		if _, err := database.DeleteAccountAsAdmin(ctx, test.actor, "CCCCCC"); !errors.Is(err, test.want) {
			t.Errorf("deletion with %s: %v", name, err)
		}
	}
	if countQueryRow(t, database, `SELECT count(*) FROM accounts WHERE id = 'CCCCCC' AND role = 'member'`) != 1 {
		t.Error("a change with a stale proof went through")
	}
	if err := database.SetAccountRole(ctx, proven, "CCCCCC", "admin"); err != nil {
		t.Errorf("role change with a valid proof: %v", err)
	}
	if _, err := database.DeleteAccountAsAdmin(ctx, proven, "CCCCCC"); err != nil {
		t.Errorf("deletion with a valid proof: %v", err)
	}
}

// A grant to an admin on a vault the deleted account owns: the grant holds the vault and then needs a key-share lock on
// the admin's row, while the deletion holds the active admins' rows and waits for the vault. Neither may deadlock.
func TestDeleteAccountAsAdminWhileAVaultIsGrantedToAnAdmin(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	createTestAccount(t, database, "CCCCCC", "c@example.com")
	setTestRole(t, database, "AAAAAA", "admin")
	setTestRole(t, database, "CCCCCC", "admin")
	createTestAccount(t, database, "BOBBBB", "bob@example.com")
	vaultID := createTestVault(t, database, "BOBBBB")

	grant, err := database.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer grant.Rollback(ctx)
	if _, err := grant.Exec(ctx, `SELECT 1 FROM vaults WHERE id = $1 FOR UPDATE`, vaultID); err != nil {
		t.Fatal(err)
	}
	deletion := make(chan error, 1)
	go func() {
		_, err := database.DeleteAccountAsAdmin(ctx, AdminActor{AccountID: "AAAAAA"}, "BOBBBB")
		deletion <- err
	}()
	deadline := time.Now().Add(10 * time.Second)
	for countQueryRow(t, database, `SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND query LIKE '%FROM vaults WHERE id IN (SELECT vault_id%'`) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the deletion never waited for the vault")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := grant.Exec(ctx, `INSERT INTO vault_access (vault_id, account_id, wrapped_key, role, granted_by) VALUES ($1, 'CCCCCC', '\x01', 'editor', 'BOBBBB')`, vaultID); err != nil {
		t.Fatalf("the grant: %v", err)
	}
	if err := grant.Commit(ctx); err != nil {
		t.Fatalf("the grant's commit: %v", err)
	}
	if err := <-deletion; err != nil {
		t.Errorf("the deletion: %v", err)
	}
}

func TestRefusedSessionOfASuspendedAccountDoesNotSlide(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	setTestRole(t, database, "AAAAAA", "admin")
	_, session := createTestAccount(t, database, "BBBBBB", "b@example.com")
	if suspended, err := database.SuspendAccount(ctx, "AAAAAA", "BBBBBB"); err != nil || !suspended {
		t.Fatalf("suspend: %v %v", suspended, err)
	}
	readExpiry := func() time.Time {
		t.Helper()
		var expiresAt time.Time
		if err := database.pool.QueryRow(ctx, `SELECT expires_at FROM sessions WHERE token_hash = $1`, session.TokenHash).Scan(&expiresAt); err != nil {
			t.Fatal(err)
		}
		return expiresAt
	}
	before := readExpiry()
	if _, err := database.GetSessionByHash(ctx, session.TokenHash, 30*24*time.Hour); !errors.Is(err, ErrAccountSuspended) {
		t.Errorf("a suspended account's session: %v", err)
	}
	if after := readExpiry(); !after.Equal(before) {
		t.Errorf("the refused session slid from %v to %v", before, after)
	}
}
