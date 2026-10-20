package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Item struct {
	ID        string
	Revision  string
	Seq       int64
	Deleted   bool
	Overview  []byte
	Details   []byte
	UpdatedAt time.Time
	UpdatedBy string
}

func scanItem(row pgx.CollectableRow) (Item, error) {
	var item Item
	err := row.Scan(&item.ID, &item.Revision, &item.Seq, &item.Deleted, &item.Overview, &item.Details, &item.UpdatedAt, &item.UpdatedBy)
	return item, err
}

// ListChanges returns items with seq > since, and the vault's current seq. A page ends after limit items, or once its
// sealed overviews and details reach maxBytes. One item can cross the budget; the first item is always included.
// isMore reports that the page ended at one of those bounds.
func (s *Store) ListChanges(ctx context.Context, vaultID string, since int64, limit int, maxBytes int64) (items []Item, vaultSeq int64, isMore bool, err error) {
	if err := s.pool.QueryRow(ctx, `SELECT seq FROM vaults WHERE id = $1`, vaultID).Scan(&vaultSeq); err != nil {
		return nil, 0, false, convertNoRowsToNotFound(err)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, revision, seq, deleted, CASE WHEN fits THEN overview END, CASE WHEN fits THEN details END, updated_at, updated_by, fits
		FROM (
			SELECT *, sum(octet_length(overview) + octet_length(details)) OVER (ORDER BY seq) - octet_length(overview) - octet_length(details) < $4 AS fits
			FROM items WHERE vault_id = $1 AND seq > $2 ORDER BY seq LIMIT $3
		) page ORDER BY seq`, vaultID, since, limit, maxBytes)
	if err != nil {
		return nil, 0, false, err
	}
	defer rows.Close()
	rowCount := 0
	for rows.Next() {
		rowCount++
		var item Item
		var fits bool
		if err := rows.Scan(&item.ID, &item.Revision, &item.Seq, &item.Deleted, &item.Overview, &item.Details, &item.UpdatedAt, &item.UpdatedBy, &fits); err != nil {
			return nil, 0, false, err
		}
		if fits {
			items = append(items, item)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, false, err
	}
	return items, vaultSeq, rowCount == limit || len(items) < rowCount, nil
}

// PutItem writes a new revision if baseRevision matches the stored one ("" for a new item).
// On ErrConflict the current item is returned; it is nil when the item does not exist.
// Re-sending an already stored revision is a no-op so that clients can retry safely.
// It locks only the vault, since an item write changes no access (§5.11), and, before any revision check, returns
// ErrStaleKeyGeneration or ErrKeyGenerationMissing unless the vault accepts keyGeneration, so an item sealed with a key a
// rotation replaced is never stored.
func (s *Store) PutItem(ctx context.Context, vaultID, baseRevision string, keyGeneration int64, item *Item) (*Item, error) {
	var current *Item
	err := s.runInTransaction(ctx, func(transaction pgx.Tx) (err error) {
		current, err = putItem(ctx, transaction, vaultID, baseRevision, keyGeneration, item)
		return err
	})
	return current, err
}

// putItem is PutItem in transaction.
func putItem(ctx context.Context, transaction pgx.Tx, vaultID, baseRevision string, keyGeneration int64, item *Item) (*Item, error) {
	vault, err := lockVaultRow(ctx, transaction, vaultID)
	if err != nil {
		return nil, err
	}
	if err := vault.checkItemKeyGeneration(keyGeneration); err != nil {
		return nil, err
	}

	rows, err := transaction.Query(ctx, `
		SELECT id, revision, seq, deleted, overview, details, updated_at, updated_by
		FROM items WHERE vault_id = $1 AND id = $2`, vaultID, item.ID)
	if err != nil {
		return nil, err
	}
	existingItems, err := pgx.CollectRows(rows, scanItem)
	if err != nil {
		return nil, err
	}

	switch {
	case len(existingItems) == 1 && existingItems[0].Revision == item.Revision:
		*item = existingItems[0]
		return nil, nil
	case len(existingItems) == 1 && existingItems[0].Revision != baseRevision:
		return &existingItems[0], ErrConflict
	case len(existingItems) == 0 && baseRevision != "":
		return nil, ErrConflict
	}

	return nil, writeItemRevision(ctx, transaction, vaultID, item)
}

func (s *Store) GetItemHistory(ctx context.Context, vaultID, itemID string) ([]Item, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, revision, 0::bigint, deleted, overview, details, updated_at, updated_by
		FROM item_history WHERE vault_id = $1 AND id = $2 ORDER BY updated_at DESC`, vaultID, itemID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanItem)
}

// PruneHistory deletes history older than maxAge, always keeping each item's current revision.
func (s *Store) PruneHistory(ctx context.Context, maxAge time.Duration) (int64, error) {
	commandTag, err := s.pool.Exec(ctx, `
		DELETE FROM item_history h WHERE h.updated_at < now() - $1::interval
		AND NOT EXISTS (SELECT 1 FROM items i WHERE i.vault_id = h.vault_id AND i.id = h.id AND i.revision = h.revision)`,
		maxAge)
	return commandTag.RowsAffected(), err
}

// ItemMove moves one live item between vaults: Tombstone replaces it in the source vault and Moved, with the same ID,
// is written to the target vault. SourceKeyGeneration and TargetKeyGeneration are the generations of the vault keys they
// are sealed with.
type ItemMove struct {
	SourceVaultID       string
	TargetVaultID       string
	AccountID           string
	BaseRevision        string
	Tombstone           Item
	Moved               Item
	SourceKeyGeneration int64
	TargetKeyGeneration int64
}

// MoveItem moves the item in one transaction, so it is never in both vaults or in neither. It locks only the two
// vaults, since a move changes no access (§5.11). It refuses with ErrConflict, returning the current item, unless
// BaseRevision is the item's live revision in the source vault; with ErrExists when the target vault has a live item
// with the same ID; with ErrTargetNotWritable unless the account may write the target vault; and with
// ErrStaleKeyGeneration or ErrKeyGenerationMissing unless both vaults accept the key generations the item is sealed
// with. On success Tombstone.Seq and Moved.Seq hold the vaults' new sequence numbers.
func (s *Store) MoveItem(ctx context.Context, move *ItemMove) (*Item, error) {
	var current *Item
	err := s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		vaultsByID, err := lockVaults(ctx, transaction, []string{move.SourceVaultID, move.TargetVaultID})
		if err != nil {
			return err
		}
		source, target := vaultsByID[move.SourceVaultID], vaultsByID[move.TargetVaultID]
		if source == nil || target == nil {
			return ErrNotFound
		}
		var targetRole string
		err = transaction.QueryRow(ctx, `SELECT role FROM vault_access WHERE vault_id = $1 AND account_id = $2`, move.TargetVaultID, move.AccountID).Scan(&targetRole)
		if err != nil {
			return convertNoRowsToNotFound(err)
		}
		if targetRole != "owner" && targetRole != "editor" {
			return ErrTargetNotWritable
		}
		if err := source.checkItemKeyGeneration(move.SourceKeyGeneration); err != nil {
			return err
		}
		if err := target.checkItemKeyGeneration(move.TargetKeyGeneration); err != nil {
			return err
		}
		itemID := move.Moved.ID
		rows, err := transaction.Query(ctx, `
			SELECT id, revision, seq, deleted, overview, details, updated_at, updated_by
			FROM items WHERE vault_id = $1 AND id = $2`, move.SourceVaultID, itemID)
		if err != nil {
			return err
		}
		sourceItems, err := pgx.CollectRows(rows, scanItem)
		if err != nil {
			return err
		}
		switch {
		case len(sourceItems) == 0 || sourceItems[0].Deleted:
			return ErrConflict
		case sourceItems[0].Revision != move.BaseRevision:
			current = &sourceItems[0]
			return ErrConflict
		}
		var isTargetLive bool
		err = transaction.QueryRow(ctx, `SELECT NOT deleted FROM items WHERE vault_id = $1 AND id = $2`, move.TargetVaultID, itemID).Scan(&isTargetLive)
		if err == nil && isTargetLive {
			return ErrExists
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		for _, write := range []struct {
			vaultID string
			item    *Item
		}{{move.SourceVaultID, &move.Tombstone}, {move.TargetVaultID, &move.Moved}} {
			write.item.ID, write.item.UpdatedBy = itemID, move.AccountID
			if err := writeItemRevision(ctx, transaction, write.vaultID, write.item); err != nil {
				return err
			}
		}
		return nil
	})
	return current, err
}

// writeItemRevision stores item as the vault's next change, replacing any earlier row with its ID, and records it in the
// item's history.
func writeItemRevision(ctx context.Context, transaction pgx.Tx, vaultID string, item *Item) error {
	if err := transaction.QueryRow(ctx, `UPDATE vaults SET seq = seq + 1 WHERE id = $1 RETURNING seq`, vaultID).Scan(&item.Seq); err != nil {
		return err
	}
	err := transaction.QueryRow(ctx, `
		INSERT INTO items (vault_id, id, revision, seq, deleted, overview, details, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (vault_id, id) DO UPDATE SET
			revision = $3, seq = $4, deleted = $5, overview = $6, details = $7, updated_by = $8, updated_at = now()
		RETURNING updated_at`,
		vaultID, item.ID, item.Revision, item.Seq, item.Deleted, item.Overview, item.Details, item.UpdatedBy).Scan(&item.UpdatedAt)
	if err != nil {
		return err
	}
	_, err = transaction.Exec(ctx, `
		INSERT INTO item_history (vault_id, id, revision, deleted, overview, details, updated_at, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		vaultID, item.ID, item.Revision, item.Deleted, item.Overview, item.Details, item.UpdatedAt, item.UpdatedBy)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}
