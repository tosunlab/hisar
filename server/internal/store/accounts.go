package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

type Account struct {
	ID           string
	Email        string
	Name         string
	Role         string
	Argon2Params json.RawMessage
	Salt         []byte
	OpaqueRecord []byte
	EncKeyset    []byte
	KEMPub       []byte
	SignPub      []byte
	KEMSig       []byte
	CreatedAt    time.Time
}

type Invite struct {
	Email     string
	Role      string
	ExpiresAt time.Time
}

type Device struct {
	ID         string
	AccountID  string
	Name       string
	Platform   string
	CreatedAt  time.Time
	LastSeenAt time.Time
}

type Session struct {
	TokenHash []byte
	AccountID string
	DeviceID  string
	Role      string
	ExpiresAt time.Time
}

const accountColumnList = `id, email, name, role, argon2_params, salt, opaque_record, enc_keyset, kem_pub, sign_pub, kem_sig, created_at`

func scanAccount(row pgx.Row) (*Account, error) {
	var account Account
	err := row.Scan(&account.ID, &account.Email, &account.Name, &account.Role, &account.Argon2Params, &account.Salt, &account.OpaqueRecord, &account.EncKeyset, &account.KEMPub, &account.SignPub, &account.KEMSig, &account.CreatedAt)
	if err != nil {
		return nil, convertNoRowsToNotFound(err)
	}
	return &account, nil
}

func (s *Store) GetAccountByEmail(ctx context.Context, email string) (*Account, error) {
	return scanAccount(s.pool.QueryRow(ctx, `SELECT `+accountColumnList+` FROM accounts WHERE email = $1`, email))
}

func (s *Store) GetAccountByID(ctx context.Context, accountID string) (*Account, error) {
	return scanAccount(s.pool.QueryRow(ctx, `SELECT `+accountColumnList+` FROM accounts WHERE id = $1`, accountID))
}

func (s *Store) ListAccounts(ctx context.Context) ([]Account, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+accountColumnList+` FROM accounts ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var accounts []Account
	for rows.Next() {
		account, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, *account)
	}
	return accounts, rows.Err()
}

func (s *Store) CreateInvite(ctx context.Context, tokenHash []byte, email, role string, createdBy *string, ttl time.Duration) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO invites (token_hash, email, role, created_by, expires_at) VALUES ($1, $2, $3, $4, $5)`,
		tokenHash, email, role, createdBy, time.Now().Add(ttl))
	return err
}

// HasAccounts reports whether any account exists.
func (s *Store) HasAccounts(ctx context.Context) (bool, error) {
	var hasAccounts bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM accounts)`).Scan(&hasAccounts)
	return hasAccounts, err
}

// DeleteUnusedSetupInvites deletes the setup invites nobody signed up with. A setup invite is the one invite without an
// email: whoever holds it signs up with the email they enter.
func (s *Store) DeleteUnusedSetupInvites(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM invites WHERE email = '' AND used_at IS NULL`)
	return err
}

func (s *Store) GetInviteByHash(ctx context.Context, tokenHash []byte) (*Invite, error) {
	var invite Invite
	err := s.pool.QueryRow(ctx,
		`SELECT email, role, expires_at FROM invites WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()`,
		tokenHash).Scan(&invite.Email, &invite.Role, &invite.ExpiresAt)
	if err != nil {
		return nil, convertNoRowsToNotFound(err)
	}
	return &invite, nil
}

type InviteWithInviter struct {
	Email        string
	ExpiresAt    time.Time
	InviterName  string
	InviterEmail string
}

// GetInviteWithInviter returns a usable invite; unknown, used and expired invites are all ErrNotFound.
func (s *Store) GetInviteWithInviter(ctx context.Context, tokenHash []byte) (*InviteWithInviter, error) {
	var invite InviteWithInviter
	err := s.pool.QueryRow(ctx, `
		SELECT i.email, i.expires_at, coalesce(a.name, ''), coalesce(a.email, '')
		FROM invites i LEFT JOIN accounts a ON a.id = i.created_by
		WHERE i.token_hash = $1 AND i.used_at IS NULL AND i.expires_at > now()`,
		tokenHash).Scan(&invite.Email, &invite.ExpiresAt, &invite.InviterName, &invite.InviterEmail)
	if err != nil {
		return nil, convertNoRowsToNotFound(err)
	}
	return &invite, nil
}

// RedeemInvite atomically consumes the invite and creates the account, its first device and a session.
func (s *Store) RedeemInvite(ctx context.Context, tokenHash []byte, account *Account, device *Device, session *Session) error {
	return s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		var email, role string
		err := transaction.QueryRow(ctx,
			`SELECT email, role FROM invites WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now() FOR UPDATE`,
			tokenHash).Scan(&email, &role)
		if err != nil {
			return convertNoRowsToNotFound(err)
		}
		if email != "" && email != account.Email {
			return ErrNotFound
		}
		account.Role = role
		_, err = transaction.Exec(ctx,
			`INSERT INTO accounts (id, email, name, role, argon2_params, salt, opaque_record, enc_keyset, kem_pub, sign_pub, kem_sig)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			account.ID, account.Email, account.Name, account.Role, account.Argon2Params, account.Salt, account.OpaqueRecord, account.EncKeyset, account.KEMPub, account.SignPub, account.KEMSig)
		if isUniqueViolation(err) {
			return ErrExists
		}
		if err != nil {
			return err
		}
		if _, err := transaction.Exec(ctx, `UPDATE invites SET used_at = now() WHERE token_hash = $1`, tokenHash); err != nil {
			return err
		}
		return insertDeviceSession(ctx, transaction, device, session)
	})
}

// CreateDeviceSession signs a device in; it fails with ErrAccountSuspended for a suspended account. The account row is
// read FOR SHARE, so a suspension that commits meanwhile waits for the new session, which GetSessionByHash then refuses.
func (s *Store) CreateDeviceSession(ctx context.Context, device *Device, session *Session) error {
	return s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		var isSuspended bool
		if err := transaction.QueryRow(ctx, `SELECT suspended_at IS NOT NULL FROM accounts WHERE id = $1 FOR SHARE`, device.AccountID).Scan(&isSuspended); err != nil {
			return convertNoRowsToNotFound(err)
		}
		if isSuspended {
			return ErrAccountSuspended
		}
		return insertDeviceSession(ctx, transaction, device, session)
	})
}

func insertDeviceSession(ctx context.Context, transaction pgx.Tx, device *Device, session *Session) error {
	if _, err := transaction.Exec(ctx,
		`INSERT INTO devices (id, account_id, name, platform) VALUES ($1, $2, $3, $4)`,
		device.ID, device.AccountID, device.Name, device.Platform); err != nil {
		return err
	}
	_, err := transaction.Exec(ctx,
		`INSERT INTO sessions (token_hash, account_id, device_id, expires_at) VALUES ($1, $2, $3, $4)`,
		session.TokenHash, session.AccountID, session.DeviceID, session.ExpiresAt)
	return err
}

// GetSessionByHash returns a live session and slides its expiry forward. The session of a suspended account stays live,
// so its device works again after reactivation, but is refused with ErrAccountSuspended until then.
func (s *Store) GetSessionByHash(ctx context.Context, tokenHash []byte, ttl time.Duration) (*Session, error) {
	var session Session
	var isSuspended bool
	err := s.pool.QueryRow(ctx, `
		UPDATE sessions s SET expires_at = now() + $2::interval
		FROM accounts a, devices d
		WHERE s.token_hash = $1 AND s.expires_at > now()
		  AND a.id = s.account_id AND d.id = s.device_id AND d.revoked_at IS NULL
		RETURNING s.token_hash, s.account_id, s.device_id, a.role, s.expires_at, a.suspended_at IS NOT NULL`,
		tokenHash, ttl).Scan(&session.TokenHash, &session.AccountID, &session.DeviceID, &session.Role, &session.ExpiresAt, &isSuspended)
	if err != nil {
		return nil, convertNoRowsToNotFound(err)
	}
	if isSuspended {
		return nil, ErrAccountSuspended
	}
	s.pool.Exec(ctx, `UPDATE devices SET last_seen_at = now() WHERE id = $1`, session.DeviceID)
	return &session, nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash []byte) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
	return err
}

func (s *Store) ListDevices(ctx context.Context, accountID string) ([]Device, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, account_id, name, platform, created_at, last_seen_at FROM devices
		 WHERE account_id = $1 AND revoked_at IS NULL ORDER BY created_at`, accountID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Device, error) {
		var device Device
		err := row.Scan(&device.ID, &device.AccountID, &device.Name, &device.Platform, &device.CreatedAt, &device.LastSeenAt)
		return device, err
	})
}

// RevokeDevice revokes the account's device and deletes its sessions, whose token hashes it returns.
func (s *Store) RevokeDevice(ctx context.Context, accountID, deviceID string) (endedTokenHashes [][]byte, err error) {
	err = s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		endedTokenHashes, err = revokeDevice(ctx, transaction, accountID, deviceID)
		return err
	})
	return endedTokenHashes, err
}

func revokeDevice(ctx context.Context, transaction pgx.Tx, accountID, deviceID string) ([][]byte, error) {
	commandTag, err := transaction.Exec(ctx,
		`UPDATE devices SET revoked_at = now() WHERE id = $1 AND account_id = $2 AND revoked_at IS NULL`,
		deviceID, accountID)
	if err != nil {
		return nil, err
	}
	if commandTag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return collectTokenHashes(ctx, transaction, `DELETE FROM sessions WHERE device_id = $1 RETURNING token_hash`, deviceID)
}

func collectTokenHashes(ctx context.Context, transaction pgx.Tx, query string, arguments ...any) ([][]byte, error) {
	rows, err := transaction.Query(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[[]byte])
}

func (s *Store) RecordAudit(ctx context.Context, accountID, deviceID *string, eventType string, detail any, ip string) {
	s.pool.Exec(ctx,
		`INSERT INTO audit_events (account_id, device_id, type, detail, ip) VALUES ($1, $2, $3, $4, $5)`,
		accountID, deviceID, eventType, detail, ip)
}

// PruneAudit deletes audit events older than maxAge.
func (s *Store) PruneAudit(ctx context.Context, maxAge time.Duration) (int64, error) {
	commandTag, err := s.pool.Exec(ctx, `DELETE FROM audit_events WHERE created_at < now() - $1::interval`, maxAge)
	return commandTag.RowsAffected(), err
}

// PruneExpiredInvitation deletes sign-up invites, pending share invitations and organization invitations that expired
// more than maxAge ago. Accepted share invitations never expire: they wait for the owner's app.
func (s *Store) PruneExpiredInvitation(ctx context.Context, maxAge time.Duration) (int64, error) {
	var prunedCount int64
	err := pgx.BeginFunc(ctx, s.pool, func(transaction pgx.Tx) error {
		for _, query := range []string{
			`DELETE FROM invites WHERE expires_at < now() - $1::interval`,
			`DELETE FROM share_invitations WHERE state = 'pending' AND expires_at < now() - $1::interval`,
			`DELETE FROM organization_invitations WHERE expires_at < now() - $1::interval`,
		} {
			commandTag, err := transaction.Exec(ctx, query, maxAge)
			if err != nil {
				return err
			}
			prunedCount += commandTag.RowsAffected()
		}
		return nil
	})
	return prunedCount, err
}
