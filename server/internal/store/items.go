package store

import (
	"context"
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

// ListChanges returns items with seq > since, and the vault's current seq.
func (s *Store) ListChanges(ctx context.Context, vaultID string, since int64, limit int) (items []Item, vaultSeq int64, err error) {
	if err := s.pool.QueryRow(ctx, `SELECT seq FROM vaults WHERE id = $1`, vaultID).Scan(&vaultSeq); err != nil {
		return nil, 0, convertNoRowsToNotFound(err)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, revision, seq, deleted, overview, details, updated_at, updated_by
		FROM items WHERE vault_id = $1 AND seq > $2 ORDER BY seq LIMIT $3`, vaultID, since, limit)
	if err != nil {
		return nil, 0, err
	}
	items, err = pgx.CollectRows(rows, scanItem)
	return items, vaultSeq, err
}

// PutItem writes a new revision if baseRevision matches the stored one ("" for a new item).
// On ErrConflict the current item is returned; it is nil when the item does not exist.
// Re-sending an already stored revision is a no-op so that clients can retry safely.
func (s *Store) PutItem(ctx context.Context, vaultID, baseRevision string, item *Item) (*Item, error) {
	var current *Item
	err := s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		var vaultSeq int64
		if err := transaction.QueryRow(ctx, `SELECT seq FROM vaults WHERE id = $1 FOR UPDATE`, vaultID).Scan(&vaultSeq); err != nil {
			return convertNoRowsToNotFound(err)
		}

		rows, err := transaction.Query(ctx, `
			SELECT id, revision, seq, deleted, overview, details, updated_at, updated_by
			FROM items WHERE vault_id = $1 AND id = $2`, vaultID, item.ID)
		if err != nil {
			return err
		}
		existingItems, err := pgx.CollectRows(rows, scanItem)
		if err != nil {
			return err
		}

		switch {
		case len(existingItems) == 1 && existingItems[0].Revision == item.Revision:
			*item = existingItems[0]
			return nil
		case len(existingItems) == 1 && existingItems[0].Revision != baseRevision:
			current = &existingItems[0]
			return ErrConflict
		case len(existingItems) == 0 && baseRevision != "":
			return ErrConflict
		}

		if err := transaction.QueryRow(ctx, `UPDATE vaults SET seq = seq + 1 WHERE id = $1 RETURNING seq`, vaultID).Scan(&item.Seq); err != nil {
			return err
		}
		err = transaction.QueryRow(ctx, `
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
	})
	return current, err
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
