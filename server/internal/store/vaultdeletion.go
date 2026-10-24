package store

import (
	"bytes"
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

var (
	ErrNotOwner          = errors.New("store: not an owner of the vault")
	ErrPersonalVault     = errors.New("store: personal vaults cannot be deleted")
	ErrOnlyVault         = errors.New("store: the account's only vault cannot be deleted")
	ErrVaultNotEmpty     = errors.New("store: the vault has items")
	ErrTargetNotWritable = errors.New("store: the target vault is not writable")
	ErrOnlyOwner         = errors.New("store: the only owner cannot leave the vault")
	ErrReviewChanged     = errors.New("store: the vault changed since it was reviewed")
)

// VaultReview is the vault record a recovery action was reviewed against (apitypes.VaultReview).
type VaultReview struct {
	EncName       []byte
	WrappedKey    []byte
	KeyGeneration int64
}

// checkReview returns ErrReviewChanged unless the vault's sealed name, the account's wrapped key and the key generation
// are still the reviewed ones. It runs under the vault's lock, so nothing changes them before the action commits.
func checkReview(ctx context.Context, transaction pgx.Tx, vaultID, accountID string, vault *lockedVault, review *VaultReview) error {
	if review == nil {
		return nil
	}
	var encName, wrappedKey []byte
	if err := transaction.QueryRow(ctx, `SELECT v.enc_name, a.wrapped_key FROM vaults v JOIN vault_access a ON a.vault_id = v.id WHERE v.id = $1 AND a.account_id = $2`,
		vaultID, accountID).Scan(&encName, &wrappedKey); err != nil {
		return convertNoRowsToNotFound(err)
	}
	if vault.keyGeneration != review.KeyGeneration || !bytes.Equal(encName, review.EncName) || !bytes.Equal(wrappedKey, review.WrappedKey) {
		return ErrReviewChanged
	}
	return nil
}

// MovedItem is an item re-encrypted for the target vault; BaseRevision is its revision in the deleted vault.
type MovedItem struct {
	ID           string
	BaseRevision string
	Revision     string
	Overview     []byte
	Details      []byte
}

// VaultDeletion: with MoveToVaultID, MovedItems must be exactly the vault's live items at their current revisions, sealed
// with the target vault's key of MoveToKeyGeneration. With Review, the vault must still be as reviewed.
type VaultDeletion struct {
	VaultID             string
	AccountID           string
	MoveToVaultID       string
	MovedItems          []MovedItem
	IsDeletingItems     bool
	MoveToKeyGeneration int64
	KeyGeneration       int64
	Review              *VaultReview
}

// RenameVault checks owner authority and the current key policy under the vault lock before storing the sealed name.
// With review, the vault must still be as reviewed (ErrReviewChanged).
func (s *Store) RenameVault(ctx context.Context, vaultID, renamedBy string, encName []byte, keyGeneration int64, review *VaultReview) error {
	return s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		vault, err := lockVault(ctx, transaction, vaultID)
		if err != nil {
			return err
		}
		if err := requireVaultOwner(ctx, transaction, vaultID, renamedBy); err != nil {
			return err
		}
		if err := vault.checkItemKeyGeneration(keyGeneration); err != nil {
			return err
		}
		if err := checkReview(ctx, transaction, vaultID, renamedBy, vault, review); err != nil {
			return err
		}
		_, err = transaction.Exec(ctx, `UPDATE vaults SET enc_name = $2 WHERE id = $1`, vaultID, encName)
		return err
	})
}

// DeleteVault deletes the vault, its access grants, items, history, invitations and team grants in one transaction,
// after moving its live items into another vault when asked. It returns the accounts that had access to the deleted
// vault. Deleting an organization vault removes access, so its organization is locked first (§5.11); the target only
// receives items, so only its row is locked. Both source and destination enforce the generation policy.
func (s *Store) DeleteVault(ctx context.Context, deletion VaultDeletion) ([]string, error) {
	var formerMemberIDs []string
	err := s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		if err := lockVaultOrganization(ctx, transaction, deletion.VaultID); err != nil {
			return err
		}
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
		if !vault.acceptsKeyGeneration(deletion.KeyGeneration) {
			if deletion.KeyGeneration == 0 {
				return ErrKeyGenerationMissing
			}
			return ErrStaleKeyGeneration
		}

		var role string
		if err := transaction.QueryRow(ctx, `SELECT role FROM vault_access WHERE vault_id = $1 AND account_id = $2`,
			deletion.VaultID, deletion.AccountID).Scan(&role); err != nil {
			return convertNoRowsToNotFound(err)
		}
		if role != "owner" {
			return ErrNotOwner
		}
		if err := checkReview(ctx, transaction, deletion.VaultID, deletion.AccountID, vault, deletion.Review); err != nil {
			return err
		}
		if vault.vaultType == "personal" {
			var isPrimary bool
			if err := transaction.QueryRow(ctx, `SELECT $1::uuid = (SELECT id FROM vaults WHERE created_by = $2 AND type = 'personal' ORDER BY created_at, id LIMIT 1)`, deletion.VaultID, deletion.AccountID).Scan(&isPrimary); err != nil {
				return err
			}
			if isPrimary {
				return ErrPersonalVault
			}
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
	if err := target.checkItemKeyGeneration(deletion.MoveToKeyGeneration); err != nil {
		return err
	}
	if len(deletion.MovedItems) != len(liveRevisionsByID) {
		return ErrConflict
	}
	for _, movedItem := range deletion.MovedItems {
		if revision, isFound := liveRevisionsByID[movedItem.ID]; !isFound || revision != movedItem.BaseRevision {
			return ErrConflict
		}
	}
	movedItemIDs := make([]string, 0, len(deletion.MovedItems))
	for _, movedItem := range deletion.MovedItems {
		movedItemIDs = append(movedItemIDs, movedItem.ID)
	}
	var isTargetLive bool
	if err := transaction.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM items WHERE vault_id = $1 AND id = ANY($2::uuid[]) AND NOT deleted)`,
		deletion.MoveToVaultID, movedItemIDs).Scan(&isTargetLive); err != nil {
		return err
	}
	if isTargetLive {
		return ErrExists
	}
	for _, movedItem := range deletion.MovedItems {
		item := &Item{ID: movedItem.ID, Revision: movedItem.Revision, Overview: movedItem.Overview, Details: movedItem.Details, UpdatedBy: deletion.AccountID}
		if err := writeItemRevision(ctx, transaction, deletion.MoveToVaultID, item); err != nil {
			return err
		}
	}
	return nil
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
