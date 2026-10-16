package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// AccountPreferences is an account's sealed preferences record; Revision 0 means the account has none yet.
type AccountPreferences struct {
	Revision   int64
	Ciphertext []byte
}

func (s *Store) GetAccountPreferences(ctx context.Context, accountID string) (AccountPreferences, error) {
	var preferences AccountPreferences
	err := s.pool.QueryRow(ctx, `SELECT revision, ciphertext FROM account_preferences WHERE account_id = $1`, accountID).
		Scan(&preferences.Revision, &preferences.Ciphertext)
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountPreferences{}, nil
	}
	return preferences, err
}

// PutAccountPreferences stores the next revision, refusing with ErrConflict unless baseRevision is the current one.
func (s *Store) PutAccountPreferences(ctx context.Context, accountID string, baseRevision int64, ciphertext []byte) (int64, error) {
	var revision int64
	var err error
	if baseRevision == 0 {
		err = s.pool.QueryRow(ctx, `
			INSERT INTO account_preferences (account_id, revision, ciphertext) VALUES ($1, 1, $2)
			ON CONFLICT (account_id) DO NOTHING
			RETURNING revision`, accountID, ciphertext).Scan(&revision)
	} else {
		err = s.pool.QueryRow(ctx, `
			UPDATE account_preferences SET revision = revision + 1, ciphertext = $3, updated_at = now()
			WHERE account_id = $1 AND revision = $2
			RETURNING revision`, accountID, baseRevision, ciphertext).Scan(&revision)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrConflict
	}
	return revision, err
}
