package store

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// ListChangesSnapshot binds a bounded page and its generation to one database snapshot.
func (s *Store) ListChangesSnapshot(ctx context.Context, accountID, vaultID string, since int64, limit int, maxBytes int64) (items []Item, sequence, generation int64, more bool, err error) {
	err = pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(transaction pgx.Tx) error {
		if err := transaction.QueryRow(ctx, `SELECT v.key_generation FROM vaults v JOIN vault_access a ON a.vault_id = v.id WHERE v.id = $1 AND a.account_id = $2`, vaultID, accountID).Scan(&generation); err != nil {
			return convertNoRowsToNotFound(err)
		}
		var readErr error
		items, sequence, more, readErr = listChanges(ctx, transaction, vaultID, since, limit, maxBytes)
		return readErr
	})
	return
}
