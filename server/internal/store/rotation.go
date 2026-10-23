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

// VaultRotation identifies the source key, access and metadata snapshots for a bounded aggregate rotation.
type VaultRotation struct {
	VaultID          string
	EncName          []byte
	Accesses         []Member
	Items            []RotatedItem
	RotatedBy        string
	KeyGeneration    int64
	AccessRevision   int64
	MetadataRevision int64
}

// RotateVault is the bounded aggregate route. It compares all item revisions and authority/metadata revisions,
// atomically replaces the name, grants and items, and increments the generation. The rotating owner stays an owner.
// Organization grants retain every current member and role. Old-key history is replaced, not appended.
func (s *Store) RotateVault(ctx context.Context, rotation VaultRotation) error {
	return s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		vault, err := lockVault(ctx, transaction, rotation.VaultID)
		if err != nil {
			return err
		}
		if err := requireVaultOwner(ctx, transaction, rotation.VaultID, rotation.RotatedBy); err != nil {
			return err
		}
		if !vault.acceptsKeyGeneration(rotation.KeyGeneration) {
			if rotation.KeyGeneration == 0 {
				return ErrKeyGenerationMissing
			}
			return ErrStaleKeyGeneration
		}
		if rotation.AccessRevision != vault.accessRevision || rotation.MetadataRevision != vault.metadataRevision {
			return ErrConflict
		}
		before, err := HistoryUsage(ctx, transaction, rotation.VaultID)
		if err != nil {
			return err
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
		keptAccountIDs := make([]string, 0, len(rotation.Accesses))
		for _, access := range rotation.Accesses {
			if _, err := transaction.Exec(ctx,
				`UPDATE vault_access SET wrapped_key=$3,role=$4,granted_by=$5,granted_at=now() WHERE vault_id=$1 AND account_id=$2`,
				rotation.VaultID, access.AccountID, access.WrappedKey, access.Role, rotation.RotatedBy); err != nil {
				return err
			}
			keptAccountIDs = append(keptAccountIDs, access.AccountID)
		}
		if _, err := transaction.Exec(ctx, `DELETE FROM vault_access WHERE vault_id=$1 AND NOT(account_id=ANY($2::text[]))`, rotation.VaultID, keptAccountIDs); err != nil {
			return err
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
		if _, err := transaction.Exec(ctx, `UPDATE vaults SET seq = $2 WHERE id = $1`, rotation.VaultID, vaultSeq); err != nil {
			return err
		}
		return CheckHistoryGrowth(ctx, transaction, rotation.VaultID, before, false)
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
