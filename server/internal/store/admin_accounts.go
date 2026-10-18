package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrAccountSuspended = errors.New("store: the account is suspended")
	ErrLastServerAdmin  = errors.New("store: the server's last active admin")
	ErrActorNotAdmin    = errors.New("store: the acting account is not an active admin")
	ErrSessionEnded     = errors.New("store: the acting session ended")
)

// AdminAccount is an account as the admin dashboard lists it. SuspendedAt is nil for an active account; LastSeenAt is
// the latest activity of any of its devices, nil when it has none.
type AdminAccount struct {
	ID          string
	Email       string
	Name        string
	Role        string
	SuspendedAt *time.Time
	CreatedAt   time.Time
	LastSeenAt  *time.Time
	DeviceCount int
	VaultCount  int
}

// AdminDevice is a device of an account, revoked ones included.
type AdminDevice struct {
	ID         string
	Name       string
	Platform   string
	CreatedAt  time.Time
	LastSeenAt time.Time
	RevokedAt  *time.Time
}

const adminAccountColumns = `a.id, a.email, a.name, a.role, a.suspended_at, a.created_at,
	(SELECT max(d.last_seen_at) FROM devices d WHERE d.account_id = a.id),
	(SELECT count(*) FROM devices d WHERE d.account_id = a.id AND d.revoked_at IS NULL),
	(SELECT count(*) FROM vault_access v WHERE v.account_id = a.id)`

func scanAdminAccount(row pgx.CollectableRow) (AdminAccount, error) {
	var account AdminAccount
	err := row.Scan(&account.ID, &account.Email, &account.Name, &account.Role, &account.SuspendedAt, &account.CreatedAt,
		&account.LastSeenAt, &account.DeviceCount, &account.VaultCount)
	return account, err
}

// ListAdminAccounts returns up to limit accounts sorted by email after the email afterEmail, keeping those whose email
// or name contains query, ignoring case.
func (s *Store) ListAdminAccounts(ctx context.Context, query, afterEmail string, limit int) ([]AdminAccount, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+adminAccountColumns+` FROM accounts a
		WHERE a.email > $2 AND ($1 = '' OR strpos(lower(a.email), lower($1)) > 0 OR strpos(lower(a.name), lower($1)) > 0)
		ORDER BY a.email LIMIT $3`, query, afterEmail, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanAdminAccount)
}

func (s *Store) GetAdminAccount(ctx context.Context, accountID string) (*AdminAccount, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+adminAccountColumns+` FROM accounts a WHERE a.id = $1`, accountID)
	if err != nil {
		return nil, err
	}
	account, err := pgx.CollectExactlyOneRow(rows, scanAdminAccount)
	if err != nil {
		return nil, convertNoRowsToNotFound(err)
	}
	return &account, nil
}

// ListAdminDevices returns every device of the account, revoked ones included: signed-in devices first, then by last
// activity.
func (s *Store) ListAdminDevices(ctx context.Context, accountID string) ([]AdminDevice, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, platform, created_at, last_seen_at, revoked_at FROM devices
		WHERE account_id = $1 ORDER BY revoked_at IS NOT NULL, last_seen_at DESC`, accountID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (AdminDevice, error) {
		var device AdminDevice
		err := row.Scan(&device.ID, &device.Name, &device.Platform, &device.CreatedAt, &device.LastSeenAt, &device.RevokedAt)
		return device, err
	})
}

// adminTarget is an account an admin changes, locked with every active admin.
type adminTarget struct {
	email                 string
	name                  string
	role                  string
	isActiveAdmin         bool
	isSuspended           bool
	otherActiveAdminCount int
}

// leavesNoAdmin reports whether taking the account's active admin role away leaves the server without one.
func (target adminTarget) leavesNoAdmin() bool {
	return target.isActiveAdmin && target.otherActiveAdminCount == 0
}

// AdminActor is the admin who makes a change. A change that needed a re-authentication carries its session's token hash
// and the hash of the OPAQUE record the proof was checked against; ProvenRecordHash is nil for one that did not.
type AdminActor struct {
	AccountID        string
	TokenHash        []byte
	ProvenRecordHash []byte
}

// lockAdminTarget locks the account and every active admin in ID order, so that two admins who change each other at
// once cannot both pass the check for a last active admin. FOR NO KEY UPDATE leaves the rows' key-share locks free, so
// a vault grant to one of them that holds a vault this change waits for can still finish. It returns ErrLastServerAdmin when taking the account's admin
// role away would leave no active admin, then ErrActorNotAdmin when the acting account is no longer an active admin, and
// for a proven change ErrConflict when the admin's OPAQUE record changed after the proof or ErrSessionEnded when the
// admin's session ended.
func lockAdminTarget(ctx context.Context, transaction pgx.Tx, actor AdminActor, accountID string, takesAdminRoleAway bool) (*adminTarget, error) {
	rows, err := transaction.Query(ctx, `
		SELECT id, email, name, role, role = 'admin' AND suspended_at IS NULL, suspended_at IS NOT NULL FROM accounts
		WHERE id = $1 OR (role = 'admin' AND suspended_at IS NULL) ORDER BY id FOR NO KEY UPDATE`, accountID)
	if err != nil {
		return nil, err
	}
	var target *adminTarget
	otherActiveAdminCount := 0
	isActorActiveAdmin := false
	var id, email, name, role string
	var isActiveAdmin, isSuspended bool
	if _, err := pgx.ForEachRow(rows, []any{&id, &email, &name, &role, &isActiveAdmin, &isSuspended}, func() error {
		if id == accountID {
			target = &adminTarget{email: email, name: name, role: role, isActiveAdmin: isActiveAdmin, isSuspended: isSuspended}
		} else if isActiveAdmin {
			otherActiveAdminCount++
		}
		if id == actor.AccountID && isActiveAdmin {
			isActorActiveAdmin = true
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if target == nil {
		return nil, ErrNotFound
	}
	target.otherActiveAdminCount = otherActiveAdminCount
	if takesAdminRoleAway && target.leavesNoAdmin() {
		return nil, ErrLastServerAdmin
	}
	if !isActorActiveAdmin {
		return nil, ErrActorNotAdmin
	}
	if actor.ProvenRecordHash != nil {
		// The admin's row is already locked above, as an active admin.
		_, _, _, err := lockProvenAccount(ctx, transaction, actor.AccountID, actor.TokenHash, actor.ProvenRecordHash)
		if errors.Is(err, ErrNotFound) {
			return nil, ErrSessionEnded
		}
		if err != nil {
			return nil, err
		}
	}
	return target, nil
}

// SuspendAccount marks the account suspended. Its devices and sessions stay, but GetSessionByHash refuses them with
// ErrAccountSuspended until it is reactivated; its vaults and organizations stay, and its share links stop opening. It
// reports false, changing nothing, for a suspended account.
func (s *Store) SuspendAccount(ctx context.Context, actorAccountID, accountID string) (suspended bool, err error) {
	err = s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		target, err := lockAdminTarget(ctx, transaction, AdminActor{AccountID: actorAccountID}, accountID, true)
		if err != nil || target.isSuspended {
			return err
		}
		suspended = true
		_, err = transaction.Exec(ctx, `UPDATE accounts SET suspended_at = now() WHERE id = $1`, accountID)
		return err
	})
	return suspended, err
}

// lockActorAndAccount locks the acting admin's account and the account they change, in ID order like lockAdminTarget. It
// returns ErrNotFound for a missing account, then ErrActorNotAdmin when the acting account is no longer an active admin,
// and otherwise whether the account is suspended.
func lockActorAndAccount(ctx context.Context, transaction pgx.Tx, actorAccountID, accountID string) (isSuspended bool, err error) {
	rows, err := transaction.Query(ctx, `
		SELECT id, role = 'admin' AND suspended_at IS NULL, suspended_at IS NOT NULL FROM accounts
		WHERE id = $1 OR id = $2 ORDER BY id FOR NO KEY UPDATE`, actorAccountID, accountID)
	if err != nil {
		return false, err
	}
	isFound, isActorActiveAdmin := false, false
	var id string
	var isActiveAdmin, isRowSuspended bool
	if _, err := pgx.ForEachRow(rows, []any{&id, &isActiveAdmin, &isRowSuspended}, func() error {
		if id == accountID {
			isFound, isSuspended = true, isRowSuspended
		}
		if id == actorAccountID {
			isActorActiveAdmin = isActiveAdmin
		}
		return nil
	}); err != nil {
		return false, err
	}
	if !isFound {
		return false, ErrNotFound
	}
	if !isActorActiveAdmin {
		return false, ErrActorNotAdmin
	}
	return isSuspended, nil
}

// ReactivateAccount lets a suspended account sign in again; it reports false, changing nothing, for an active account.
func (s *Store) ReactivateAccount(ctx context.Context, actorAccountID, accountID string) (reactivated bool, err error) {
	err = s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		isSuspended, err := lockActorAndAccount(ctx, transaction, actorAccountID, accountID)
		if err != nil || !isSuspended {
			return err
		}
		reactivated = true
		_, err = transaction.Exec(ctx, `UPDATE accounts SET suspended_at = NULL WHERE id = $1`, accountID)
		return err
	})
	return reactivated, err
}

// RevokeAccountDevice revokes one device of the account and deletes its sessions, whose token hashes it returns, as
// RevokeDevice does, after checking that the acting account is still an active admin.
func (s *Store) RevokeAccountDevice(ctx context.Context, actorAccountID, accountID, deviceID string) (endedTokenHashes [][]byte, err error) {
	err = s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		if _, err := lockActorAndAccount(ctx, transaction, actorAccountID, accountID); err != nil {
			return err
		}
		endedTokenHashes, err = revokeDevice(ctx, transaction, accountID, deviceID)
		return err
	})
	return endedTokenHashes, err
}

// SetAccountRole makes the account an admin or a member; it returns ErrLastServerAdmin when that leaves no active admin.
// Like every change here it returns ErrActorNotAdmin when the acting account is no longer an active admin.
func (s *Store) SetAccountRole(ctx context.Context, actor AdminActor, accountID, role string) error {
	return s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		if _, err := lockAdminTarget(ctx, transaction, actor, accountID, role != "admin"); err != nil {
			return err
		}
		_, err := transaction.Exec(ctx, `UPDATE accounts SET role = $2 WHERE id = $1`, accountID, role)
		return err
	})
}

// DeleteAccountAsAdmin deletes the account like DeleteAccount, without a proof from the account itself.
func (s *Store) DeleteAccountAsAdmin(ctx context.Context, actor AdminActor, accountID string) (*DeletedAccount, error) {
	return s.deleteAccount(ctx, accountID, func(transaction pgx.Tx) (email, name string, err error) {
		target, err := lockAdminTarget(ctx, transaction, actor, accountID, true)
		if err != nil {
			return "", "", err
		}
		return target.email, target.name, nil
	})
}

// SignOutAccount revokes every device of the account and deletes its sessions, after checking that the acting account is
// still an active admin; it returns how many devices it revoked.
func (s *Store) SignOutAccount(ctx context.Context, actorAccountID, accountID string) (revokedDeviceCount int, err error) {
	err = s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		if _, err := lockActorAndAccount(ctx, transaction, actorAccountID, accountID); err != nil {
			return err
		}
		commandTag, err := transaction.Exec(ctx, `UPDATE devices SET revoked_at = now() WHERE account_id = $1 AND revoked_at IS NULL`, accountID)
		if err != nil {
			return err
		}
		revokedDeviceCount = int(commandTag.RowsAffected())
		_, err = transaction.Exec(ctx, `DELETE FROM sessions WHERE account_id = $1`, accountID)
		return err
	})
	return revokedDeviceCount, err
}
