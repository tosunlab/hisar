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

func (s *Store) CreateVault(ctx context.Context, vault *Vault, ownerWrappedKey []byte) error {
	return s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		_, err := transaction.Exec(ctx,
			`INSERT INTO vaults (id, type, enc_name, created_by) VALUES ($1, $2, $3, $4)`,
			vault.ID, vault.Type, vault.EncName, vault.CreatedBy)
		if isUniqueViolation(err) {
			return ErrExists
		}
		if err != nil {
			return err
		}
		_, err = transaction.Exec(ctx,
			`INSERT INTO vault_access (vault_id, account_id, wrapped_key, role, granted_by) VALUES ($1, $2, $3, 'owner', $2)`,
			vault.ID, vault.CreatedBy, ownerWrappedKey)
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
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Vault, error) {
		var vault Vault
		err := row.Scan(&vault.ID, &vault.Type, &vault.EncName, &vault.Seq, &vault.CreatedBy, &vault.CreatedAt, &vault.Role, &vault.WrappedKey)
		return vault, err
	})
}

func (s *Store) GetVaultType(ctx context.Context, vaultID string) (string, error) {
	var vaultType string
	err := s.pool.QueryRow(ctx, `SELECT type FROM vaults WHERE id = $1`, vaultID).Scan(&vaultType)
	return vaultType, convertNoRowsToNotFound(err)
}

func (s *Store) GetVaultRole(ctx context.Context, vaultID, accountID string) (string, error) {
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
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Member, error) {
		var member Member
		err := row.Scan(&member.AccountID, &member.Role, &member.WrappedKey, &member.GrantedBy)
		return member, err
	})
}

func (s *Store) SetAccess(ctx context.Context, vaultID string, member Member) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO vault_access (vault_id, account_id, wrapped_key, role, granted_by) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (vault_id, account_id) DO UPDATE SET wrapped_key = $3, role = $4, granted_by = $5, granted_at = now()`,
		vaultID, member.AccountID, member.WrappedKey, member.Role, member.GrantedBy)
	return err
}

// GrantAccess changes an existing member's access, or adds an account that accepted a share invitation to the vault,
// consuming the invitation. It returns ErrNoInvitation otherwise.
func (s *Store) GrantAccess(ctx context.Context, vaultID string, member Member) (isNewMember bool, err error) {
	err = s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		var isMember bool
		if err := transaction.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM vault_access WHERE vault_id = $1 AND account_id = $2)`, vaultID, member.AccountID).Scan(&isMember); err != nil {
			return err
		}
		if !isMember {
			commandTag, err := transaction.Exec(ctx,
				`DELETE FROM share_invitations WHERE vault_id = $1 AND accepted_by = $2 AND state = 'accepted'`, vaultID, member.AccountID)
			if err != nil {
				return err
			}
			if commandTag.RowsAffected() == 0 {
				return ErrNoInvitation
			}
			isNewMember = true
		}
		_, err := transaction.Exec(ctx, `
			INSERT INTO vault_access (vault_id, account_id, wrapped_key, role, granted_by) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (vault_id, account_id) DO UPDATE SET wrapped_key = $3, role = $4, granted_by = $5, granted_at = now()`,
			vaultID, member.AccountID, member.WrappedKey, member.Role, member.GrantedBy)
		return err
	})
	return isNewMember, err
}

func (s *Store) RemoveAccess(ctx context.Context, vaultID, accountID string) error {
	commandTag, err := s.pool.Exec(ctx, `DELETE FROM vault_access WHERE vault_id = $1 AND account_id = $2`, vaultID, accountID)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
