package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// ShareLink: MaxViews 0 is unlimited; PassphraseSalt is nil when the link has no passphrase.
type ShareLink struct {
	ID                   string
	AccountID            string
	Ciphertext           []byte
	EncryptedLabel       []byte
	PassphraseSalt       []byte
	PassphraseIterations int
	MaxViews             int
	ViewCount            int
	CreatedAt            time.Time
	ExpiresAt            time.Time
}

// CreateShareLink refuses with ErrConflict when the account already has maxLiveCount live links.
func (s *Store) CreateShareLink(ctx context.Context, link *ShareLink, ttl time.Duration, maxLiveCount int) error {
	return s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		if _, err := transaction.Exec(ctx, `SELECT 1 FROM accounts WHERE id = $1 FOR UPDATE`, link.AccountID); err != nil {
			return err
		}
		var liveCount int
		if err := transaction.QueryRow(ctx, `SELECT count(*) FROM share_links WHERE account_id = $1 AND expires_at > now()`, link.AccountID).Scan(&liveCount); err != nil {
			return err
		}
		if liveCount >= maxLiveCount {
			return ErrConflict
		}
		var passphraseIterations *int
		if link.PassphraseSalt != nil {
			passphraseIterations = &link.PassphraseIterations
		}
		err := transaction.QueryRow(ctx, `
			INSERT INTO share_links (id, account_id, ciphertext, encrypted_label, passphrase_salt, passphrase_iterations, max_views, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, now() + $8::interval)
			RETURNING created_at, expires_at`,
			link.ID, link.AccountID, link.Ciphertext, link.EncryptedLabel, link.PassphraseSalt, passphraseIterations, link.MaxViews, ttl,
		).Scan(&link.CreatedAt, &link.ExpiresAt)
		if isUniqueViolation(err) {
			return ErrExists
		}
		return err
	})
}

func (s *Store) ListAllShareLink(ctx context.Context, accountID string) ([]ShareLink, error) {
	allRow, err := s.pool.Query(ctx, `
		SELECT id, account_id, encrypted_label, passphrase_salt, coalesce(passphrase_iterations, 0), max_views, view_count, created_at, expires_at
		FROM share_links WHERE account_id = $1 AND expires_at > now()
		ORDER BY created_at DESC, id`, accountID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(allRow, func(row pgx.CollectableRow) (ShareLink, error) {
		var link ShareLink
		err := row.Scan(&link.ID, &link.AccountID, &link.EncryptedLabel, &link.PassphraseSalt, &link.PassphraseIterations, &link.MaxViews, &link.ViewCount, &link.CreatedAt, &link.ExpiresAt)
		return link, err
	})
}

// DeleteShareLink revokes one of the account's links; ErrNotFound covers another account's link too.
func (s *Store) DeleteShareLink(ctx context.Context, accountID, linkID string) error {
	commandTag, err := s.pool.Exec(ctx, `DELETE FROM share_links WHERE id = $1 AND account_id = $2 AND expires_at > now()`, linkID, accountID)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ViewShareLink counts one view; the row lock serializes concurrent views and the last allowed one deletes the link.
func (s *Store) ViewShareLink(ctx context.Context, linkID string) (*ShareLink, error) {
	var link ShareLink
	err := s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		err := transaction.QueryRow(ctx, `
			UPDATE share_links SET view_count = view_count + 1
			WHERE id = $1 AND expires_at > now() AND (max_views = 0 OR view_count < max_views)
			RETURNING id, account_id, ciphertext, passphrase_salt, coalesce(passphrase_iterations, 0), max_views, view_count, created_at, expires_at`,
			linkID,
		).Scan(&link.ID, &link.AccountID, &link.Ciphertext, &link.PassphraseSalt, &link.PassphraseIterations, &link.MaxViews, &link.ViewCount, &link.CreatedAt, &link.ExpiresAt)
		if err != nil {
			return convertNoRowsToNotFound(err)
		}
		if link.MaxViews > 0 && link.ViewCount >= link.MaxViews {
			_, err = transaction.Exec(ctx, `DELETE FROM share_links WHERE id = $1`, linkID)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return &link, nil
}

func (s *Store) DeleteAllExpiredShareLink(ctx context.Context) (int64, error) {
	commandTag, err := s.pool.Exec(ctx, `DELETE FROM share_links WHERE expires_at <= now()`)
	return commandTag.RowsAffected(), err
}
