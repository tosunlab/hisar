package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrNotOwner          = errors.New("store: not an owner of the vault")
	ErrPersonalVault     = errors.New("store: personal vaults cannot be deleted")
	ErrOnlyVault         = errors.New("store: the account's only vault cannot be deleted")
	ErrVaultNotEmpty     = errors.New("store: the vault has items")
	ErrTargetNotWritable = errors.New("store: the target vault is not writable")
	ErrOnlyOwner         = errors.New("store: the only owner cannot leave the vault")
)

// MovedItem is an item re-encrypted for the target vault; BaseRevision is its revision in the deleted vault.
type MovedItem struct {
	ID           string
	BaseRevision string
	Revision     string
	Overview     []byte
	Details      []byte
}

// VaultDeletion: with MoveToVaultID, MovedItems must be exactly the vault's live items at their current revisions, sealed
// with the target vault's key of MoveToKeyGeneration.
type VaultDeletion struct {
	VaultID             string
	AccountID           string
	MoveToVaultID       string
	MovedItems          []MovedItem
	IsDeletingItems     bool
	MoveToKeyGeneration int64
}

// RenameVault stores the vault's new sealed name. It locks like every change to an organization vault (§5.11) and
// returns ErrConflict unless keyGeneration is the vault's current one (required for organization vaults, checked for any
// vault when nonzero), so a name sealed with a key a rotation replaced is never stored.
func (s *Store) RenameVault(ctx context.Context, vaultID string, encName []byte, keyGeneration int64) error {
	return s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		vault, err := lockVault(ctx, transaction, vaultID)
		if err != nil {
			return err
		}
		if !vault.acceptsKeyGeneration(keyGeneration) {
			return ErrConflict
		}
		_, err = transaction.Exec(ctx, `UPDATE vaults SET enc_name = $2 WHERE id = $1`, vaultID, encName)
		return err
	})
}

// DeleteVault deletes the vault, its access grants, items, history, invitations and team grants in one transaction,
// after moving its live items into another vault when asked. It returns the accounts that had access to the deleted
// vault. The organizations of both vaults are locked first (§5.11), and moved items sealed with a target key a rotation
// replaced are refused (ErrStaleKeyGeneration).
func (s *Store) DeleteVault(ctx context.Context, deletion VaultDeletion) ([]string, error) {
	var formerMemberIDs []string
	err := s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		lockedVaultIDs := []string{deletion.VaultID}
		if deletion.MoveToVaultID != "" {
			lockedVaultIDs = append(lockedVaultIDs, deletion.MoveToVaultID)
		}
		vaultsByID, err := lockVaults(ctx, transaction, lockedVaultIDs)
		if err != nil {
			return err
		}
		vault, isFound := vaultsByID[deletion.VaultID]
		if !isFound {
			return ErrNotFound
		}

		var role string
		if err := transaction.QueryRow(ctx, `SELECT role FROM vault_access WHERE vault_id = $1 AND account_id = $2`,
			deletion.VaultID, deletion.AccountID).Scan(&role); err != nil {
			return convertNoRowsToNotFound(err)
		}
		if role != "owner" {
			return ErrNotOwner
		}
		if vault.vaultType == "personal" {
			return ErrPersonalVault
		}
		var vaultCount int
		if err := transaction.QueryRow(ctx, `SELECT count(*) FROM vault_access WHERE account_id = $1`, deletion.AccountID).Scan(&vaultCount); err != nil {
			return err
		}
		if vaultCount <= 1 {
			return ErrOnlyVault
		}

		liveRevisionsByID, err := listLiveRevisions(ctx, transaction, deletion.VaultID)
		if err != nil {
			return err
		}
		switch {
		case deletion.MoveToVaultID != "":
			target, isFound := vaultsByID[deletion.MoveToVaultID]
			if !isFound {
				return ErrNotFound
			}
			if err := moveItems(ctx, transaction, deletion, target, liveRevisionsByID); err != nil {
				return err
			}
		case len(liveRevisionsByID) > 0 && !deletion.IsDeletingItems:
			return ErrVaultNotEmpty
		}

		memberRow, err := transaction.Query(ctx, `SELECT account_id FROM vault_access WHERE vault_id = $1`, deletion.VaultID)
		if err != nil {
			return err
		}
		if formerMemberIDs, err = pgx.CollectRows(memberRow, pgx.RowTo[string]); err != nil {
			return err
		}
		_, err = transaction.Exec(ctx, `DELETE FROM vaults WHERE id = $1`, deletion.VaultID)
		return err
	})
	return formerMemberIDs, err
}

func listLiveRevisions(ctx context.Context, transaction pgx.Tx, vaultID string) (map[string]string, error) {
	rows, err := transaction.Query(ctx, `SELECT id::text, revision::text FROM items WHERE vault_id = $1 AND NOT deleted`, vaultID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	revisionsByID := map[string]string{}
	for rows.Next() {
		var itemID, revision string
		if err := rows.Scan(&itemID, &revision); err != nil {
			return nil, err
		}
		revisionsByID[itemID] = revision
	}
	return revisionsByID, rows.Err()
}

func moveItems(ctx context.Context, transaction pgx.Tx, deletion VaultDeletion, target *lockedVault, liveRevisionsByID map[string]string) error {
	if deletion.MoveToVaultID == deletion.VaultID {
		return ErrTargetNotWritable
	}
	var targetRole string
	err := transaction.QueryRow(ctx, `SELECT role FROM vault_access WHERE vault_id = $1 AND account_id = $2`,
		deletion.MoveToVaultID, deletion.AccountID).Scan(&targetRole)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if targetRole != "owner" && targetRole != "editor" {
		return ErrTargetNotWritable
	}
	if !target.acceptsKeyGeneration(deletion.MoveToKeyGeneration) {
		return ErrStaleKeyGeneration
	}
	if len(deletion.MovedItems) != len(liveRevisionsByID) {
		return ErrConflict
	}
	for _, movedItem := range deletion.MovedItems {
		if revision, isFound := liveRevisionsByID[movedItem.ID]; !isFound || revision != movedItem.BaseRevision {
			return ErrConflict
		}
	}
	var targetSeq int64
	if err := transaction.QueryRow(ctx, `SELECT seq FROM vaults WHERE id = $1`, deletion.MoveToVaultID).Scan(&targetSeq); err != nil {
		return err
	}
	for _, movedItem := range deletion.MovedItems {
		targetSeq++
		var updatedAt time.Time
		err := transaction.QueryRow(ctx, `
			INSERT INTO items (vault_id, id, revision, seq, deleted, overview, details, updated_by)
			VALUES ($1, $2, $3, $4, false, $5, $6, $7) RETURNING updated_at`,
			deletion.MoveToVaultID, movedItem.ID, movedItem.Revision, targetSeq, movedItem.Overview, movedItem.Details, deletion.AccountID).Scan(&updatedAt)
		if isUniqueViolation(err) {
			return ErrExists
		}
		if err != nil {
			return err
		}
		if _, err := transaction.Exec(ctx, `
			INSERT INTO item_history (vault_id, id, revision, deleted, overview, details, updated_at, updated_by)
			VALUES ($1, $2, $3, false, $4, $5, $6, $7)`,
			deletion.MoveToVaultID, movedItem.ID, movedItem.Revision, movedItem.Overview, movedItem.Details, updatedAt, deletion.AccountID); err != nil {
			return err
		}
	}
	_, err = transaction.Exec(ctx, `UPDATE vaults SET seq = $2 WHERE id = $1`, deletion.MoveToVaultID, targetSeq)
	return err
}

// LeaveVault removes the account's access and wrapped key; an owner may leave only while another owner remains.
// Access to an organization vault comes from its teams (ErrOrganizationVault).
func (s *Store) LeaveVault(ctx context.Context, vaultID, accountID string) error {
	return s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		vault, err := lockVault(ctx, transaction, vaultID)
		if err != nil {
			return err
		}
		var role string
		if err := transaction.QueryRow(ctx, `SELECT role FROM vault_access WHERE vault_id = $1 AND account_id = $2`,
			vaultID, accountID).Scan(&role); err != nil {
			return convertNoRowsToNotFound(err)
		}
		if vault.organizationID != "" {
			return ErrOrganizationVault
		}
		if role == "owner" {
			var ownerCount int
			if err := transaction.QueryRow(ctx, `SELECT count(*) FROM vault_access WHERE vault_id = $1 AND role = 'owner'`, vaultID).Scan(&ownerCount); err != nil {
				return err
			}
			if ownerCount <= 1 {
				return ErrOnlyOwner
			}
		}
		_, err = transaction.Exec(ctx, `DELETE FROM vault_access WHERE vault_id = $1 AND account_id = $2`, vaultID, accountID)
		return err
	})
}
