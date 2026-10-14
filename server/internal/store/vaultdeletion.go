package store

import (
	"context"
	"errors"
	"slices"
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

// VaultDeletion: with MoveToVaultID, AllMovedItem must be exactly the vault's live items at their current revisions.
type VaultDeletion struct {
	VaultID         string
	AccountID       string
	MoveToVaultID   string
	AllMovedItem    []MovedItem
	IsDeletingItems bool
}

func (s *Store) RenameVault(ctx context.Context, vaultID string, encName []byte) error {
	commandTag, err := s.pool.Exec(ctx, `UPDATE vaults SET enc_name = $2 WHERE id = $1`, vaultID, encName)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteVault deletes the vault, its access grants, items, history and invitations in one transaction, after moving
// its live items into another vault when asked. It returns the accounts that had access to the deleted vault.
func (s *Store) DeleteVault(ctx context.Context, deletion VaultDeletion) ([]string, error) {
	var allFormerMemberID []string
	err := s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		allLockedVaultID := []string{deletion.VaultID}
		if deletion.MoveToVaultID != "" {
			allLockedVaultID = append(allLockedVaultID, deletion.MoveToVaultID)
		}
		// Locking in ID order keeps two concurrent moves between the same vaults from deadlocking.
		slices.Sort(allLockedVaultID)
		allRow, err := transaction.Query(ctx, `SELECT id::text, type FROM vaults WHERE id = ANY($1::uuid[]) ORDER BY id FOR UPDATE`, allLockedVaultID)
		if err != nil {
			return err
		}
		allVaultTypeByID := map[string]string{}
		for allRow.Next() {
			var vaultID, vaultType string
			if err := allRow.Scan(&vaultID, &vaultType); err != nil {
				allRow.Close()
				return err
			}
			allVaultTypeByID[vaultID] = vaultType
		}
		allRow.Close()
		if err := allRow.Err(); err != nil {
			return err
		}
		vaultType, isFound := allVaultTypeByID[deletion.VaultID]
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
		if vaultType == "personal" {
			return ErrPersonalVault
		}
		var vaultCount int
		if err := transaction.QueryRow(ctx, `SELECT count(*) FROM vault_access WHERE account_id = $1`, deletion.AccountID).Scan(&vaultCount); err != nil {
			return err
		}
		if vaultCount <= 1 {
			return ErrOnlyVault
		}

		allLiveRevisionByID, err := listAllLiveRevision(ctx, transaction, deletion.VaultID)
		if err != nil {
			return err
		}
		switch {
		case deletion.MoveToVaultID != "":
			if _, isFound := allVaultTypeByID[deletion.MoveToVaultID]; !isFound {
				return ErrNotFound
			}
			if err := moveAllItem(ctx, transaction, deletion, allLiveRevisionByID); err != nil {
				return err
			}
		case len(allLiveRevisionByID) > 0 && !deletion.IsDeletingItems:
			return ErrVaultNotEmpty
		}

		memberRow, err := transaction.Query(ctx, `SELECT account_id FROM vault_access WHERE vault_id = $1`, deletion.VaultID)
		if err != nil {
			return err
		}
		if allFormerMemberID, err = pgx.CollectRows(memberRow, pgx.RowTo[string]); err != nil {
			return err
		}
		_, err = transaction.Exec(ctx, `DELETE FROM vaults WHERE id = $1`, deletion.VaultID)
		return err
	})
	return allFormerMemberID, err
}

func listAllLiveRevision(ctx context.Context, transaction pgx.Tx, vaultID string) (map[string]string, error) {
	allRow, err := transaction.Query(ctx, `SELECT id::text, revision::text FROM items WHERE vault_id = $1 AND NOT deleted`, vaultID)
	if err != nil {
		return nil, err
	}
	defer allRow.Close()
	allRevisionByID := map[string]string{}
	for allRow.Next() {
		var itemID, revision string
		if err := allRow.Scan(&itemID, &revision); err != nil {
			return nil, err
		}
		allRevisionByID[itemID] = revision
	}
	return allRevisionByID, allRow.Err()
}

func moveAllItem(ctx context.Context, transaction pgx.Tx, deletion VaultDeletion, allLiveRevisionByID map[string]string) error {
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
	if len(deletion.AllMovedItem) != len(allLiveRevisionByID) {
		return ErrConflict
	}
	for _, movedItem := range deletion.AllMovedItem {
		if revision, isFound := allLiveRevisionByID[movedItem.ID]; !isFound || revision != movedItem.BaseRevision {
			return ErrConflict
		}
	}
	var targetSeq int64
	if err := transaction.QueryRow(ctx, `SELECT seq FROM vaults WHERE id = $1`, deletion.MoveToVaultID).Scan(&targetSeq); err != nil {
		return err
	}
	for _, movedItem := range deletion.AllMovedItem {
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
func (s *Store) LeaveVault(ctx context.Context, vaultID, accountID string) error {
	return s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		if _, err := transaction.Exec(ctx, `SELECT 1 FROM vaults WHERE id = $1 FOR UPDATE`, vaultID); err != nil {
			return err
		}
		var role string
		if err := transaction.QueryRow(ctx, `SELECT role FROM vault_access WHERE vault_id = $1 AND account_id = $2`,
			vaultID, accountID).Scan(&role); err != nil {
			return convertNoRowsToNotFound(err)
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
		_, err := transaction.Exec(ctx, `DELETE FROM vault_access WHERE vault_id = $1 AND account_id = $2`, vaultID, accountID)
		return err
	})
}
