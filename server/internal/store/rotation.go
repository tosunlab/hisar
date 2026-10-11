package store

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// RotatedItem is an item re-encrypted under the new vault key; BaseRevision is the revision it replaces.
type RotatedItem struct {
	ID           string
	BaseRevision string
	Revision     string
	Deleted      bool
	Overview     []byte
	Details      []byte
}

type VaultRotation struct {
	VaultID   string
	EncName   []byte
	AllAccess []Member
	AllItem   []RotatedItem
	RotatedBy string
	// RecoveryWrappedKey replaces the vault's recovery wrapping; it is required when the vault has one.
	RecoveryWrappedKey []byte
}

// RotateVault replaces the vault name, all access grants and all items in one transaction.
// It returns ErrConflict unless rotation.AllItem matches the stored items and revisions exactly,
// or when the vault has a recovery wrapping and the rotation carries none, and ErrNotMember when it would add an account.
// Item history is dropped because it is encrypted with the previous key.
func (s *Store) RotateVault(ctx context.Context, rotation VaultRotation) error {
	return s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		var vaultSeq int64
		if err := transaction.QueryRow(ctx, `SELECT seq FROM vaults WHERE id = $1 FOR UPDATE`, rotation.VaultID).Scan(&vaultSeq); err != nil {
			return convertNoRowsToNotFound(err)
		}

		allRow, err := transaction.Query(ctx, `SELECT id, revision FROM items WHERE vault_id = $1`, rotation.VaultID)
		if err != nil {
			return err
		}
		allCurrentRevisionByID := map[string]string{}
		for allRow.Next() {
			var itemID, revision string
			if err := allRow.Scan(&itemID, &revision); err != nil {
				allRow.Close()
				return err
			}
			allCurrentRevisionByID[itemID] = revision
		}
		allRow.Close()
		if err := allRow.Err(); err != nil {
			return err
		}
		if len(allCurrentRevisionByID) != len(rotation.AllItem) {
			return ErrConflict
		}
		for _, rotatedItem := range rotation.AllItem {
			if currentRevision, found := allCurrentRevisionByID[rotatedItem.ID]; !found || currentRevision != rotatedItem.BaseRevision {
				return ErrConflict
			}
		}

		var currentMemberCount int
		allRotatedAccountID := make([]string, 0, len(rotation.AllAccess))
		for _, access := range rotation.AllAccess {
			allRotatedAccountID = append(allRotatedAccountID, access.AccountID)
		}
		if err := transaction.QueryRow(ctx, `SELECT count(*) FROM vault_access WHERE vault_id = $1 AND account_id = ANY($2)`,
			rotation.VaultID, allRotatedAccountID).Scan(&currentMemberCount); err != nil {
			return err
		}
		if currentMemberCount != len(rotation.AllAccess) {
			return ErrNotMember
		}

		if rotation.RecoveryWrappedKey == nil {
			var isRecoverable bool
			if err := transaction.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM vault_recovery WHERE vault_id = $1)`, rotation.VaultID).Scan(&isRecoverable); err != nil {
				return err
			}
			if isRecoverable {
				return ErrConflict
			}
		} else if _, err := transaction.Exec(ctx, `
			INSERT INTO vault_recovery (vault_id, wrapped_key) VALUES ($1, $2)
			ON CONFLICT (vault_id) DO UPDATE SET wrapped_key = $2, updated_at = now()`,
			rotation.VaultID, rotation.RecoveryWrappedKey); err != nil {
			return err
		}

		if _, err := transaction.Exec(ctx, `UPDATE vaults SET enc_name = $2 WHERE id = $1`, rotation.VaultID, rotation.EncName); err != nil {
			return err
		}
		if _, err := transaction.Exec(ctx, `DELETE FROM vault_access WHERE vault_id = $1`, rotation.VaultID); err != nil {
			return err
		}
		for _, access := range rotation.AllAccess {
			if _, err := transaction.Exec(ctx,
				`INSERT INTO vault_access (vault_id, account_id, wrapped_key, role, granted_by) VALUES ($1, $2, $3, $4, $5)`,
				rotation.VaultID, access.AccountID, access.WrappedKey, access.Role, rotation.RotatedBy); err != nil {
				return err
			}
		}
		if _, err := transaction.Exec(ctx, `DELETE FROM item_history WHERE vault_id = $1`, rotation.VaultID); err != nil {
			return err
		}
		for _, rotatedItem := range rotation.AllItem {
			vaultSeq++
			if _, err := transaction.Exec(ctx, `
				UPDATE items SET revision = $3, seq = $4, deleted = $5, overview = $6, details = $7, updated_by = $8, updated_at = now()
				WHERE vault_id = $1 AND id = $2`,
				rotation.VaultID, rotatedItem.ID, rotatedItem.Revision, vaultSeq, rotatedItem.Deleted, rotatedItem.Overview, rotatedItem.Details, rotation.RotatedBy); err != nil {
				return err
			}
			if _, err := transaction.Exec(ctx, `
				INSERT INTO item_history (vault_id, id, revision, deleted, overview, details, updated_at, updated_by)
				VALUES ($1, $2, $3, $4, $5, $6, now(), $7)`,
				rotation.VaultID, rotatedItem.ID, rotatedItem.Revision, rotatedItem.Deleted, rotatedItem.Overview, rotatedItem.Details, rotation.RotatedBy); err != nil {
				return err
			}
		}
		_, err = transaction.Exec(ctx, `UPDATE vaults SET seq = $2 WHERE id = $1`, rotation.VaultID, vaultSeq)
		return err
	})
}
