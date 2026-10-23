package store

import (
	"bytes"
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// CanCreateSecrets is false even after the last account is deleted if this database
// has already enrolled authentication keys.
func (s *Store) CanCreateSecrets(ctx context.Context) (bool, error) {
	var allowed bool
	err := s.pool.QueryRow(ctx, `SELECT NOT EXISTS (SELECT 1 FROM accounts) AND NOT EXISTS
		(SELECT 1 FROM operational_secrets WHERE fingerprint IS NOT NULL)`).Scan(&allowed)
	return allowed, err
}

// InitializeOperationalSecrets runs before settings are read or requests are served.
// It enrolls the secrets fingerprint under one singleton lock and refuses a different one afterwards.
func (s *Store) InitializeOperationalSecrets(ctx context.Context, fingerprint []byte) error {
	if len(fingerprint) != 32 {
		return errors.New("store: invalid secrets fingerprint")
	}
	return s.runInTransaction(ctx, func(tx pgx.Tx) error {
		var enrolled []byte
		if err := tx.QueryRow(ctx, `SELECT fingerprint FROM operational_secrets WHERE id = 1 FOR UPDATE`).Scan(&enrolled); err != nil {
			return err
		}
		if enrolled != nil && !bytes.Equal(enrolled, fingerprint) {
			return errors.New("store: server secrets do not match this database; restore the matching database and hisar-secrets.json backup together, without replacing or deleting either")
		}
		_, err := tx.Exec(ctx, `UPDATE operational_secrets SET fingerprint = $1 WHERE id = 1`, fingerprint)
		return err
	})
}
