package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrOrganizationVault  = errors.New("store: access to an organization vault comes from its teams")
	ErrNotEntitled        = errors.New("store: the account's teams do not give it the vault with this role")
	ErrRoleFromTeams      = errors.New("store: roles in an organization vault come from its teams")
	ErrStaleKeyGeneration = errors.New("store: not the vault's current key generation")
)

type Vault struct {
	ID        string
	Type      string
	EncName   []byte
	Seq       int64
	CreatedBy string
	CreatedAt time.Time
	// OrganizationID is empty except for organization vaults.
	OrganizationID string
	KeyGeneration  int64
	NeedsRotation  bool
	Role           string
	WrappedKey     []byte
}

type Member struct {
	AccountID  string
	Role       string
	WrappedKey []byte
	GrantedBy  string
}

func (s *Store) CreateVault(ctx context.Context, vault *Vault, ownerWrappedKey []byte) error {
	return s.runInTransaction(ctx, func(transaction pgx.Tx) error { return insertVault(ctx, transaction, vault, ownerWrappedKey) })
}

func insertVault(ctx context.Context, transaction pgx.Tx, vault *Vault, ownerWrappedKey []byte) error {
	_, err := transaction.Exec(ctx,
		`INSERT INTO vaults (id, type, enc_name, created_by, organization_id) VALUES ($1, $2, $3, $4, NULLIF($5, '')::uuid)`,
		vault.ID, vault.Type, vault.EncName, vault.CreatedBy, vault.OrganizationID)
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
}

func (s *Store) ListVaults(ctx context.Context, accountID string) ([]Vault, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT v.id, v.type, v.enc_name, v.seq, v.created_by, v.created_at, coalesce(v.organization_id::text, ''), v.key_generation,
			v.needs_rotation, a.role, a.wrapped_key
		FROM vaults v JOIN vault_access a ON a.vault_id = v.id
		WHERE a.account_id = $1 ORDER BY v.created_at`, accountID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Vault, error) {
		var vault Vault
		err := row.Scan(&vault.ID, &vault.Type, &vault.EncName, &vault.Seq, &vault.CreatedBy, &vault.CreatedAt, &vault.OrganizationID,
			&vault.KeyGeneration, &vault.NeedsRotation, &vault.Role, &vault.WrappedKey)
		return vault, err
	})
}

func (s *Store) GetVaultType(ctx context.Context, vaultID string) (string, error) {
	var vaultType string
	err := s.pool.QueryRow(ctx, `SELECT type FROM vaults WHERE id = $1`, vaultID).Scan(&vaultType)
	return vaultType, convertNoRowsToNotFound(err)
}

// GetVaultOrganizationID returns the organization of an organization vault, and "" for any other vault.
func (s *Store) GetVaultOrganizationID(ctx context.Context, vaultID string) (string, error) {
	var organizationID string
	err := s.pool.QueryRow(ctx, `SELECT coalesce(organization_id::text, '') FROM vaults WHERE id = $1`, vaultID).Scan(&organizationID)
	return organizationID, convertNoRowsToNotFound(err)
}

func (s *Store) GetVaultRole(ctx context.Context, vaultID, accountID string) (string, error) {
	role, err := queryVaultRole(ctx, s.pool, vaultID, accountID)
	if err == nil && role == "" {
		return "", ErrNotFound
	}
	return role, err
}

// queryVaultRole returns "" when the account has no access to the vault.
func queryVaultRole(ctx context.Context, database queryer, vaultID, accountID string) (string, error) {
	var role string
	err := database.QueryRow(ctx, `SELECT role FROM vault_access WHERE vault_id = $1 AND account_id = $2`, vaultID, accountID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return role, err
}

func requireVaultOwner(ctx context.Context, transaction pgx.Tx, vaultID, accountID string) error {
	role, err := queryVaultRole(ctx, transaction, vaultID, accountID)
	switch {
	case err != nil:
		return err
	case role == "":
		return ErrNotFound
	case role != "owner":
		return ErrNotOwner
	}
	return nil
}

// lockedVault is a vault whose row the transaction holds.
type lockedVault struct {
	vaultType      string
	organizationID string
	seq            int64
	keyGeneration  int64
}

// lockVault locks the vault's organization, when it has one, and then the vault: the order of every change to an
// organization (§5.11), so that access is never checked against a state another transaction is changing.
func lockVault(ctx context.Context, transaction pgx.Tx, vaultID string) (*lockedVault, error) {
	vaultsByID, err := lockVaults(ctx, transaction, []string{vaultID})
	if err != nil {
		return nil, err
	}
	vault, isFound := vaultsByID[vaultID]
	if !isFound {
		return nil, ErrNotFound
	}
	return vault, nil
}

// lockVaults locks the organizations of the vaults and then the vaults, each in ID order, as lockVault does for one. A
// vault's organization never changes, so it is read before the locks. A vault that does not exist is not in the result.
func lockVaults(ctx context.Context, transaction pgx.Tx, vaultIDs []string) (map[string]*lockedVault, error) {
	rows, err := transaction.Query(ctx, `
		SELECT organization_id::text FROM vaults WHERE id = ANY($1::uuid[]) AND organization_id IS NOT NULL
		GROUP BY organization_id ORDER BY organization_id`, vaultIDs)
	if err != nil {
		return nil, err
	}
	organizationIDs, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	for _, organizationID := range organizationIDs {
		if err := lockOrganization(ctx, transaction, organizationID); err != nil {
			return nil, err
		}
	}
	rows, err = transaction.Query(ctx, `
		SELECT id::text, type, coalesce(organization_id::text, ''), seq, key_generation FROM vaults
		WHERE id = ANY($1::uuid[]) ORDER BY id FOR UPDATE`, vaultIDs)
	if err != nil {
		return nil, err
	}
	vaultsByID := map[string]*lockedVault{}
	var vaultID string
	var scanned lockedVault
	_, err = pgx.ForEachRow(rows, []any{&vaultID, &scanned.vaultType, &scanned.organizationID, &scanned.seq, &scanned.keyGeneration}, func() error {
		vault := scanned
		vaultsByID[vaultID] = &vault
		return nil
	})
	return vaultsByID, err
}

// acceptsKeyGeneration reports whether something sealed with the vault key of keyGeneration may be stored: the vault's
// current generation is required for organization vaults and checked for any vault when nonzero, so older clients keep
// working with personal and shared vaults, and nothing sealed with a key a rotation replaced is stored.
func (vault *lockedVault) acceptsKeyGeneration(keyGeneration int64) bool {
	return keyGeneration == vault.keyGeneration || keyGeneration == 0 && vault.organizationID == ""
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

// GrantAccess stores a member's wrapped key; member.GrantedBy must be an owner (ErrNotOwner). A shared vault adds an
// account only when it accepted a share invitation, consuming it (ErrNoInvitation), and otherwise changes an existing
// member's role. An organization vault adds only an account its teams entitle to exactly that role (ErrNotEntitled) and
// never changes a member's role (ErrRoleFromTeams). A nonzero keyGeneration, which organization vaults require, must be
// the vault's current one (ErrStaleKeyGeneration).
func (s *Store) GrantAccess(ctx context.Context, vaultID string, member Member, keyGeneration int64) (isNewMember bool, err error) {
	err = s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		vault, err := lockVault(ctx, transaction, vaultID)
		if err != nil {
			return err
		}
		if err := requireVaultOwner(ctx, transaction, vaultID, member.GrantedBy); err != nil {
			return err
		}
		if !vault.acceptsKeyGeneration(keyGeneration) {
			return ErrStaleKeyGeneration
		}
		currentRole, err := queryVaultRole(ctx, transaction, vaultID, member.AccountID)
		if err != nil {
			return err
		}
		isNewMember = currentRole == ""
		switch {
		case vault.organizationID != "" && !isNewMember:
			if member.Role != currentRole {
				return ErrRoleFromTeams
			}
		case vault.organizationID != "":
			entitledRole, err := queryEntitledRole(ctx, transaction, vaultID, member.AccountID)
			if err != nil {
				return err
			}
			if entitledRole != member.Role {
				return ErrNotEntitled
			}
		case isNewMember:
			commandTag, err := transaction.Exec(ctx,
				`DELETE FROM share_invitations WHERE vault_id = $1 AND accepted_by = $2 AND state = 'accepted'`, vaultID, member.AccountID)
			if err != nil {
				return err
			}
			if commandTag.RowsAffected() == 0 {
				return ErrNoInvitation
			}
		}
		_, err = transaction.Exec(ctx, `
			INSERT INTO vault_access (vault_id, account_id, wrapped_key, role, granted_by) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (vault_id, account_id) DO UPDATE SET wrapped_key = $3, role = $4, granted_by = $5, granted_at = now()`,
			vaultID, member.AccountID, member.WrappedKey, member.Role, member.GrantedBy)
		return err
	})
	return isNewMember, err
}

// RemoveAccess removes a member and their wrapped key. Access to an organization vault comes from its teams
// (ErrOrganizationVault).
func (s *Store) RemoveAccess(ctx context.Context, vaultID, accountID string) error {
	return s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		vault, err := lockVault(ctx, transaction, vaultID)
		if err != nil {
			return err
		}
		if vault.organizationID != "" {
			return ErrOrganizationVault
		}
		commandTag, err := transaction.Exec(ctx, `DELETE FROM vault_access WHERE vault_id = $1 AND account_id = $2`, vaultID, accountID)
		if err != nil {
			return err
		}
		if commandTag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}
