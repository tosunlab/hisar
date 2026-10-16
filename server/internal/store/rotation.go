package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

var ErrRotationMembers = errors.New("store: a rotation of an organization vault must keep every member and role")

// RotatedItem is an item re-encrypted under the new vault key; BaseRevision is the revision it replaces.
type RotatedItem struct {
	ID           string
	BaseRevision string
	Revision     string
	Deleted      bool
	Overview     []byte
	Details      []byte
}

// VaultRotation: KeyGeneration is the generation of the key being replaced; organization vaults require it.
type VaultRotation struct {
	VaultID       string
	EncName       []byte
	Accesses      []Member
	Items         []RotatedItem
	RotatedBy     string
	KeyGeneration int64
}

// RotateVault replaces the vault name, all access grants and all items in one transaction, increments the vault's key
// generation and clears needs_rotation. RotatedBy must be an owner (ErrNotOwner). It returns ErrConflict unless
// rotation.Items matches the stored items and revisions exactly, and unless rotation.KeyGeneration is the vault's
// current one (required for organization vaults, checked for any vault when nonzero), so two owners rotating from the
// same key cannot both succeed. It returns ErrNotMember when it would add an account; an organization vault must keep
// exactly its members with their roles (ErrRotationMembers), since its teams decide them.
// Item history is dropped because it is encrypted with the previous key.
func (s *Store) RotateVault(ctx context.Context, rotation VaultRotation) error {
	return s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		vault, err := lockVault(ctx, transaction, rotation.VaultID)
		if err != nil {
			return err
		}
		if err := requireVaultOwner(ctx, transaction, rotation.VaultID, rotation.RotatedBy); err != nil {
			return err
		}
		if rotation.KeyGeneration != vault.keyGeneration && (rotation.KeyGeneration != 0 || vault.organizationID != "") {
			return ErrConflict
		}

		rows, err := transaction.Query(ctx, `SELECT id, revision FROM items WHERE vault_id = $1`, rotation.VaultID)
		if err != nil {
			return err
		}
		currentRevisionsByID := map[string]string{}
		for rows.Next() {
			var itemID, revision string
			if err := rows.Scan(&itemID, &revision); err != nil {
				rows.Close()
				return err
			}
			currentRevisionsByID[itemID] = revision
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(currentRevisionsByID) != len(rotation.Items) {
			return ErrConflict
		}
		for _, rotatedItem := range rotation.Items {
			if currentRevision, found := currentRevisionsByID[rotatedItem.ID]; !found || currentRevision != rotatedItem.BaseRevision {
				return ErrConflict
			}
		}

		currentRolesByAccountID, err := listVaultRoles(ctx, transaction, rotation.VaultID)
		if err != nil {
			return err
		}
		if vault.organizationID != "" && len(rotation.Accesses) != len(currentRolesByAccountID) {
			return ErrRotationMembers
		}
		for _, access := range rotation.Accesses {
			currentRole, isMember := currentRolesByAccountID[access.AccountID]
			switch {
			case vault.organizationID != "" && currentRole != access.Role:
				return ErrRotationMembers
			case !isMember:
				return ErrNotMember
			}
		}

		if _, err := transaction.Exec(ctx, `UPDATE vaults SET enc_name = $2, key_generation = key_generation + 1, needs_rotation = false WHERE id = $1`,
			rotation.VaultID, rotation.EncName); err != nil {
			return err
		}
		if _, err := transaction.Exec(ctx, `DELETE FROM vault_access WHERE vault_id = $1`, rotation.VaultID); err != nil {
			return err
		}
		for _, access := range rotation.Accesses {
			if _, err := transaction.Exec(ctx,
				`INSERT INTO vault_access (vault_id, account_id, wrapped_key, role, granted_by) VALUES ($1, $2, $3, $4, $5)`,
				rotation.VaultID, access.AccountID, access.WrappedKey, access.Role, rotation.RotatedBy); err != nil {
				return err
			}
		}
		if _, err := transaction.Exec(ctx, `DELETE FROM item_history WHERE vault_id = $1`, rotation.VaultID); err != nil {
			return err
		}
		vaultSeq := vault.seq
		for _, rotatedItem := range rotation.Items {
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

func listVaultRoles(ctx context.Context, transaction pgx.Tx, vaultID string) (map[string]string, error) {
	rows, err := transaction.Query(ctx, `SELECT account_id, role FROM vault_access WHERE vault_id = $1`, vaultID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rolesByAccountID := map[string]string{}
	for rows.Next() {
		var accountID, role string
		if err := rows.Scan(&accountID, &role); err != nil {
			return nil, err
		}
		rolesByAccountID[accountID] = role
	}
	return rolesByAccountID, rows.Err()
}
