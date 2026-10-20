package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
)

// createTestOrganization creates an organization with AAAAAA as its admin and the other accounts as members.
func createTestOrganization(t *testing.T, database *Store, memberIDs ...string) string {
	t.Helper()
	ctx := context.Background()
	organizationID := uuid.NewString()
	if err := database.CreateOrganization(ctx, organizationID, "Acme", "AAAAAA"); err != nil {
		t.Fatal(err)
	}
	for _, memberID := range memberIDs {
		if _, err := database.pool.Exec(ctx, `INSERT INTO organization_members (organization_id, account_id, role, added_by) VALUES ($1, $2, 'member', 'AAAAAA')`,
			organizationID, memberID); err != nil {
			t.Fatal(err)
		}
	}
	return organizationID
}

func createTestTeam(t *testing.T, database *Store, organizationID string, memberIDs ...string) string {
	t.Helper()
	ctx := context.Background()
	teamID := uuid.NewString()
	if _, err := database.CreateTeam(ctx, organizationID, "AAAAAA", teamID, teamID); err != nil {
		t.Fatal(err)
	}
	for _, memberID := range memberIDs {
		if _, err := database.AddTeamMember(ctx, organizationID, "AAAAAA", teamID, memberID); err != nil {
			t.Fatal(err)
		}
	}
	return teamID
}

func createTestOrganizationVault(t *testing.T, database *Store, organizationID, teamID string) string {
	t.Helper()
	vault := &Vault{ID: uuid.NewString(), Type: "organization", EncName: []byte{1}, CreatedBy: "AAAAAA", OrganizationID: organizationID}
	if _, err := database.CreateOrganizationVault(context.Background(), vault, []byte{1}, teamID); err != nil {
		t.Fatal(err)
	}
	return vault.ID
}

func queryNeedsRotation(t *testing.T, database *Store, vaultID string) bool {
	t.Helper()
	var needsRotation bool
	if err := database.pool.QueryRow(context.Background(), `SELECT needs_rotation FROM vaults WHERE id = $1`, vaultID).Scan(&needsRotation); err != nil {
		t.Fatal(err)
	}
	return needsRotation
}

func TestReconcileGivesTheStrongestTeamRole(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	createTestAccount(t, database, "BBBBBB", "b@example.com")
	organizationID := createTestOrganization(t, database, "BBBBBB")
	ownersTeamID := createTestTeam(t, database, organizationID, "AAAAAA")
	editorsTeamID := createTestTeam(t, database, organizationID, "BBBBBB")
	readersTeamID := createTestTeam(t, database, organizationID, "BBBBBB")
	vaultID := createTestOrganizationVault(t, database, organizationID, ownersTeamID)
	for teamID, role := range map[string]string{editorsTeamID: "editor", readersTeamID: "reader"} {
		if _, err := database.GrantTeamVault(ctx, organizationID, "AAAAAA", teamID, vaultID, role); err != nil {
			t.Fatal(err)
		}
	}

	keyRequests, err := database.ListVaultKeyRequests(ctx, "AAAAAA")
	if err != nil || len(keyRequests) != 1 || keyRequests[0].Recipient.ID != "BBBBBB" || keyRequests[0].Role != "editor" || keyRequests[0].KeyGeneration != 1 {
		t.Fatalf("key requests: %+v %v", keyRequests, err)
	}
	if isNewMember, err := database.GrantAccess(ctx, vaultID, Member{AccountID: "BBBBBB", Role: "editor", WrappedKey: []byte{1}, GrantedBy: "AAAAAA"}, 1); err != nil || !isNewMember {
		t.Fatalf("delivery: %v %v", isNewMember, err)
	}

	change, err := database.RemoveTeamMember(ctx, organizationID, "AAAAAA", editorsTeamID, "BBBBBB")
	if err != nil {
		t.Fatal(err)
	}
	if role, _ := database.GetVaultRole(ctx, vaultID, "BBBBBB"); role != "reader" {
		t.Errorf("role after leaving the editors: %q", role)
	}
	if queryNeedsRotation(t, database, vaultID) {
		t.Error("a role change marked the vault for rotation")
	}
	if !slices.Equal(change.VaultAccountIDs, []string{"AAAAAA", "BBBBBB"}) || !slices.Equal(change.AccountIDs, []string{"AAAAAA", "BBBBBB"}) {
		t.Errorf("change: %+v", change)
	}

	if _, err := database.RemoveTeamMember(ctx, organizationID, "AAAAAA", readersTeamID, "BBBBBB"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.GetVaultRole(ctx, vaultID, "BBBBBB"); !errors.Is(err, ErrNotFound) {
		t.Errorf("access after leaving every team of the vault: %v", err)
	}
	if !queryNeedsRotation(t, database, vaultID) {
		t.Error("the vault is not marked for rotation after a removal")
	}
}

// Item writes change no access, so they lock only their vault (§5.11): a write held open in one vault of an organization
// does not hold up a write to another of its vaults.
func TestItemWritesToOrganizationVaultsRunConcurrently(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	organizationID := createTestOrganization(t, database)
	teamID := createTestTeam(t, database, organizationID, "AAAAAA")
	firstVaultID := createTestOrganizationVault(t, database, organizationID, teamID)
	secondVaultID := createTestOrganizationVault(t, database, organizationID, teamID)
	createItem := func() *Item {
		return &Item{ID: uuid.NewString(), Revision: uuid.NewString(), Overview: []byte{1}, Details: []byte{1}, UpdatedBy: "AAAAAA"}
	}

	transaction, err := database.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback(ctx)
	if _, err := putItem(ctx, transaction, firstVaultID, "", 1, createItem()); err != nil {
		t.Fatal(err)
	}
	writeContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := database.PutItem(writeContext, secondVaultID, "", 1, createItem()); err != nil {
		t.Fatalf("a write to another vault of the organization waited for the open one: %v", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestOrganizationVaultMemberLimit(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	organizationID := createTestOrganization(t, database)
	ownersTeamID := createTestTeam(t, database, organizationID, "AAAAAA")
	vaultID := createTestOrganizationVault(t, database, organizationID, ownersTeamID)
	crowdTeamID := createTestTeam(t, database, organizationID)
	for _, statement := range []struct {
		query     string
		arguments []any
	}{
		{`INSERT INTO accounts (id, email, name, role, argon2_params, salt, opaque_record, enc_keyset, kem_pub, sign_pub, kem_sig)
			SELECT 'X' || lpad(n::text, 5, '0'), 'x' || n || '@example.com', 'x', 'member', '{}', '\x01', '\x01', '\x01', '\x01', '\x01', '\x01'
			FROM generate_series(1, $1::int) n`, []any{maxEntitledAccountCount}},
		{`INSERT INTO organization_members (organization_id, account_id, role, added_by)
			SELECT $1::uuid, id, 'member', 'AAAAAA' FROM accounts WHERE id LIKE 'X%'`, []any{organizationID}},
		{`INSERT INTO team_members (team_id, organization_id, account_id, added_by)
			SELECT $1::uuid, $2::uuid, id, 'AAAAAA' FROM accounts WHERE id LIKE 'X%'`, []any{crowdTeamID, organizationID}},
	} {
		if _, err := database.pool.Exec(ctx, statement.query, statement.arguments...); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.GrantTeamVault(ctx, organizationID, "AAAAAA", crowdTeamID, vaultID, "reader"); !errors.Is(err, ErrTooManyVaultMembers) {
		t.Fatalf("giving the vault to %d more accounts: %v", maxEntitledAccountCount, err)
	}
	if countQueryRow(t, database, `SELECT count(*) FROM team_vaults WHERE team_id = $1`, crowdTeamID) != 0 {
		t.Error("the refused grant was stored")
	}
}

func TestOrganizationMigrationRollbackKeepsOrganizationVaults(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	organizationID := createTestOrganization(t, database)
	vaultID := createTestOrganizationVault(t, database, organizationID, createTestTeam(t, database, organizationID, "AAAAAA"))
	rollbackTestMigrationsTo(t, database, 8)
	if _, err := database.RollbackMigration(ctx); err == nil {
		t.Fatal("the organizations migration rolled back over an organization vault")
	}
	if countTable(t, database, "organizations") != 1 || countQueryRow(t, database, `SELECT count(*) FROM vaults WHERE id = $1`, vaultID) != 1 {
		t.Error("the refused rollback changed the schema or the vault")
	}
}

func TestPruneExpiredOrganizationInvitation(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	organizationID := createTestOrganization(t, database)
	for email, ttl := range map[string]time.Duration{"old@example.com": -40 * 24 * time.Hour, "recent@example.com": -time.Hour, "live@example.com": time.Hour} {
		if _, _, err := database.UpsertOrganizationInvitation(ctx, &OrganizationInvitation{OrganizationID: organizationID, Email: email, Role: "member", InvitedBy: "AAAAAA"}, ttl); err != nil {
			t.Fatal(err)
		}
	}
	prunedCount, err := database.PruneExpiredInvitation(ctx, 30*24*time.Hour)
	if err != nil || prunedCount != 1 {
		t.Fatalf("pruned %d, %v", prunedCount, err)
	}
	if countQueryRow(t, database, `SELECT count(*) FROM organization_invitations WHERE email IN ('recent@example.com', 'live@example.com')`) != 2 {
		t.Error("an invitation that expired recently, or not at all, was pruned")
	}
}
