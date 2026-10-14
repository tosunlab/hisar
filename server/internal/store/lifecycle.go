package store

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

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

type DeletionPreview struct {
	AllOwnedVault           []DeletionOwnedVault
	AllMemberVault          []DeletionMemberVault
	ShareLinkCount          int
	SentInvitationCount     int
	ReceivedInvitationCount int
	DeviceCount             int
}

type DeletedAccount struct {
	Email string
	Name  string
	// AllVaultMemberAccountID shared a vault with the deleted account: their vault list or member list changed.
	AllVaultMemberAccountID []string
	// AllInvitationAccountID had a share invitation from or to the deleted account.
	AllInvitationAccountID []string
	DeletedVaultCount      int
}

func HashOpaqueRecord(opaqueRecord []byte) []byte {
	hash := sha256.Sum256(opaqueRecord)
	return hash[:]
}

// lockProvenAccount fails with ErrConflict when the OPAQUE record changed after the proof, and ErrNotFound when the session ended.
func lockProvenAccount(ctx context.Context, transaction pgx.Tx, accountID string, tokenHash, provenRecordHash []byte) (email, name, deviceID string, err error) {
	var opaqueRecord []byte
	err = transaction.QueryRow(ctx, `SELECT email, name, opaque_record FROM accounts WHERE id = $1 FOR UPDATE`, accountID).Scan(&email, &name, &opaqueRecord)
	if err != nil {
		return "", "", "", convertNoRowsToNotFound(err)
	}
	if subtle.ConstantTimeCompare(HashOpaqueRecord(opaqueRecord), provenRecordHash) != 1 {
		return "", "", "", ErrConflict
	}
	err = transaction.QueryRow(ctx, `
		SELECT s.device_id FROM sessions s JOIN devices d ON d.id = s.device_id
		WHERE s.token_hash = $1 AND s.account_id = $2 AND s.expires_at > now() AND d.revoked_at IS NULL`,
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

func listAllAccountVaultAccess(ctx context.Context, queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, accountID string) ([]accountVaultAccess, error) {
	allRow, err := queryer.Query(ctx, `
		SELECT v.id, v.type, a.role,
			EXISTS (SELECT 1 FROM vault_access o WHERE o.vault_id = v.id AND o.role = 'owner' AND o.account_id <> $1),
			(SELECT count(*) FROM vault_access m WHERE m.vault_id = v.id AND m.account_id <> $1)
		FROM vault_access a JOIN vaults v ON v.id = a.vault_id
		WHERE a.account_id = $1 ORDER BY v.created_at`, accountID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(allRow, func(row pgx.CollectableRow) (accountVaultAccess, error) {
		var access accountVaultAccess
		err := row.Scan(&access.vaultID, &access.vaultType, &access.role, &access.hasOtherOwner, &access.otherMemberCount)
		return access, err
	})
}

func (access accountVaultAccess) isDeletedWithAccount() bool {
	return access.role == "owner" && !access.hasOtherOwner
}

func (s *Store) PreviewAccountDeletion(ctx context.Context, accountID string) (*DeletionPreview, error) {
	account, err := s.GetAccountByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	allAccess, err := listAllAccountVaultAccess(ctx, s.pool, accountID)
	if err != nil {
		return nil, err
	}
	preview := &DeletionPreview{AllOwnedVault: []DeletionOwnedVault{}, AllMemberVault: []DeletionMemberVault{}}
	for _, access := range allAccess {
		if access.isDeletedWithAccount() {
			preview.AllOwnedVault = append(preview.AllOwnedVault, DeletionOwnedVault{ID: access.vaultID, Type: access.vaultType, OtherMemberCount: access.otherMemberCount})
		} else {
			preview.AllMemberVault = append(preview.AllMemberVault, DeletionMemberVault{ID: access.vaultID, Type: access.vaultType, Role: access.role})
		}
	}
	err = s.pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM share_links WHERE account_id = $1 AND expires_at > now()),
			(SELECT count(*) FROM share_invitations WHERE invited_by = $1)
				+ (SELECT count(*) FROM invites i WHERE i.created_by = $1 AND i.used_at IS NULL AND i.expires_at > now()
					AND NOT EXISTS (SELECT 1 FROM share_invitations s WHERE s.invited_by = $1 AND s.email = i.email)),
			(SELECT count(*) FROM share_invitations WHERE (email = $2 OR accepted_by = $1) AND invited_by <> $1),
			(SELECT count(*) FROM devices WHERE account_id = $1 AND revoked_at IS NULL)`,
		accountID, account.Email).Scan(&preview.ShareLinkCount, &preview.SentInvitationCount, &preview.ReceivedInvitationCount, &preview.DeviceCount)
	return preview, err
}

func collectAllAccountID(ctx context.Context, transaction pgx.Tx, query string, allArgument ...any) ([]string, error) {
	allRow, err := transaction.Query(ctx, query, allArgument...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(allRow, pgx.RowTo[string])
}

// DeleteAccount deletes the account and everything that is only its own in one transaction.
// Vaults with another owner stay with them; only this account's access is removed.
func (s *Store) DeleteAccount(ctx context.Context, accountID string, tokenHash, provenRecordHash []byte) (*DeletedAccount, error) {
	deleted := &DeletedAccount{}
	err := s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		email, name, _, err := lockProvenAccount(ctx, transaction, accountID, tokenHash, provenRecordHash)
		if err != nil {
			return err
		}
		deleted.Email, deleted.Name = email, name
		allAccess, err := listAllAccountVaultAccess(ctx, transaction, accountID)
		if err != nil {
			return err
		}
		allDeletedVaultID := []string{}
		for _, access := range allAccess {
			if access.isDeletedWithAccount() {
				allDeletedVaultID = append(allDeletedVaultID, access.vaultID)
			}
		}
		deleted.DeletedVaultCount = len(allDeletedVaultID)
		if deleted.AllVaultMemberAccountID, err = collectAllAccountID(ctx, transaction, `
			SELECT DISTINCT m.account_id FROM vault_access m
			WHERE m.vault_id IN (SELECT vault_id FROM vault_access WHERE account_id = $1) AND m.account_id <> $1`, accountID); err != nil {
			return err
		}
		if deleted.AllInvitationAccountID, err = collectAllAccountID(ctx, transaction, `
			SELECT o.account_id FROM share_invitations i JOIN vault_access o ON o.vault_id = i.vault_id AND o.role = 'owner'
			WHERE (i.email = $2 OR i.accepted_by = $1) AND o.account_id <> $1
			UNION
			SELECT a.id FROM share_invitations i JOIN accounts a ON a.email = i.email
			WHERE (i.invited_by = $1 OR i.vault_id = ANY($3::uuid[])) AND a.id <> $1`, accountID, email, allDeletedVaultID); err != nil {
			return err
		}
		for _, statement := range []struct {
			query       string
			allArgument []any
		}{
			{`DELETE FROM vaults WHERE id = ANY($1::uuid[])`, []any{allDeletedVaultID}},
			{`DELETE FROM share_invitations WHERE invited_by = $1 OR accepted_by = $1 OR email = $2`, []any{accountID, email}},
			{`DELETE FROM invites WHERE email = $2 OR (created_by = $1 AND used_at IS NULL)`, []any{accountID, email}},
			{`DELETE FROM share_links WHERE account_id = $1`, []any{accountID}},
			{`DELETE FROM sessions WHERE account_id = $1`, []any{accountID}},
			{`DELETE FROM devices WHERE account_id = $1`, []any{accountID}},
			{`DELETE FROM vault_access WHERE account_id = $1`, []any{accountID}},
			{`DELETE FROM accounts WHERE id = $1`, []any{accountID}},
		} {
			if _, err := transaction.Exec(ctx, statement.query, statement.allArgument...); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return deleted, nil
}
