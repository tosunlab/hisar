package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// GetInstanceSettings returns the stored settings document, or nil before an admin first saves one.
func (s *Store) GetInstanceSettings(ctx context.Context) ([]byte, error) {
	var document []byte
	err := s.pool.QueryRow(ctx, `SELECT settings FROM instance_settings WHERE id = 1`).Scan(&document)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return document, err
}

// UpdateInstanceSettings replaces the settings document with what update makes of the current one, holding the row
// locked so concurrent saves apply one after the other. An admin's change (isByAdmin) needs updatedBy to still be an
// active admin (ErrActorNotAdmin); setup's own changes do not. An error from update leaves the row unchanged.
func (s *Store) UpdateInstanceSettings(ctx context.Context, updatedBy string, isByAdmin bool, update func(document []byte) ([]byte, error)) error {
	return s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		if isByAdmin {
			if err := requireActiveAdmin(ctx, transaction, updatedBy); err != nil {
				return err
			}
		}
		if _, err := transaction.Exec(ctx, `INSERT INTO instance_settings (id, settings) VALUES (1, '{}') ON CONFLICT (id) DO NOTHING`); err != nil {
			return err
		}
		var document []byte
		if err := transaction.QueryRow(ctx, `SELECT settings FROM instance_settings WHERE id = 1 FOR UPDATE`).Scan(&document); err != nil {
			return err
		}
		updated, err := update(document)
		if err != nil {
			return err
		}
		_, err = transaction.Exec(ctx, `UPDATE instance_settings SET settings = $1, updated_at = now(), updated_by = $2 WHERE id = 1`, updated, updatedBy)
		return err
	})
}
