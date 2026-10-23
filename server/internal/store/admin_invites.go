package store

import (
	"context"
	"errors"
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

// CreateAdminInvite requires fresh proven authority for admin-role invites.
func (s *Store) CreateAdminInvite(ctx context.Context, tokenHash []byte, email, role string, actor AdminActor, expiresAt time.Time) error {
	return s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		if err := requireInviteAdmin(ctx, transaction, actor, role); err != nil {
			return err
		}
		_, err := transaction.Exec(ctx, `INSERT INTO invites (token_hash, email, role, created_by, expires_at) VALUES ($1, $2, $3, $4, $5)`,
			tokenHash, email, role, actor.AccountID, expiresAt)
		return err
	})
}

// ReplaceInvite deletes the unused invite with an email of oldTokenHash and creates one for the same email and role
// under newTokenHash, in one transaction, and returns the new invite; ErrNotFound when the old one is gone or used, and
// ErrActorNotAdmin unless actor is still an active admin.
func (s *Store) ReplaceInvite(ctx context.Context, oldTokenHash, newTokenHash []byte, actor AdminActor, expiresAt time.Time) (*AdminInvite, error) {
	var replaced *AdminInvite
	err := pgx.BeginFunc(ctx, s.pool, func(transaction pgx.Tx) error {
		var role string
		if err := transaction.QueryRow(ctx, `SELECT role FROM invites WHERE token_hash = $1`, oldTokenHash).Scan(&role); err != nil {
			return convertNoRowsToNotFound(err)
		}
		if err := requireInviteAdmin(ctx, transaction, actor, role); err != nil {
			return err
		}
		if err := lockInviteSource(ctx, transaction, oldTokenHash); err != nil {
			return err
		}
		var email string
		var shareSource, organizationSource *string
		err := transaction.QueryRow(ctx, `DELETE FROM invites WHERE token_hash = $1 AND used_at IS NULL AND email <> ''
			RETURNING email, role, share_invitation_id::text, organization_invitation_id::text`,
			oldTokenHash).Scan(&email, &role, &shareSource, &organizationSource)
		if err != nil {
			return convertNoRowsToNotFound(err)
		}
		_, err = transaction.Exec(ctx, `INSERT INTO invites (token_hash, email, role, created_by, expires_at, share_invitation_id, organization_invitation_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			newTokenHash, email, role, actor.AccountID, expiresAt, shareSource, organizationSource)
		if err != nil {
			return err
		}
		replaced, err = scanAdminInvite(transaction.QueryRow(ctx, `SELECT `+adminInviteColumns+` WHERE i.token_hash = $1`, newTokenHash))
		return err
	})
	return replaced, err
}

func requireInviteAdmin(ctx context.Context, transaction pgx.Tx, actor AdminActor, role string) error {
	if role == "admin" {
		if len(actor.ProvenRecordHash) == 0 {
			return ErrConflict
		}
		if _, _, _, err := lockProvenAccount(ctx, transaction, actor.AccountID, actor.TokenHash, actor.ProvenRecordHash); errors.Is(err, ErrNotFound) {
			return ErrSessionEnded
		} else if err != nil {
			return err
		}
	}
	return requireActiveAdmin(ctx, transaction, actor.AccountID)
}

// Source deletion locks its row before invalidating tokens. Keep the same order in redemption and resend.
func lockInviteSource(ctx context.Context, transaction pgx.Tx, tokenHash []byte) error {
	var shareSource, organizationSource *string
	if err := transaction.QueryRow(ctx, `SELECT share_invitation_id::text, organization_invitation_id::text FROM invites WHERE token_hash = $1`,
		tokenHash).Scan(&shareSource, &organizationSource); err != nil {
		return convertNoRowsToNotFound(err)
	}
	var id string
	if shareSource != nil {
		return convertNoRowsToNotFound(transaction.QueryRow(ctx, `SELECT id::text FROM share_invitations WHERE id = $1 FOR SHARE`, *shareSource).Scan(&id))
	}
	if organizationSource != nil {
		return convertNoRowsToNotFound(transaction.QueryRow(ctx, `SELECT id::text FROM organization_invitations WHERE id = $1 FOR SHARE`, *organizationSource).Scan(&id))
	}
	return nil
}
