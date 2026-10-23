package store

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tryhisar/hisar/core/apitypes"
)

var (
	ErrNotOrganizationAdmin  = errors.New("store: not an admin of the organization")
	ErrLastAdmin             = errors.New("store: the organization's last admin")
	ErrLastMember            = errors.New("store: the organization's last member")
	ErrOrganizationHasVaults = errors.New("store: the organization has vaults")
	ErrNotTeamMember         = errors.New("store: not a member of the team")
	ErrNoVaultOwner          = errors.New("store: an organization vault would have no owner")
	ErrTooManyVaultMembers   = errors.New("store: an organization vault would have too many members")
)

// maxEntitledAccountCount is the most accounts an organization vault may be given to: the most one rotation carries.
const maxEntitledAccountCount = 1000

type OrganizationSummary struct {
	ID   string
	Name string
	// Role is the caller's role in the organization.
	Role        string
	MemberCount int
}

// OrganizationMember: Account has only the ID, email, name, role and public keys.
type OrganizationMember struct {
	Account Account
	Role    string
}

type TeamVault struct {
	VaultID string
	Role    string
}

type Team struct {
	ID        string
	Name      string
	MemberIDs []string
	Vaults    []TeamVault
}

type OrganizationInvitation struct {
	ID             string
	OrganizationID string
	Email          string
	Role           string
	InvitedBy      string
	CreatedAt      time.Time
	ExpiresAt      time.Time
}

type ReceivedOrganizationInvitation struct {
	OrganizationInvitation
	OrganizationName string
	InvitedByName    string
	InvitedByEmail   string
}

// Organization is an organization as a member sees it; Invitations is nil unless the member is an admin.
type Organization struct {
	OrganizationSummary
	Members     []OrganizationMember
	Teams       []Team
	Invitations []OrganizationInvitation
}

// OrganizationChange is who a committed change to an organization concerns.
type OrganizationChange struct {
	// AccountIDs are the members before and after the change, and the accounts whose invitation it withdrew.
	AccountIDs []string
	// VaultAccountIDs lost access to one of the organization's vaults or got another role in one, or own a vault whose
	// access or key requests changed.
	VaultAccountIDs []string
}

// VaultKeyRequest is an account the teams entitle to an organization vault that has no wrapped key yet. Recipient has
// only the ID, email, name, role and public keys.
type VaultKeyRequest struct {
	VaultID       string
	Role          string
	KeyGeneration int64
	Recipient     Account
}

type WaitingVaultKey struct {
	VaultID        string
	OrganizationID string
	Role           string
}

type vaultAccountKey struct {
	vaultID   string
	accountID string
}

// isInvitedByAdmin holds for an organization invitation i whose inviter is still an admin of its organization: an
// invitation carries its inviter's authority, so one from an admin who was demoted or removed can no longer be accepted.
const isInvitedByAdmin = `EXISTS (SELECT 1 FROM organization_members inviter_membership
	WHERE inviter_membership.organization_id = i.organization_id AND inviter_membership.account_id = i.invited_by
	AND inviter_membership.role = 'admin')`

func lockOrganization(ctx context.Context, transaction pgx.Tx, organizationID string) error {
	var isLocked bool
	err := transaction.QueryRow(ctx, `SELECT true FROM organizations WHERE id = $1 FOR UPDATE`, organizationID).Scan(&isLocked)
	return convertNoRowsToNotFound(err)
}

// queryOrganizationRole returns "" when the account is not a member.
func queryOrganizationRole(ctx context.Context, database queryer, organizationID, accountID string) (string, error) {
	var role string
	err := database.QueryRow(ctx, `SELECT role FROM organization_members WHERE organization_id = $1 AND account_id = $2`,
		organizationID, accountID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return role, err
}

func requireOrganizationAdmin(ctx context.Context, transaction pgx.Tx, organizationID, accountID string) error {
	role, err := queryOrganizationRole(ctx, transaction, organizationID, accountID)
	switch {
	case err != nil:
		return err
	case role == "":
		return ErrNotFound
	case role != apitypes.OrganizationRoleAdmin:
		return ErrNotOrganizationAdmin
	}
	return nil
}

func requireTeam(ctx context.Context, transaction pgx.Tx, organizationID, teamID string) error {
	var isFound bool
	err := transaction.QueryRow(ctx, `SELECT true FROM teams WHERE id = $1 AND organization_id = $2`, teamID, organizationID).Scan(&isFound)
	return convertNoRowsToNotFound(err)
}

func listOrganizationMemberIDs(ctx context.Context, database queryer, organizationID string) ([]string, error) {
	rows, err := database.Query(ctx, `SELECT account_id FROM organization_members WHERE organization_id = $1`, organizationID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func (s *Store) ListOrganizationMemberIDs(ctx context.Context, organizationID string) ([]string, error) {
	return listOrganizationMemberIDs(ctx, s.pool, organizationID)
}

func mergeAccountIDs(groups ...[]string) []string {
	merged := slices.Concat(groups...)
	slices.Sort(merged)
	return slices.Compact(merged)
}

// changeOrganization runs change in one transaction that first locks the organization, as every change to an
// organization does (§5.11), and ends with reconcileOrganizationVaults when isReconciling. change may add the accounts
// whose invitation it withdrew to the change's AccountIDs. A reconciling change also locks the organization's vaults,
// in ID order, before change: a grant's foreign key would otherwise share-lock one vault ahead of the others and wait
// for them while an item move holds one and waits for it.
func (s *Store) changeOrganization(ctx context.Context, organizationID string, isReconciling bool, change func(pgx.Tx, *OrganizationChange) error) (*OrganizationChange, error) {
	organizationChange := &OrganizationChange{}
	err := s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		if err := lockOrganization(ctx, transaction, organizationID); err != nil {
			return err
		}
		if isReconciling {
			if _, err := transaction.Exec(ctx, `SELECT 1 FROM vaults WHERE organization_id = $1 ORDER BY id FOR UPDATE`, organizationID); err != nil {
				return err
			}
		}
		memberIDsBefore, err := listOrganizationMemberIDs(ctx, transaction, organizationID)
		if err != nil {
			return err
		}
		if err := change(transaction, organizationChange); err != nil {
			return err
		}
		memberIDsAfter, err := listOrganizationMemberIDs(ctx, transaction, organizationID)
		if err != nil {
			return err
		}
		organizationChange.AccountIDs = mergeAccountIDs(organizationChange.AccountIDs, memberIDsBefore, memberIDsAfter)
		if !isReconciling {
			return nil
		}
		changedAccountIDs, affectedVaultIDs, err := reconcileOrganizationVaults(ctx, transaction, organizationID)
		if err != nil {
			return err
		}
		ownerRows, err := transaction.Query(ctx,
			`SELECT account_id FROM vault_access WHERE vault_id = ANY($1::uuid[]) AND role = 'owner'`, affectedVaultIDs)
		if err != nil {
			return err
		}
		ownerIDs, err := pgx.CollectRows(ownerRows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		organizationChange.VaultAccountIDs = mergeAccountIDs(changedAccountIDs, ownerIDs)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return organizationChange, nil
}

// listEntitledRoles returns the strongest role (apitypes.RankVaultRole) the teams give each account in each vault,
// among the team grants condition selects from team_vaults tv joined with team_members tm.
func listEntitledRoles(ctx context.Context, database queryer, condition string, arguments ...any) (map[vaultAccountKey]string, error) {
	rows, err := database.Query(ctx, `
		SELECT DISTINCT tv.vault_id::text, tm.account_id, tv.role
		FROM team_vaults tv JOIN team_members tm ON tm.team_id = tv.team_id
		WHERE `+condition, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rolesByKey := map[vaultAccountKey]string{}
	for rows.Next() {
		var key vaultAccountKey
		var role string
		if err := rows.Scan(&key.vaultID, &key.accountID, &role); err != nil {
			return nil, err
		}
		keepStrongerRole(rolesByKey, key, role)
	}
	return rolesByKey, rows.Err()
}

func keepStrongerRole[K comparable](rolesByKey map[K]string, key K, role string) {
	if apitypes.RankVaultRole(role) > apitypes.RankVaultRole(rolesByKey[key]) {
		rolesByKey[key] = role
	}
}

// queryEntitledRole returns "" when no team gives the account the vault.
func queryEntitledRole(ctx context.Context, transaction pgx.Tx, vaultID, accountID string) (string, error) {
	rolesByKey, err := listEntitledRoles(ctx, transaction, `tv.vault_id = $1 AND tm.account_id = $2`, vaultID, accountID)
	return rolesByKey[vaultAccountKey{vaultID: vaultID, accountID: accountID}], err
}

// reconcileOrganizationVaults makes the access rows of the organization's vaults agree with its teams (§5.11): the
// row of an account no team entitles any longer is deleted and its vault marked for rotation, and every other row
// gets the strongest role of the account's teams. It refuses a vault without an owner row (ErrNoVaultOwner) or with
// more than maxEntitledAccountCount entitled accounts (ErrTooManyVaultMembers). It returns the accounts whose rows
// changed, and the vaults whose rows changed or that have key requests. The caller holds the organization's lock.
func reconcileOrganizationVaults(ctx context.Context, transaction pgx.Tx, organizationID string) (changedAccountIDs, affectedVaultIDs []string, err error) {
	vaultRows, err := transaction.Query(ctx, `SELECT id::text FROM vaults WHERE organization_id = $1 ORDER BY id FOR UPDATE`, organizationID)
	if err != nil {
		return nil, nil, err
	}
	vaultIDs, err := pgx.CollectRows(vaultRows, pgx.RowTo[string])
	if err != nil || len(vaultIDs) == 0 {
		return nil, nil, err
	}
	entitledRolesByKey, err := listEntitledRoles(ctx, transaction, `tv.vault_id = ANY($1::uuid[])`, vaultIDs)
	if err != nil {
		return nil, nil, err
	}
	entitledCountsByVaultID := map[string]int{}
	for key := range entitledRolesByKey {
		entitledCountsByVaultID[key.vaultID]++
		if entitledCountsByVaultID[key.vaultID] > maxEntitledAccountCount {
			return nil, nil, ErrTooManyVaultMembers
		}
	}

	accessRows, err := transaction.Query(ctx, `SELECT vault_id::text, account_id, role FROM vault_access WHERE vault_id = ANY($1::uuid[])`, vaultIDs)
	if err != nil {
		return nil, nil, err
	}
	defer accessRows.Close()
	var removedVaultIDs, removedAccountIDs, updatedVaultIDs, updatedAccountIDs, updatedRoles []string
	hasAccessByKey := map[vaultAccountKey]bool{}
	hasOwnerByVaultID := map[string]bool{}
	isAffectedByVaultID := map[string]bool{}
	for accessRows.Next() {
		var key vaultAccountKey
		var role string
		if err := accessRows.Scan(&key.vaultID, &key.accountID, &role); err != nil {
			return nil, nil, err
		}
		hasAccessByKey[key] = true
		entitledRole, isEntitled := entitledRolesByKey[key]
		switch {
		case !isEntitled:
			removedVaultIDs, removedAccountIDs = append(removedVaultIDs, key.vaultID), append(removedAccountIDs, key.accountID)
		case entitledRole != role:
			updatedVaultIDs, updatedAccountIDs = append(updatedVaultIDs, key.vaultID), append(updatedAccountIDs, key.accountID)
			updatedRoles = append(updatedRoles, entitledRole)
		}
		if !isEntitled || entitledRole != role {
			changedAccountIDs = append(changedAccountIDs, key.accountID)
			isAffectedByVaultID[key.vaultID] = true
		}
		if entitledRole == apitypes.VaultOwner {
			hasOwnerByVaultID[key.vaultID] = true
		}
	}
	accessRows.Close()
	if err := accessRows.Err(); err != nil {
		return nil, nil, err
	}
	for _, vaultID := range vaultIDs {
		if !hasOwnerByVaultID[vaultID] {
			return nil, nil, ErrNoVaultOwner
		}
	}
	for key := range entitledRolesByKey {
		if !hasAccessByKey[key] {
			isAffectedByVaultID[key.vaultID] = true
		}
	}

	if len(removedVaultIDs) > 0 {
		if _, err := transaction.Exec(ctx, `
			DELETE FROM vault_access a USING unnest($1::uuid[], $2::text[]) AS removed (vault_id, account_id)
			WHERE a.vault_id = removed.vault_id AND a.account_id = removed.account_id`, removedVaultIDs, removedAccountIDs); err != nil {
			return nil, nil, err
		}
		if _, err := transaction.Exec(ctx, `UPDATE vaults SET needs_rotation = true WHERE id = ANY($1::uuid[])`, removedVaultIDs); err != nil {
			return nil, nil, err
		}
	}
	if len(updatedVaultIDs) > 0 {
		if _, err := transaction.Exec(ctx, `
			UPDATE vault_access a SET role = updated.role FROM unnest($1::uuid[], $2::text[], $3::text[]) AS updated (vault_id, account_id, role)
			WHERE a.vault_id = updated.vault_id AND a.account_id = updated.account_id`, updatedVaultIDs, updatedAccountIDs, updatedRoles); err != nil {
			return nil, nil, err
		}
	}
	for vaultID := range isAffectedByVaultID {
		affectedVaultIDs = append(affectedVaultIDs, vaultID)
	}
	slices.Sort(affectedVaultIDs)
	return mergeAccountIDs(changedAccountIDs), affectedVaultIDs, nil
}

// CreateOrganization creates an organization with the account as its first admin.
func (s *Store) CreateOrganization(ctx context.Context, organizationID, name, accountID string) error {
	return s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		var creatorID string
		if err := transaction.QueryRow(ctx, `SELECT id FROM accounts WHERE id = $1 FOR SHARE`, accountID).Scan(&creatorID); err != nil {
			return convertNoRowsToNotFound(err)
		}
		if _, err := transaction.Exec(ctx, `INSERT INTO organizations (id, name, created_by) VALUES ($1, $2, $3)`,
			organizationID, name, accountID); err != nil {
			return err
		}
		_, err := transaction.Exec(ctx,
			`INSERT INTO organization_members (organization_id, account_id, role, added_by) VALUES ($1, $2, 'admin', $2)`, organizationID, accountID)
		return err
	})
}

const organizationSummaryQuery = `
	SELECT o.id::text, o.name, m.role, (SELECT count(*) FROM organization_members c WHERE c.organization_id = o.id)
	FROM organization_members m JOIN organizations o ON o.id = m.organization_id`

func scanOrganizationSummary(row pgx.Row) (OrganizationSummary, error) {
	var summary OrganizationSummary
	err := row.Scan(&summary.ID, &summary.Name, &summary.Role, &summary.MemberCount)
	return summary, err
}

func (s *Store) ListOrganizations(ctx context.Context, accountID string) ([]OrganizationSummary, error) {
	rows, err := s.pool.Query(ctx, organizationSummaryQuery+` WHERE m.account_id = $1 ORDER BY lower(o.name), o.id`, accountID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (OrganizationSummary, error) { return scanOrganizationSummary(row) })
}

// GetOrganizationSummary returns ErrNotFound unless the account is a member.
func (s *Store) GetOrganizationSummary(ctx context.Context, organizationID, accountID string) (*OrganizationSummary, error) {
	return getOrganizationSummary(ctx, s.pool, organizationID, accountID)
}

func getOrganizationSummary(ctx context.Context, database queryer, organizationID, accountID string) (*OrganizationSummary, error) {
	summary, err := scanOrganizationSummary(database.QueryRow(ctx,
		organizationSummaryQuery+` WHERE m.organization_id = $1 AND m.account_id = $2`, organizationID, accountID))
	if err != nil {
		return nil, convertNoRowsToNotFound(err)
	}
	return &summary, nil
}

// GetOrganization returns the organization as the account sees it, from one snapshot; ErrNotFound unless it is a member.
func (s *Store) GetOrganization(ctx context.Context, organizationID, accountID string) (*Organization, error) {
	organization := &Organization{Members: []OrganizationMember{}, Teams: []Team{}}
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(transaction pgx.Tx) error {
		summary, err := getOrganizationSummary(ctx, transaction, organizationID, accountID)
		if err != nil {
			return err
		}
		organization.OrganizationSummary = *summary

		memberRows, err := transaction.Query(ctx, `
			SELECT a.id, a.email, a.name, a.role, a.kem_pub, a.sign_pub, a.kem_sig, m.role
			FROM organization_members m JOIN accounts a ON a.id = m.account_id
			WHERE m.organization_id = $1 ORDER BY m.added_at, a.id`, organizationID)
		if err != nil {
			return err
		}
		if organization.Members, err = pgx.CollectRows(memberRows, func(row pgx.CollectableRow) (OrganizationMember, error) {
			var member OrganizationMember
			err := row.Scan(&member.Account.ID, &member.Account.Email, &member.Account.Name, &member.Account.Role,
				&member.Account.KEMPub, &member.Account.SignPub, &member.Account.KEMSig, &member.Role)
			return member, err
		}); err != nil {
			return err
		}

		teamRows, err := transaction.Query(ctx, `SELECT id::text, name FROM teams WHERE organization_id = $1 ORDER BY lower(name), id`, organizationID)
		if err != nil {
			return err
		}
		if organization.Teams, err = pgx.CollectRows(teamRows, func(row pgx.CollectableRow) (Team, error) {
			team := Team{MemberIDs: []string{}, Vaults: []TeamVault{}}
			err := row.Scan(&team.ID, &team.Name)
			return team, err
		}); err != nil {
			return err
		}
		teamIndexesByID := map[string]int{}
		for index, team := range organization.Teams {
			teamIndexesByID[team.ID] = index
		}
		var teamID, memberID, vaultID, role string
		teamMemberRows, err := transaction.Query(ctx,
			`SELECT team_id::text, account_id FROM team_members WHERE organization_id = $1 ORDER BY added_at, account_id`, organizationID)
		if err != nil {
			return err
		}
		if _, err := pgx.ForEachRow(teamMemberRows, []any{&teamID, &memberID}, func() error {
			team := &organization.Teams[teamIndexesByID[teamID]]
			team.MemberIDs = append(team.MemberIDs, memberID)
			return nil
		}); err != nil {
			return err
		}
		teamVaultRows, err := transaction.Query(ctx,
			`SELECT team_id::text, vault_id::text, role FROM team_vaults WHERE organization_id = $1 ORDER BY granted_at, vault_id`, organizationID)
		if err != nil {
			return err
		}
		if _, err := pgx.ForEachRow(teamVaultRows, []any{&teamID, &vaultID, &role}, func() error {
			team := &organization.Teams[teamIndexesByID[teamID]]
			team.Vaults = append(team.Vaults, TeamVault{VaultID: vaultID, Role: role})
			return nil
		}); err != nil {
			return err
		}

		if organization.Role != apitypes.OrganizationRoleAdmin {
			return nil
		}
		invitationRows, err := transaction.Query(ctx, `
			SELECT i.id::text, i.organization_id::text, i.email, i.role, i.invited_by, i.created_at, i.expires_at
			FROM organization_invitations i WHERE i.organization_id = $1 AND i.expires_at > now() AND `+isInvitedByAdmin+` ORDER BY i.created_at`, organizationID)
		if err != nil {
			return err
		}
		organization.Invitations, err = pgx.CollectRows(invitationRows, scanOrganizationInvitation)
		return err
	})
	if err != nil {
		return nil, err
	}
	return organization, nil
}

func scanOrganizationInvitation(row pgx.CollectableRow) (OrganizationInvitation, error) {
	var invitation OrganizationInvitation
	err := row.Scan(&invitation.ID, &invitation.OrganizationID, &invitation.Email, &invitation.Role, &invitation.InvitedBy,
		&invitation.CreatedAt, &invitation.ExpiresAt)
	return invitation, err
}

func (s *Store) RenameOrganization(ctx context.Context, organizationID, accountID, name string) (*OrganizationChange, error) {
	return s.changeOrganization(ctx, organizationID, false, func(transaction pgx.Tx, _ *OrganizationChange) error {
		if err := requireOrganizationAdmin(ctx, transaction, organizationID, accountID); err != nil {
			return err
		}
		_, err := transaction.Exec(ctx, `UPDATE organizations SET name = $2 WHERE id = $1`, organizationID, name)
		return err
	})
}

// DeleteOrganization deletes an organization without vaults (ErrOrganizationHasVaults), with its members, teams and
// invitations.
func (s *Store) DeleteOrganization(ctx context.Context, organizationID, accountID string) (*OrganizationChange, error) {
	return s.changeOrganization(ctx, organizationID, false, func(transaction pgx.Tx, change *OrganizationChange) error {
		if err := requireOrganizationAdmin(ctx, transaction, organizationID, accountID); err != nil {
			return err
		}
		var hasVaults bool
		if err := transaction.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM vaults WHERE organization_id = $1)`, organizationID).Scan(&hasVaults); err != nil {
			return err
		}
		if hasVaults {
			return ErrOrganizationHasVaults
		}
		invitedRows, err := transaction.Query(ctx, `
			SELECT a.id FROM organization_invitations i JOIN accounts a ON a.email = i.email WHERE i.organization_id = $1`, organizationID)
		if err != nil {
			return err
		}
		invitedAccountIDs, err := pgx.CollectRows(invitedRows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		change.AccountIDs = append(change.AccountIDs, invitedAccountIDs...)
		_, err = transaction.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, organizationID)
		return err
	})
}

// UpsertOrganizationInvitation stores an invitation, or renews the one to the same organization and email, and returns
// its ID. InvitedBy must be an admin; the email of a member is ErrExists.
func (s *Store) UpsertOrganizationInvitation(ctx context.Context, invitation *OrganizationInvitation, ttl time.Duration) (string, *OrganizationChange, error) {
	var invitationID string
	change, err := s.changeOrganization(ctx, invitation.OrganizationID, false, func(transaction pgx.Tx, _ *OrganizationChange) error {
		if err := requireOrganizationAdmin(ctx, transaction, invitation.OrganizationID, invitation.InvitedBy); err != nil {
			return err
		}
		isMember, err := isOrganizationMemberEmail(ctx, transaction, invitation.OrganizationID, invitation.Email)
		if err != nil {
			return err
		}
		if isMember {
			return ErrExists
		}
		return transaction.QueryRow(ctx, `
			INSERT INTO organization_invitations (id, organization_id, email, role, invited_by, expires_at) VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (organization_id, email) DO UPDATE SET role = $4, invited_by = $5, expires_at = $6, created_at = now()
			RETURNING id::text`,
			uuid.NewString(), invitation.OrganizationID, invitation.Email, invitation.Role, invitation.InvitedBy, time.Now().Add(ttl)).Scan(&invitationID)
	})
	return invitationID, change, err
}

const isPendingOrganizationInvitation = `i.id = $1 AND i.email = $2 AND i.invited_by = $3 AND i.role = $4
	AND i.expires_at > now() AND ` + isInvitedByAdmin

func (s *Store) CheckPendingOrganizationInvitation(ctx context.Context, invitation *OrganizationInvitation) error {
	var exists int
	err := s.pool.QueryRow(ctx, `SELECT 1 FROM organization_invitations i WHERE `+isPendingOrganizationInvitation,
		invitation.ID, invitation.Email, invitation.InvitedBy, invitation.Role).Scan(&exists)
	return convertNoRowsToNotFound(err)
}

// CreateOrganizationSignUpInvite mints a member sign-up token only while its source invitation is still pending.
func (s *Store) CreateOrganizationSignUpInvite(ctx context.Context, invitation *OrganizationInvitation, tokenHash []byte, expiresAt time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO invites (token_hash, email, role, created_by, expires_at)
		SELECT $5, i.email, 'member', i.invited_by, $6 FROM organization_invitations i WHERE `+isPendingOrganizationInvitation,
		invitation.ID, invitation.Email, invitation.InvitedBy, invitation.Role, tokenHash, expiresAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func isOrganizationMemberEmail(ctx context.Context, database queryer, organizationID, email string) (bool, error) {
	var isMember bool
	err := database.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM organization_members m JOIN accounts a ON a.id = m.account_id WHERE m.organization_id = $1 AND a.email = $2)`,
		organizationID, email).Scan(&isMember)
	return isMember, err
}

func (s *Store) IsOrganizationMemberEmail(ctx context.Context, organizationID, email string) (bool, error) {
	return isOrganizationMemberEmail(ctx, s.pool, organizationID, email)
}

// ListReceivedOrganizationInvitations returns the unexpired invitations to email.
func (s *Store) ListReceivedOrganizationInvitations(ctx context.Context, email string) ([]ReceivedOrganizationInvitation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT i.id::text, i.organization_id::text, i.email, i.role, i.invited_by, i.created_at, i.expires_at, o.name, a.name, a.email
		FROM organization_invitations i JOIN organizations o ON o.id = i.organization_id JOIN accounts a ON a.id = i.invited_by
		WHERE i.email = $1 AND i.expires_at > now() AND `+isInvitedByAdmin+`
		ORDER BY i.created_at`, email)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ReceivedOrganizationInvitation, error) {
		var invitation ReceivedOrganizationInvitation
		err := row.Scan(&invitation.ID, &invitation.OrganizationID, &invitation.Email, &invitation.Role, &invitation.InvitedBy,
			&invitation.CreatedAt, &invitation.ExpiresAt, &invitation.OrganizationName, &invitation.InvitedByName, &invitation.InvitedByEmail)
		return invitation, err
	})
}

// answerOrganizationInvitation deletes an unexpired invitation to email whose inviter is still an admin, under its
// organization's lock, adding the account as a member when it accepts, and returns the organization.
func (s *Store) answerOrganizationInvitation(ctx context.Context, invitationID, accountID, email string, isAccepted bool) (string, *OrganizationChange, error) {
	var organizationID string
	if err := s.pool.QueryRow(ctx, `SELECT organization_id::text FROM organization_invitations WHERE id = $1 AND email = $2 AND expires_at > now()`,
		invitationID, email).Scan(&organizationID); err != nil {
		return "", nil, convertNoRowsToNotFound(err)
	}
	change, err := s.changeOrganization(ctx, organizationID, false, func(transaction pgx.Tx, change *OrganizationChange) error {
		if isAccepted {
			var memberID string
			if err := transaction.QueryRow(ctx, `SELECT id FROM accounts WHERE id = $1 FOR SHARE`, accountID).Scan(&memberID); err != nil {
				return convertNoRowsToNotFound(err)
			}
		}
		var role, invitedBy string
		if err := transaction.QueryRow(ctx, `
			DELETE FROM organization_invitations i WHERE i.id = $1 AND i.email = $2 AND i.expires_at > now() AND `+isInvitedByAdmin+`
			RETURNING i.role, i.invited_by`,
			invitationID, email).Scan(&role, &invitedBy); err != nil {
			return convertNoRowsToNotFound(err)
		}
		change.AccountIDs = append(change.AccountIDs, accountID)
		if !isAccepted {
			return nil
		}
		_, err := transaction.Exec(ctx, `
			INSERT INTO organization_members (organization_id, account_id, role, added_by) VALUES ($1, $2, $3, $4)
			ON CONFLICT (organization_id, account_id) DO NOTHING`, organizationID, accountID, role, invitedBy)
		return err
	})
	return organizationID, change, err
}

// AcceptOrganizationInvitation makes the account a member with the invitation's role and deletes the invitation.
func (s *Store) AcceptOrganizationInvitation(ctx context.Context, invitationID, accountID, email string) (string, *OrganizationChange, error) {
	return s.answerOrganizationInvitation(ctx, invitationID, accountID, email, true)
}

func (s *Store) DeclineOrganizationInvitation(ctx context.Context, invitationID, accountID, email string) (string, *OrganizationChange, error) {
	return s.answerOrganizationInvitation(ctx, invitationID, accountID, email, false)
}

// CancelOrganizationInvitation deletes an invitation to the organization; accountID must be an admin.
func (s *Store) CancelOrganizationInvitation(ctx context.Context, organizationID, invitationID, accountID string) (*OrganizationChange, error) {
	return s.changeOrganization(ctx, organizationID, false, func(transaction pgx.Tx, change *OrganizationChange) error {
		if err := requireOrganizationAdmin(ctx, transaction, organizationID, accountID); err != nil {
			return err
		}
		var email string
		if err := transaction.QueryRow(ctx, `DELETE FROM organization_invitations WHERE id = $1 AND organization_id = $2 RETURNING email`,
			invitationID, organizationID).Scan(&email); err != nil {
			return convertNoRowsToNotFound(err)
		}
		recipientRows, err := transaction.Query(ctx, `SELECT id FROM accounts WHERE email = $1`, email)
		if err != nil {
			return err
		}
		recipientIDs, err := pgx.CollectRows(recipientRows, pgx.RowTo[string])
		change.AccountIDs = append(change.AccountIDs, recipientIDs...)
		return err
	})
}

func countOrganizationMembers(ctx context.Context, transaction pgx.Tx, organizationID string) (memberCount, adminCount int, err error) {
	err = transaction.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE role = 'admin') FROM organization_members WHERE organization_id = $1`,
		organizationID).Scan(&memberCount, &adminCount)
	return memberCount, adminCount, err
}

// SetOrganizationRole changes a member's role; accountID must be an admin, and the last admin stays one (ErrLastAdmin).
func (s *Store) SetOrganizationRole(ctx context.Context, organizationID, accountID, memberID, role string) (*OrganizationChange, error) {
	return s.changeOrganization(ctx, organizationID, false, func(transaction pgx.Tx, _ *OrganizationChange) error {
		if err := requireOrganizationAdmin(ctx, transaction, organizationID, accountID); err != nil {
			return err
		}
		currentRole, err := queryOrganizationRole(ctx, transaction, organizationID, memberID)
		if err != nil {
			return err
		}
		if currentRole == "" {
			return ErrNotFound
		}
		if currentRole == apitypes.OrganizationRoleAdmin && role != apitypes.OrganizationRoleAdmin {
			_, adminCount, err := countOrganizationMembers(ctx, transaction, organizationID)
			if err != nil {
				return err
			}
			if adminCount <= 1 {
				return ErrLastAdmin
			}
		}
		_, err = transaction.Exec(ctx, `UPDATE organization_members SET role = $3 WHERE organization_id = $1 AND account_id = $2`,
			organizationID, memberID, role)
		return err
	})
}

// RemoveOrganizationMember removes a member from the organization and its teams: admins remove anyone, and anyone
// removes themselves, except the last member (ErrLastMember) and the last admin while others remain (ErrLastAdmin).
func (s *Store) RemoveOrganizationMember(ctx context.Context, organizationID, accountID, memberID string) (*OrganizationChange, error) {
	return s.changeOrganization(ctx, organizationID, true, func(transaction pgx.Tx, _ *OrganizationChange) error {
		role, err := queryOrganizationRole(ctx, transaction, organizationID, accountID)
		if err != nil {
			return err
		}
		switch {
		case role == "":
			return ErrNotFound
		case memberID != accountID && role != apitypes.OrganizationRoleAdmin:
			return ErrNotOrganizationAdmin
		}
		memberRole, err := queryOrganizationRole(ctx, transaction, organizationID, memberID)
		if err != nil {
			return err
		}
		if memberRole == "" {
			return ErrNotFound
		}
		memberCount, adminCount, err := countOrganizationMembers(ctx, transaction, organizationID)
		switch {
		case err != nil:
			return err
		case memberCount <= 1:
			return ErrLastMember
		case memberRole == apitypes.OrganizationRoleAdmin && adminCount <= 1:
			return ErrLastAdmin
		}
		_, err = transaction.Exec(ctx, `DELETE FROM organization_members WHERE organization_id = $1 AND account_id = $2`, organizationID, memberID)
		return err
	})
}

// CreateTeam creates a team; a name another team of the organization has, regardless of case, is ErrExists.
func (s *Store) CreateTeam(ctx context.Context, organizationID, accountID, teamID, name string) (*OrganizationChange, error) {
	return s.changeOrganization(ctx, organizationID, false, func(transaction pgx.Tx, _ *OrganizationChange) error {
		if err := requireOrganizationAdmin(ctx, transaction, organizationID, accountID); err != nil {
			return err
		}
		_, err := transaction.Exec(ctx, `INSERT INTO teams (id, organization_id, name, created_by) VALUES ($1, $2, $3, $4)`,
			teamID, organizationID, name, accountID)
		if isUniqueViolation(err) {
			return ErrExists
		}
		return err
	})
}

func (s *Store) RenameTeam(ctx context.Context, organizationID, accountID, teamID, name string) (*OrganizationChange, error) {
	return s.changeOrganization(ctx, organizationID, false, func(transaction pgx.Tx, _ *OrganizationChange) error {
		if err := requireOrganizationAdmin(ctx, transaction, organizationID, accountID); err != nil {
			return err
		}
		commandTag, err := transaction.Exec(ctx, `UPDATE teams SET name = $3 WHERE id = $1 AND organization_id = $2`, teamID, organizationID, name)
		switch {
		case isUniqueViolation(err):
			return ErrExists
		case err != nil:
			return err
		case commandTag.RowsAffected() == 0:
			return ErrNotFound
		}
		return nil
	})
}

// deleteFromOrganization runs a delete as an admin of the organization; nothing deleted is ErrNotFound.
func (s *Store) deleteFromOrganization(ctx context.Context, organizationID, accountID, query string, arguments ...any) (*OrganizationChange, error) {
	return s.changeOrganization(ctx, organizationID, true, func(transaction pgx.Tx, _ *OrganizationChange) error {
		if err := requireOrganizationAdmin(ctx, transaction, organizationID, accountID); err != nil {
			return err
		}
		commandTag, err := transaction.Exec(ctx, query, arguments...)
		if err != nil {
			return err
		}
		if commandTag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}

func (s *Store) DeleteTeam(ctx context.Context, organizationID, accountID, teamID string) (*OrganizationChange, error) {
	return s.deleteFromOrganization(ctx, organizationID, accountID,
		`DELETE FROM teams WHERE id = $1 AND organization_id = $2`, teamID, organizationID)
}

// AddTeamMember adds a member of the organization (ErrNotFound otherwise) to one of its teams.
func (s *Store) AddTeamMember(ctx context.Context, organizationID, accountID, teamID, memberID string) (*OrganizationChange, error) {
	return s.changeOrganization(ctx, organizationID, true, func(transaction pgx.Tx, _ *OrganizationChange) error {
		if err := requireOrganizationAdmin(ctx, transaction, organizationID, accountID); err != nil {
			return err
		}
		if err := requireTeam(ctx, transaction, organizationID, teamID); err != nil {
			return err
		}
		memberRole, err := queryOrganizationRole(ctx, transaction, organizationID, memberID)
		if err != nil {
			return err
		}
		if memberRole == "" {
			return ErrNotFound
		}
		_, err = transaction.Exec(ctx, `
			INSERT INTO team_members (team_id, organization_id, account_id, added_by) VALUES ($1, $2, $3, $4)
			ON CONFLICT (team_id, account_id) DO NOTHING`, teamID, organizationID, memberID, accountID)
		return err
	})
}

func (s *Store) RemoveTeamMember(ctx context.Context, organizationID, accountID, teamID, memberID string) (*OrganizationChange, error) {
	return s.deleteFromOrganization(ctx, organizationID, accountID,
		`DELETE FROM team_members WHERE team_id = $1 AND organization_id = $2 AND account_id = $3`, teamID, organizationID, memberID)
}

// GrantTeamVault gives one of the organization's vaults to one of its teams with role, or changes the role.
func (s *Store) GrantTeamVault(ctx context.Context, organizationID, accountID, teamID, vaultID, role string) (*OrganizationChange, error) {
	return s.changeOrganization(ctx, organizationID, true, func(transaction pgx.Tx, _ *OrganizationChange) error {
		if err := requireOrganizationAdmin(ctx, transaction, organizationID, accountID); err != nil {
			return err
		}
		if err := requireTeam(ctx, transaction, organizationID, teamID); err != nil {
			return err
		}
		var isFound bool
		if err := transaction.QueryRow(ctx, `SELECT true FROM vaults WHERE id = $1 AND organization_id = $2`, vaultID, organizationID).Scan(&isFound); err != nil {
			return convertNoRowsToNotFound(err)
		}
		_, err := transaction.Exec(ctx, `
			INSERT INTO team_vaults (team_id, vault_id, organization_id, role, granted_by) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (team_id, vault_id) DO UPDATE SET role = $4, granted_by = $5, granted_at = now()`,
			teamID, vaultID, organizationID, role, accountID)
		return err
	})
}

func (s *Store) RevokeTeamVault(ctx context.Context, organizationID, accountID, teamID, vaultID string) (*OrganizationChange, error) {
	return s.deleteFromOrganization(ctx, organizationID, accountID,
		`DELETE FROM team_vaults WHERE team_id = $1 AND vault_id = $2 AND organization_id = $3`, teamID, vaultID, organizationID)
}

// CreateOrganizationVault creates a vault of vault.OrganizationID for one of its teams, which gets it as owner, with an
// owner row for the creator. The creator must be in the team (ErrNotTeamMember); an organization the creator is not
// a member of, or a team of another organization, is ErrNotFound.
func (s *Store) CreateOrganizationVault(ctx context.Context, vault *Vault, ownerWrappedKey []byte, teamID string) (*OrganizationChange, error) {
	return s.changeOrganization(ctx, vault.OrganizationID, true, func(transaction pgx.Tx, _ *OrganizationChange) error {
		role, err := queryOrganizationRole(ctx, transaction, vault.OrganizationID, vault.CreatedBy)
		if err != nil {
			return err
		}
		if role == "" {
			return ErrNotFound
		}
		if err := requireTeam(ctx, transaction, vault.OrganizationID, teamID); err != nil {
			return err
		}
		var isTeamMember bool
		if err := transaction.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM team_members WHERE team_id = $1 AND account_id = $2)`,
			teamID, vault.CreatedBy).Scan(&isTeamMember); err != nil {
			return err
		}
		if !isTeamMember {
			return ErrNotTeamMember
		}
		if err := insertVault(ctx, transaction, vault, ownerWrappedKey); err != nil {
			return err
		}
		_, err = transaction.Exec(ctx, `
			INSERT INTO team_vaults (team_id, vault_id, organization_id, role, granted_by) VALUES ($1, $2, $3, 'owner', $4)`,
			teamID, vault.ID, vault.OrganizationID, vault.CreatedBy)
		return err
	})
}

// ListVaultKeyRequests returns the key requests of the organization vaults the account owns.
func (s *Store) ListVaultKeyRequests(ctx context.Context, accountID string) ([]VaultKeyRequest, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT grants.vault_id::text, grants.role, v.key_generation, a.id, a.email, a.name, a.role, a.kem_pub, a.sign_pub, a.kem_sig
		FROM (
			SELECT DISTINCT tv.vault_id, tm.account_id, tv.role
			FROM vault_access mine
			JOIN team_vaults tv ON tv.vault_id = mine.vault_id
			JOIN team_members tm ON tm.team_id = tv.team_id
			WHERE mine.account_id = $1 AND mine.role = 'owner'
				AND NOT EXISTS (SELECT 1 FROM vault_access other WHERE other.vault_id = tv.vault_id AND other.account_id = tm.account_id)
		) grants
		JOIN vaults v ON v.id = grants.vault_id
		JOIN accounts a ON a.id = grants.account_id`, accountID)
	if err != nil {
		return nil, err
	}
	grantRows, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (VaultKeyRequest, error) {
		var request VaultKeyRequest
		err := row.Scan(&request.VaultID, &request.Role, &request.KeyGeneration, &request.Recipient.ID, &request.Recipient.Email,
			&request.Recipient.Name, &request.Recipient.Role, &request.Recipient.KEMPub, &request.Recipient.SignPub, &request.Recipient.KEMSig)
		return request, err
	})
	if err != nil {
		return nil, err
	}
	requestIndexesByKey := map[vaultAccountKey]int{}
	requests := []VaultKeyRequest{}
	for _, grant := range grantRows {
		key := vaultAccountKey{vaultID: grant.VaultID, accountID: grant.Recipient.ID}
		index, isFound := requestIndexesByKey[key]
		if !isFound {
			requestIndexesByKey[key] = len(requests)
			requests = append(requests, grant)
			continue
		}
		if apitypes.RankVaultRole(grant.Role) > apitypes.RankVaultRole(requests[index].Role) {
			requests[index].Role = grant.Role
		}
	}
	slices.SortFunc(requests, func(first, second VaultKeyRequest) int {
		if order := strings.Compare(first.VaultID, second.VaultID); order != 0 {
			return order
		}
		return strings.Compare(first.Recipient.Email, second.Recipient.Email)
	})
	return requests, nil
}

// ListWaitingVaultKeys returns the organization vaults the account's teams entitle it to and it has no key for.
func (s *Store) ListWaitingVaultKeys(ctx context.Context, accountID string) ([]WaitingVaultKey, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT tv.vault_id::text, tv.organization_id::text, tv.role
		FROM team_members tm JOIN team_vaults tv ON tv.team_id = tm.team_id
		WHERE tm.account_id = $1
			AND NOT EXISTS (SELECT 1 FROM vault_access a WHERE a.vault_id = tv.vault_id AND a.account_id = $1)`, accountID)
	if err != nil {
		return nil, err
	}
	organizationIDsByVaultID := map[string]string{}
	rolesByVaultID := map[string]string{}
	var vaultID, organizationID, role string
	if _, err := pgx.ForEachRow(rows, []any{&vaultID, &organizationID, &role}, func() error {
		organizationIDsByVaultID[vaultID] = organizationID
		keepStrongerRole(rolesByVaultID, vaultID, role)
		return nil
	}); err != nil {
		return nil, err
	}
	waiting := make([]WaitingVaultKey, 0, len(rolesByVaultID))
	for vaultID, role := range rolesByVaultID {
		waiting = append(waiting, WaitingVaultKey{VaultID: vaultID, OrganizationID: organizationIDsByVaultID[vaultID], Role: role})
	}
	slices.SortFunc(waiting, func(first, second WaitingVaultKey) int { return strings.Compare(first.VaultID, second.VaultID) })
	return waiting, nil
}
