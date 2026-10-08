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

func scanItem(r pgx.CollectableRow) (Item, error) {
	var it Item
	err := r.Scan(&it.ID, &it.Revision, &it.Seq, &it.Deleted, &it.Overview, &it.Details, &it.UpdatedAt, &it.UpdatedBy)
	return it, err
}

// Changes returns items with seq > since, and the vault's current seq.
func (s *Store) Changes(ctx context.Context, vaultID string, since int64, limit int) (items []Item, vaultSeq int64, err error) {
	if err := s.pool.QueryRow(ctx, `SELECT seq FROM vaults WHERE id = $1`, vaultID).Scan(&vaultSeq); err != nil {
		return nil, 0, notFound(err)
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
func (s *Store) PutItem(ctx context.Context, vaultID, baseRevision string, it *Item) (*Item, error) {
	var current *Item
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var vaultSeq int64
		if err := tx.QueryRow(ctx, `SELECT seq FROM vaults WHERE id = $1 FOR UPDATE`, vaultID).Scan(&vaultSeq); err != nil {
			return notFound(err)
		}

		rows, err := tx.Query(ctx, `
			SELECT id, revision, seq, deleted, overview, details, updated_at, updated_by
			FROM items WHERE vault_id = $1 AND id = $2`, vaultID, it.ID)
		if err != nil {
			return err
		}
		existing, err := pgx.CollectRows(rows, scanItem)
		if err != nil {
			return err
		}

		switch {
		case len(existing) == 1 && existing[0].Revision == it.Revision:
			*it = existing[0]
			return nil
		case len(existing) == 1 && existing[0].Revision != baseRevision:
			current = &existing[0]
			return ErrConflict
		case len(existing) == 0 && baseRevision != "":
			return ErrConflict
		}

		if err := tx.QueryRow(ctx, `UPDATE vaults SET seq = seq + 1 WHERE id = $1 RETURNING seq`, vaultID).Scan(&it.Seq); err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO items (vault_id, id, revision, seq, deleted, overview, details, updated_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (vault_id, id) DO UPDATE SET
				revision = $3, seq = $4, deleted = $5, overview = $6, details = $7, updated_by = $8, updated_at = now()
			RETURNING updated_at`,
			vaultID, it.ID, it.Revision, it.Seq, it.Deleted, it.Overview, it.Details, it.UpdatedBy).Scan(&it.UpdatedAt)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO item_history (vault_id, id, revision, deleted, overview, details, updated_at, updated_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			vaultID, it.ID, it.Revision, it.Deleted, it.Overview, it.Details, it.UpdatedAt, it.UpdatedBy)
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return err
	})
	return current, err
}

func (s *Store) ItemHistory(ctx context.Context, vaultID, itemID string) ([]Item, error) {
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
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM item_history h WHERE h.updated_at < now() - $1::interval
		AND NOT EXISTS (SELECT 1 FROM items i WHERE i.vault_id = h.vault_id AND i.id = h.id AND i.revision = h.revision)`,
		maxAge)
	return tag.RowsAffected(), err
}
