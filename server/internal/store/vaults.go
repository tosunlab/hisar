package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type Vault struct {
	ID         string
	Type       string
	EncName    []byte
	Seq        int64
	CreatedBy  string
	CreatedAt  time.Time
	Role       string
	WrappedKey []byte
}

type Member struct {
	AccountID  string
	Role       string
	WrappedKey []byte
	GrantedBy  string
}

func (s *Store) CreateVault(ctx context.Context, v *Vault, ownerWrappedKey []byte) error {
	return s.runInTransaction(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO vaults (id, type, enc_name, created_by) VALUES ($1, $2, $3, $4)`,
			v.ID, v.Type, v.EncName, v.CreatedBy)
		if isUniqueViolation(err) {
			return ErrExists
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO vault_access (vault_id, account_id, wrapped_key, role, granted_by) VALUES ($1, $2, $3, 'owner', $2)`,
			v.ID, v.CreatedBy, ownerWrappedKey)
		return err
	})
}

func (s *Store) ListVaults(ctx context.Context, accountID string) ([]Vault, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT v.id, v.type, v.enc_name, v.seq, v.created_by, v.created_at, a.role, a.wrapped_key
		FROM vaults v JOIN vault_access a ON a.vault_id = v.id
		WHERE a.account_id = $1 ORDER BY v.created_at`, accountID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Vault, error) {
		var v Vault
		err := r.Scan(&v.ID, &v.Type, &v.EncName, &v.Seq, &v.CreatedBy, &v.CreatedAt, &v.Role, &v.WrappedKey)
		return v, err
	})
}

func (s *Store) VaultRole(ctx context.Context, vaultID, accountID string) (string, error) {
	var role string
	err := s.pool.QueryRow(ctx,
		`SELECT role FROM vault_access WHERE vault_id = $1 AND account_id = $2`, vaultID, accountID).Scan(&role)
	return role, convertNoRowsToNotFound(err)
}

func (s *Store) ListMembers(ctx context.Context, vaultID string) ([]Member, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT account_id, role, wrapped_key, granted_by FROM vault_access WHERE vault_id = $1 ORDER BY granted_at`, vaultID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Member, error) {
		var m Member
		err := r.Scan(&m.AccountID, &m.Role, &m.WrappedKey, &m.GrantedBy)
		return m, err
	})
}

func (s *Store) SetAccess(ctx context.Context, vaultID string, m Member) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO vault_access (vault_id, account_id, wrapped_key, role, granted_by) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (vault_id, account_id) DO UPDATE SET wrapped_key = $3, role = $4, granted_by = $5, granted_at = now()`,
		vaultID, m.AccountID, m.WrappedKey, m.Role, m.GrantedBy)
	return err
}

func (s *Store) RemoveAccess(ctx context.Context, vaultID, accountID string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM vault_access WHERE vault_id = $1 AND account_id = $2`, vaultID, accountID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
