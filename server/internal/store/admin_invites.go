package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// AdminInvite is a sign-up invite with an email as the admin dashboard sees it; setup invites have no email and are never
// one.
type AdminInvite struct {
	TokenHash []byte
	Email     string
	Role      string
	// CreatedBy is nil for an operator's invite; CreatorName and CreatorEmail are then empty.
	CreatedBy    *string
	CreatorName  string
	CreatorEmail string
	CreatedAt    time.Time
	ExpiresAt    time.Time
	UsedAt       *time.Time
}

const adminInviteColumns = `i.token_hash, i.email, i.role, i.created_by, coalesce(a.name, ''), coalesce(a.email, ''), i.created_at, i.expires_at, i.used_at
	FROM invites i LEFT JOIN accounts a ON a.id = i.created_by`

func scanAdminInvite(row pgx.Row) (*AdminInvite, error) {
	var invite AdminInvite
	err := row.Scan(&invite.TokenHash, &invite.Email, &invite.Role, &invite.CreatedBy, &invite.CreatorName, &invite.CreatorEmail,
		&invite.CreatedAt, &invite.ExpiresAt, &invite.UsedAt)
	if err != nil {
		return nil, convertNoRowsToNotFound(err)
	}
	return &invite, nil
}

// ListUnusedInvites returns the invites with an email that nobody signed up with, newest first, including expired
// ones that are not pruned yet. Admin invites whose issuer lost authority are hidden until it returns.
func (s *Store) ListUnusedInvites(ctx context.Context) ([]AdminInvite, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+adminInviteColumns+`
		WHERE i.used_at IS NULL AND i.email <> '' AND `+isWithinCreatorAuthority+` ORDER BY i.created_at DESC, i.token_hash`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (AdminInvite, error) {
		invite, err := scanAdminInvite(row)
		if err != nil {
			return AdminInvite{}, err
		}
		return *invite, nil
	})
}

// GetAdminInvite returns the invite with an email of tokenHash, used or not. An unused invite whose issuer lacks
// authority is ErrNotFound, just as on the invitation page.
func (s *Store) GetAdminInvite(ctx context.Context, tokenHash []byte) (*AdminInvite, error) {
	return scanAdminInvite(s.pool.QueryRow(ctx, `SELECT `+adminInviteColumns+`
		WHERE i.token_hash = $1 AND i.email <> '' AND (i.used_at IS NOT NULL OR `+isWithinCreatorAuthority+`)`, tokenHash))
}

// DeleteUnusedInvite deletes the unused invite with an email of tokenHash; ErrNotFound when there is none.
func (s *Store) DeleteUnusedInvite(ctx context.Context, tokenHash []byte) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM invites WHERE token_hash = $1 AND used_at IS NULL AND email <> ''`, tokenHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateAdminInvite creates an invite from the admin adminID, who must still be an active admin (ErrActorNotAdmin).
func (s *Store) CreateAdminInvite(ctx context.Context, tokenHash []byte, email, role, adminID string, expiresAt time.Time) error {
	return s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		if err := requireActiveAdmin(ctx, transaction, adminID); err != nil {
			return err
		}
		_, err := transaction.Exec(ctx, `INSERT INTO invites (token_hash, email, role, created_by, expires_at) VALUES ($1, $2, $3, $4, $5)`,
			tokenHash, email, role, adminID, expiresAt)
		return err
	})
}

// ReplaceInvite deletes the unused invite with an email of oldTokenHash and creates one for the same email and role
// under newTokenHash, in one transaction, and returns the new invite; ErrNotFound when the old one is gone or used, and
// ErrActorNotAdmin unless createdBy is still an active admin.
func (s *Store) ReplaceInvite(ctx context.Context, oldTokenHash, newTokenHash []byte, createdBy string, expiresAt time.Time) (*AdminInvite, error) {
	var replaced *AdminInvite
	err := pgx.BeginFunc(ctx, s.pool, func(transaction pgx.Tx) error {
		if err := requireActiveAdmin(ctx, transaction, createdBy); err != nil {
			return err
		}
		var email, role string
		err := transaction.QueryRow(ctx, `DELETE FROM invites WHERE token_hash = $1 AND used_at IS NULL AND email <> '' RETURNING email, role`,
			oldTokenHash).Scan(&email, &role)
		if err != nil {
			return convertNoRowsToNotFound(err)
		}
		_, err = transaction.Exec(ctx, `INSERT INTO invites (token_hash, email, role, created_by, expires_at) VALUES ($1, $2, $3, $4, $5)`,
			newTokenHash, email, role, createdBy, expiresAt)
		if err != nil {
			return err
		}
		replaced, err = scanAdminInvite(transaction.QueryRow(ctx, `SELECT `+adminInviteColumns+` WHERE i.token_hash = $1`, newTokenHash))
		return err
	})
	return replaced, err
}
