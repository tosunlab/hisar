package store

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrDeletionBusy refuses deletion when concurrent memberships prevent a stable locked set. The caller can retry.
var ErrDeletionBusy = errors.New("store: account memberships changed during deletion")

type CredentialUpdate struct {
	AccountID string
	TokenHash []byte
	// ProvenRecordHash is the SHA-256 of the OPAQUE record the re-authentication was checked against.
	ProvenRecordHash []byte
	Argon2Params     json.RawMessage
	Salt             []byte
	OpaqueRecord     []byte
	EncKeyset        []byte
}

type DeletionOwnedVault struct {
	ID               string
	Type             string
	OtherMemberCount int
}

type DeletionMemberVault struct {
	ID   string
	Type string
	Role string
}

// DeletionOrganization is an organization the account is a member of. IsDeleted: the account is its last member.
// NewAdminName names the member who becomes admin because the account is its last admin.
type DeletionOrganization struct {
	ID           string
	Name         string
	IsDeleted    bool
	NewAdminName string
}

type DeletionPreview struct {
	OwnedVaults             []DeletionOwnedVault
	MemberVaults            []DeletionMemberVault
	Organizations           []DeletionOrganization
	ShareLinkCount          int
	SentInvitationCount     int
	ReceivedInvitationCount int
	DeviceCount             int
}

type DeletedAccount struct {
	Email string
	Name  string
	// VaultMemberAccountIDs shared a vault with the deleted account: their vault list or member list changed.
	VaultMemberAccountIDs []string
	// InvitationAccountIDs had a share invitation from or to the deleted account.
	InvitationAccountIDs []string
	// OrganizationAccountIDs remain in an organization with the deleted account, had an organization invitation from it,
	// or are in an organization that had an invitation from or to it.
	OrganizationAccountIDs []string
	DeletedVaultCount      int
}

func HashOpaqueRecord(opaqueRecord []byte) []byte {
	hash := sha256.Sum256(opaqueRecord)
	return hash[:]
}

// lockProvenAccount fails with ErrConflict when the OPAQUE record changed after the proof, and ErrNotFound when the
// session ended. Devices are locked before sessions, matching revokeDevice. Locking every device also covers changes
// that sign out the account's other devices.
func lockProvenAccount(ctx context.Context, transaction pgx.Tx, accountID string, tokenHash, provenRecordHash []byte) (email, name, deviceID string, err error) {
	var opaqueRecord []byte
	err = transaction.QueryRow(ctx, `SELECT email, name, opaque_record FROM accounts WHERE id = $1 FOR NO KEY UPDATE`, accountID).Scan(&email, &name, &opaqueRecord)
	if err != nil {
		return "", "", "", convertNoRowsToNotFound(err)
	}
	if subtle.ConstantTimeCompare(HashOpaqueRecord(opaqueRecord), provenRecordHash) != 1 {
		return "", "", "", ErrConflict
	}
	if _, err := transaction.Exec(ctx, `SELECT 1 FROM devices WHERE account_id = $1 ORDER BY id FOR UPDATE`, accountID); err != nil {
		return "", "", "", err
	}
	err = transaction.QueryRow(ctx, `
		SELECT s.device_id FROM sessions s JOIN devices d ON d.id = s.device_id
		WHERE s.token_hash = $1 AND s.account_id = $2 AND s.expires_at > now() AND d.revoked_at IS NULL
		FOR SHARE OF s`,
		tokenHash, accountID).Scan(&deviceID)
	if err != nil {
		return "", "", "", convertNoRowsToNotFound(err)
	}
	return email, name, deviceID, nil
}

// UpdateAccountCredentials swaps the credentials and signs out every other device in one transaction.
func (s *Store) UpdateAccountCredentials(ctx context.Context, update *CredentialUpdate) (signedOutDeviceCount int, err error) {
	err = s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		_, _, deviceID, err := lockProvenAccount(ctx, transaction, update.AccountID, update.TokenHash, update.ProvenRecordHash)
		if err != nil {
			return err
		}
		if _, err := transaction.Exec(ctx,
			`UPDATE accounts SET argon2_params = $2, salt = $3, opaque_record = $4, enc_keyset = $5 WHERE id = $1`,
			update.AccountID, update.Argon2Params, update.Salt, update.OpaqueRecord, update.EncKeyset); err != nil {
			return err
		}
		commandTag, err := transaction.Exec(ctx,
			`UPDATE devices SET revoked_at = now() WHERE account_id = $1 AND id <> $2 AND revoked_at IS NULL`, update.AccountID, deviceID)
		if err != nil {
			return err
		}
		signedOutDeviceCount = int(commandTag.RowsAffected())
		_, err = transaction.Exec(ctx, `DELETE FROM sessions WHERE account_id = $1 AND device_id <> $2`, update.AccountID, deviceID)
		return err
	})
	return signedOutDeviceCount, err
}

type accountVaultAccess struct {
	vaultID          string
	vaultType        string
	role             string
	hasOtherOwner    bool
	otherMemberCount int
}

func listAccountVaultAccesses(ctx context.Context, database queryer, accountID string) ([]accountVaultAccess, error) {
	rows, err := database.Query(ctx, `
		SELECT v.id, v.type, a.role,
			EXISTS (SELECT 1 FROM vault_access o WHERE o.vault_id = v.id AND o.role = 'owner' AND o.account_id <> $1),
			(SELECT count(*) FROM vault_access m WHERE m.vault_id = v.id AND m.account_id <> $1)
		FROM vault_access a JOIN vaults v ON v.id = a.vault_id
		WHERE a.account_id = $1 ORDER BY v.created_at`, accountID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (accountVaultAccess, error) {
		var access accountVaultAccess
		err := row.Scan(&access.vaultID, &access.vaultType, &access.role, &access.hasOtherOwner, &access.otherMemberCount)
		return access, err
	})
}

func (access accountVaultAccess) isDeletedWithAccount() bool {
	return access.role == "owner" && !access.hasOtherOwner
}

// accountMembership is an organization the account is a member of, as its deletion changes it (§5.11).
type accountMembership struct {
	organizationID   string
	name             string
	role             string
	otherMemberCount int
	otherAdminCount  int
	// successorID is the remaining member who joined earliest, "" when there is none.
	successorID   string
	successorName string
}

func listAccountMemberships(ctx context.Context, database queryer, accountID string) ([]accountMembership, error) {
	rows, err := database.Query(ctx, `
		SELECT o.id::text, o.name, m.role,
			(SELECT count(*) FROM organization_members x WHERE x.organization_id = o.id AND x.account_id <> $1),
			(SELECT count(*) FROM organization_members x WHERE x.organization_id = o.id AND x.account_id <> $1 AND x.role = 'admin'),
			coalesce(successor.account_id, ''), coalesce(successor.name, '')
		FROM organization_members m JOIN organizations o ON o.id = m.organization_id
		LEFT JOIN LATERAL (
			SELECT x.account_id, a.name FROM organization_members x JOIN accounts a ON a.id = x.account_id
			WHERE x.organization_id = o.id AND x.account_id <> $1 ORDER BY x.added_at, x.account_id LIMIT 1) successor ON true
		WHERE m.account_id = $1 ORDER BY o.id`, accountID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (accountMembership, error) {
		var membership accountMembership
		err := row.Scan(&membership.organizationID, &membership.name, &membership.role, &membership.otherMemberCount,
			&membership.otherAdminCount, &membership.successorID, &membership.successorName)
		return membership, err
	})
}

func (membership accountMembership) isDeletedWithAccount() bool {
	return membership.otherMemberCount == 0
}

func (membership accountMembership) needsNewAdmin() bool {
	return membership.role == "admin" && membership.otherAdminCount == 0 && membership.otherMemberCount > 0
}

func (s *Store) PreviewAccountDeletion(ctx context.Context, accountID string) (*DeletionPreview, error) {
	account, err := s.GetAccountByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	accesses, err := listAccountVaultAccesses(ctx, s.pool, accountID)
	if err != nil {
		return nil, err
	}
	memberships, err := listAccountMemberships(ctx, s.pool, accountID)
	if err != nil {
		return nil, err
	}
	preview := &DeletionPreview{OwnedVaults: []DeletionOwnedVault{}, MemberVaults: []DeletionMemberVault{}, Organizations: []DeletionOrganization{}}
	for _, access := range accesses {
		if access.isDeletedWithAccount() {
			preview.OwnedVaults = append(preview.OwnedVaults, DeletionOwnedVault{ID: access.vaultID, Type: access.vaultType, OtherMemberCount: access.otherMemberCount})
		} else {
			preview.MemberVaults = append(preview.MemberVaults, DeletionMemberVault{ID: access.vaultID, Type: access.vaultType, Role: access.role})
		}
	}
	for _, membership := range memberships {
		organization := DeletionOrganization{ID: membership.organizationID, Name: membership.name, IsDeleted: membership.isDeletedWithAccount()}
		if membership.needsNewAdmin() {
			organization.NewAdminName = membership.successorName
		}
		preview.Organizations = append(preview.Organizations, organization)
	}
	err = s.pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM share_links WHERE account_id = $1 AND expires_at > now()),
			(SELECT count(*) FROM share_invitations WHERE invited_by = $1)
				+ (SELECT count(*) FROM organization_invitations WHERE invited_by = $1)
				+ (SELECT count(*) FROM invites i WHERE i.created_by = $1 AND i.used_at IS NULL AND i.expires_at > now()
					AND NOT EXISTS (SELECT 1 FROM share_invitations s WHERE s.invited_by = $1 AND s.email = i.email)
					AND NOT EXISTS (SELECT 1 FROM organization_invitations o WHERE o.invited_by = $1 AND o.email = i.email)),
			(SELECT count(*) FROM share_invitations WHERE (email = $2 OR accepted_by = $1) AND invited_by <> $1)
				+ (SELECT count(*) FROM organization_invitations WHERE email = $2),
			(SELECT count(*) FROM devices WHERE account_id = $1 AND revoked_at IS NULL)`,
		accountID, account.Email).Scan(&preview.ShareLinkCount, &preview.SentInvitationCount, &preview.ReceivedInvitationCount, &preview.DeviceCount)
	return preview, err
}

func collectAccountIDs(ctx context.Context, transaction pgx.Tx, query string, arguments ...any) ([]string, error) {
	rows, err := transaction.Query(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// DeleteAccount deletes the account and everything that is only its own in one transaction, after checking that the
// session is live and the OPAQUE record is the one the proof was checked against.
func (s *Store) DeleteAccount(ctx context.Context, accountID string, tokenHash, provenRecordHash []byte) (*DeletedAccount, error) {
	return s.deleteAccount(ctx, accountID, func(transaction pgx.Tx) (email, name string, err error) {
		email, name, _, err = lockProvenAccount(ctx, transaction, accountID, tokenHash, provenRecordHash)
		return email, name, err
	})
}

// deleteAccount runs the deletion steps after lockAccount locked the account row and returned its email and name.
// Vaults with another owner stay with them; only this account's access is removed, and an organization vault it held a
// key to is marked for rotation, as after a team removal (§5.11). In its organizations the remaining member who joined
// earliest becomes admin after their last admin, and an organization left without members is deleted.
func (s *Store) deleteAccount(ctx context.Context, accountID string, lockAccount func(transaction pgx.Tx) (email, name string, err error)) (*DeletedAccount, error) {
	for range 3 {
		restart := false
		deleted := &DeletedAccount{}
		err := s.runInTransaction(ctx, func(transaction pgx.Tx) error {
			// Organizations are locked before the account, in ID order, like every other change to them (§5.11).
			if _, err := transaction.Exec(ctx, `
			SELECT 1 FROM organizations o JOIN organization_members m ON m.organization_id = o.id
			WHERE m.account_id = $1 ORDER BY o.id FOR UPDATE OF o`, accountID); err != nil {
				return err
			}
			email, name, err := lockAccount(transaction)
			if err != nil {
				return err
			}
			deleted.Email, deleted.Name = email, name
			// Membership creation can commit while the first organization snapshot waits. The account lock now stops it.
			// Never wait for an omitted organization: an acceptance could hold it while waiting for this account.
			organizationRows, err := transaction.Query(ctx, `
			SELECT o.id::text FROM organizations o JOIN organization_members m ON m.organization_id = o.id
			WHERE m.account_id = $1 ORDER BY o.id FOR UPDATE OF o NOWAIT`, accountID)
			if err != nil {
				return err
			}
			organizationIDs, err := pgx.CollectRows(organizationRows, pgx.RowTo[string])
			if err != nil {
				return err
			}
			// An admin's proof locks their devices, not the target's. Take these before deleting the target's sessions.
			if _, err := transaction.Exec(ctx, `SELECT 1 FROM devices WHERE account_id = $1 ORDER BY id FOR UPDATE`, accountID); err != nil {
				return err
			}
			// Keep account key-share locks available until the sorted vault locks are held, so grants can finish.
			vaultRows, err := transaction.Query(ctx, `
			SELECT id::text FROM vaults WHERE id IN (SELECT vault_id FROM vault_access WHERE account_id = $1)
				OR organization_id IN (SELECT organization_id FROM organization_members WHERE account_id = $1)
			ORDER BY id FOR UPDATE`, accountID)
			if err != nil {
				return err
			}
			vaultIDs, err := pgx.CollectRows(vaultRows, pgx.RowTo[string])
			if err != nil {
				return err
			}
			// Grants consume invitations before taking an account key-share lock. Let them finish before upgrading.
			for _, query := range []string{
				`SELECT 1 FROM organization_invitations WHERE invited_by = $1 OR email = $2 ORDER BY id FOR UPDATE`,
				`SELECT 1 FROM share_invitations WHERE invited_by = $1 OR accepted_by = $1 OR email = $2 ORDER BY id FOR UPDATE`,
			} {
				if _, err := transaction.Exec(ctx, query, accountID, email); err != nil {
					return err
				}
			}
			// Never wait on a new foreign-key holder while holding invitation rows it might need.
			if _, err := transaction.Exec(ctx, `SELECT 1 FROM accounts WHERE id = $1 FOR UPDATE NOWAIT`, accountID); err != nil {
				return err
			}
			if err := transaction.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM organization_members WHERE account_id = $1
				AND NOT (organization_id = ANY(coalesce($2::uuid[], '{}'))))
			OR EXISTS (SELECT 1 FROM vaults WHERE
				(id IN (SELECT vault_id FROM vault_access WHERE account_id = $1)
					OR organization_id IN (SELECT organization_id FROM organization_members WHERE account_id = $1))
				AND NOT (id = ANY(coalesce($3::uuid[], '{}'))))`, accountID, organizationIDs, vaultIDs).Scan(&restart); err != nil {
				return err
			}
			if restart {
				return nil
			}
			accesses, err := listAccountVaultAccesses(ctx, transaction, accountID)
			if err != nil {
				return err
			}
			deletedVaultIDs := []string{}
			for _, access := range accesses {
				if access.isDeletedWithAccount() {
					deletedVaultIDs = append(deletedVaultIDs, access.vaultID)
				}
			}
			deleted.DeletedVaultCount = len(deletedVaultIDs)
			if deleted.VaultMemberAccountIDs, err = collectAccountIDs(ctx, transaction, `
			SELECT DISTINCT m.account_id FROM vault_access m
			WHERE m.vault_id IN (SELECT vault_id FROM vault_access WHERE account_id = $1) AND m.account_id <> $1`, accountID); err != nil {
				return err
			}
			if deleted.InvitationAccountIDs, err = collectAccountIDs(ctx, transaction, `
			SELECT o.account_id FROM share_invitations i JOIN vault_access o ON o.vault_id = i.vault_id AND o.role = 'owner'
			WHERE (i.email = $2 OR i.accepted_by = $1) AND o.account_id <> $1
			UNION
			SELECT a.id FROM share_invitations i JOIN accounts a ON a.email = i.email
			WHERE (i.invited_by = $1 OR i.vault_id = ANY($3::uuid[])) AND a.id <> $1`, accountID, email, deletedVaultIDs); err != nil {
				return err
			}
			// Everyone in an organization whose invitations change hears of it: the members of the account's organizations,
			// the recipients of its invitations, and the members of every organization with an invitation from or to it.
			if deleted.OrganizationAccountIDs, err = collectAccountIDs(ctx, transaction, `
			SELECT other.account_id FROM organization_members mine JOIN organization_members other ON other.organization_id = mine.organization_id
			WHERE mine.account_id = $1 AND other.account_id <> $1
			UNION
			SELECT a.id FROM organization_invitations i JOIN accounts a ON a.email = i.email
			WHERE i.invited_by = $1 AND a.id <> $1
			UNION
			SELECT m.account_id FROM organization_invitations i JOIN organization_members m ON m.organization_id = i.organization_id
			WHERE (i.invited_by = $1 OR i.email = $2) AND m.account_id <> $1`, accountID, email); err != nil {
				return err
			}
			memberships, err := listAccountMemberships(ctx, transaction, accountID)
			if err != nil {
				return err
			}
			deletedOrganizationIDs := []string{}
			for _, membership := range memberships {
				switch {
				case membership.isDeletedWithAccount():
					deletedOrganizationIDs = append(deletedOrganizationIDs, membership.organizationID)
				case membership.needsNewAdmin():
					if _, err := transaction.Exec(ctx, `UPDATE organization_members SET role = 'admin' WHERE organization_id = $1 AND account_id = $2`,
						membership.organizationID, membership.successorID); err != nil {
						return err
					}
				}
			}
			for _, statement := range []struct {
				query     string
				arguments []any
			}{
				{`DELETE FROM vaults WHERE id = ANY($1::uuid[])`, []any{deletedVaultIDs}},
				{`DELETE FROM vaults WHERE organization_id = ANY($1::uuid[])`, []any{deletedOrganizationIDs}},
				{`DELETE FROM organizations WHERE id = ANY($1::uuid[])`, []any{deletedOrganizationIDs}},
				// The vaults deleted above are gone, so this marks the organization vaults that stay.
				{`UPDATE vaults SET needs_rotation = true WHERE type = 'organization' AND id IN (SELECT vault_id FROM vault_access WHERE account_id = $1)`,
					[]any{accountID}},
				// Email-only invitations can arrive after the upgrade. Lock the deletion's exact snapshot without waiting.
				{`WITH locked AS MATERIALIZED (
					SELECT id FROM organization_invitations WHERE invited_by = $1 OR email = $2 ORDER BY id FOR UPDATE NOWAIT)
					DELETE FROM organization_invitations WHERE id IN (SELECT id FROM locked)`, []any{accountID, email}},
				{`WITH locked AS MATERIALIZED (
					SELECT id FROM share_invitations WHERE invited_by = $1 OR accepted_by = $1 OR email = $2 ORDER BY id FOR UPDATE NOWAIT)
					DELETE FROM share_invitations WHERE id IN (SELECT id FROM locked)`, []any{accountID, email}},
				{`DELETE FROM invites WHERE email = $2 OR (created_by = $1 AND used_at IS NULL)`, []any{accountID, email}},
				{`DELETE FROM share_links WHERE account_id = $1`, []any{accountID}},
				{`DELETE FROM sessions WHERE account_id = $1`, []any{accountID}},
				{`DELETE FROM devices WHERE account_id = $1`, []any{accountID}},
				{`DELETE FROM vault_access WHERE account_id = $1`, []any{accountID}},
				{`UPDATE instance_settings SET updated_by = NULL WHERE updated_by = $1`, []any{accountID}},
				{`DELETE FROM accounts WHERE id = $1`, []any{accountID}},
			} {
				if _, err := transaction.Exec(ctx, statement.query, statement.arguments...); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			var postgresError *pgconn.PgError
			if errors.As(err, &postgresError) && postgresError.Code == "55P03" {
				return nil, ErrDeletionBusy
			}
			return nil, err
		}
		if restart {
			continue
		}
		return deleted, nil
	}
	return nil, ErrDeletionBusy
}
