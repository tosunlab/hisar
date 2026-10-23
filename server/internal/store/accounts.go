package store

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
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

// isWithinCreatorAuthority holds for an invite i that its creator may still give: an admin invite from an account needs
// that account to still be an active admin. Operator and setup invites have no creator.
const isWithinCreatorAuthority = `(i.role <> 'admin' OR i.created_by IS NULL OR EXISTS (SELECT 1 FROM accounts creator
	WHERE creator.id = i.created_by AND creator.role = 'admin' AND creator.suspended_at IS NULL))`

func (s *Store) GetInviteByHash(ctx context.Context, tokenHash []byte) (*Invite, error) {
	var invite Invite
	err := s.pool.QueryRow(ctx,
		`SELECT i.email, i.role, i.expires_at FROM invites i WHERE i.token_hash = $1 AND i.used_at IS NULL AND i.expires_at > now() AND `+isWithinCreatorAuthority,
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
		WHERE i.token_hash = $1 AND i.used_at IS NULL AND i.expires_at > now() AND `+isWithinCreatorAuthority,
		tokenHash).Scan(&invite.Email, &invite.ExpiresAt, &invite.InviterName, &invite.InviterEmail)
	if err != nil {
		return nil, convertNoRowsToNotFound(err)
	}
	return &invite, nil
}

// RedeemInvite atomically consumes the invite and creates the account, its first device and a session. An admin invite
// from an account that is no longer an active admin is ErrNotFound.
func (s *Store) RedeemInvite(ctx context.Context, tokenHash []byte, account *Account, device *Device, session *Session) error {
	return s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		var creatorID *string
		var inviteRole string
		if err := transaction.QueryRow(ctx, `SELECT role, created_by FROM invites WHERE token_hash = $1`, tokenHash).Scan(&inviteRole, &creatorID); err != nil {
			return convertNoRowsToNotFound(err)
		}
		// Account changes lock accounts before invitations. Hold the issuer's authority through redemption too.
		if inviteRole == "admin" && creatorID != nil {
			if err := requireActiveAdmin(ctx, transaction, *creatorID); errors.Is(err, ErrActorNotAdmin) {
				return ErrNotFound
			} else if err != nil {
				return err
			}
		}
		var email, role string
		err := transaction.QueryRow(ctx,
			`SELECT i.email, i.role FROM invites i WHERE i.token_hash = $1 AND i.used_at IS NULL AND i.expires_at > now()
				AND i.role = $2 AND i.created_by IS NOT DISTINCT FROM $3 AND `+isWithinCreatorAuthority+`
			FOR UPDATE OF i`,
			tokenHash, inviteRole, creatorID).Scan(&email, &role)
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

// CreateDeviceSession signs a device in with a login proven against the OPAQUE record whose hash is provenRecordHash. It
// fails with ErrAccountSuspended for a suspended account, and with ErrConflict when the record changed after the proof,
// so a login started before a credential change cannot finish after it. The account row is read FOR SHARE, so a
// suspension or credential change that commits meanwhile waits for the new session, which it then refuses or signs out.
func (s *Store) CreateDeviceSession(ctx context.Context, device *Device, session *Session, provenRecordHash []byte) error {
	return s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		var isSuspended bool
		var opaqueRecord []byte
		if err := transaction.QueryRow(ctx, `SELECT suspended_at IS NOT NULL, opaque_record FROM accounts WHERE id = $1 FOR SHARE`,
			device.AccountID).Scan(&isSuspended, &opaqueRecord); err != nil {
			return convertNoRowsToNotFound(err)
		}
		if subtle.ConstantTimeCompare(HashOpaqueRecord(opaqueRecord), provenRecordHash) != 1 {
			return ErrConflict
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

// GetSessionByHash atomically slides a live session's expiry and records device activity, locking the device first.
// Suspended accounts return ErrAccountSuspended without extending expiry or recording activity.
func (s *Store) GetSessionByHash(ctx context.Context, tokenHash []byte, ttl time.Duration) (*Session, error) {
	var session Session
	var isSuspended bool
	err := s.pool.QueryRow(ctx, `
		WITH device AS (
			SELECT d.id FROM sessions s JOIN devices d ON d.id = s.device_id
			WHERE s.token_hash = $1 AND s.expires_at > now() AND d.revoked_at IS NULL
			FOR NO KEY UPDATE OF d
		), live AS (
			UPDATE sessions s SET expires_at = CASE WHEN a.suspended_at IS NULL THEN now() + $2::interval ELSE s.expires_at END
			FROM accounts a, device d
			WHERE s.token_hash = $1 AND s.expires_at > now()
			  AND a.id = s.account_id AND d.id = s.device_id
			RETURNING s.token_hash, s.account_id, s.device_id, a.role, s.expires_at, a.suspended_at IS NOT NULL AS is_suspended
		), seen AS (
			UPDATE devices SET last_seen_at = now() WHERE id IN (SELECT device_id FROM live WHERE NOT is_suspended)
		)
		SELECT token_hash, account_id, device_id, role, expires_at, is_suspended FROM live`,
		tokenHash, ttl).Scan(&session.TokenHash, &session.AccountID, &session.DeviceID, &session.Role, &session.ExpiresAt, &isSuspended)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			log.Printf("session lookup or device last-seen update failed: %v", err)
		}
		return nil, convertNoRowsToNotFound(err)
	}
	if isSuspended {
		return nil, ErrAccountSuspended
	}
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

func (s *Store) RecordAudit(ctx context.Context, accountID, deviceID *string, eventType string, detail any, ip string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO audit_events (account_id, device_id, type, detail, ip) VALUES ($1, $2, $3, $4, $5)`,
		accountID, deviceID, eventType, detail, ip)
	return err
}

// PruneAudit deletes audit events older than maxAge.
func (s *Store) PruneAudit(ctx context.Context, maxAge time.Duration) (int64, error) {
	commandTag, err := s.pool.Exec(ctx, `DELETE FROM audit_events WHERE created_at < now() - $1::interval`, maxAge)
	return commandTag.RowsAffected(), err
}

// PruneExpiredInvitation deletes sign-up invites, pending share invitations and organization invitations that expired
// more than maxAge ago, and the retired share invitation IDs whose signed expiry passed. Accepted share invitations are
// kept until an owner completes or cancels them. The count is of invitations only.
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
		_, err := transaction.Exec(ctx, `DELETE FROM retired_share_invitations WHERE expires_at <= now()`)
		return err
	})
	return prunedCount, err
}
