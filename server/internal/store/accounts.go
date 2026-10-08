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

const accountCols = `id, email, name, role, argon2_params, salt, opaque_record, enc_keyset, kem_pub, sign_pub, kem_sig, created_at`

func scanAccount(row pgx.Row) (*Account, error) {
	var a Account
	err := row.Scan(&a.ID, &a.Email, &a.Name, &a.Role, &a.Argon2Params, &a.Salt, &a.OpaqueRecord, &a.EncKeyset, &a.KEMPub, &a.SignPub, &a.KEMSig, &a.CreatedAt)
	if err != nil {
		return nil, notFound(err)
	}
	return &a, nil
}

func (s *Store) AccountByEmail(ctx context.Context, email string) (*Account, error) {
	return scanAccount(s.pool.QueryRow(ctx, `SELECT `+accountCols+` FROM accounts WHERE email = $1`, email))
}

func (s *Store) AccountByID(ctx context.Context, id string) (*Account, error) {
	return scanAccount(s.pool.QueryRow(ctx, `SELECT `+accountCols+` FROM accounts WHERE id = $1`, id))
}

func (s *Store) ListAccounts(ctx context.Context) ([]Account, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+accountCols+` FROM accounts ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (s *Store) CreateInvite(ctx context.Context, tokenHash []byte, email, role string, createdBy *string, ttl time.Duration) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO invites (token_hash, email, role, created_by, expires_at) VALUES ($1, $2, $3, $4, $5)`,
		tokenHash, email, role, createdBy, time.Now().Add(ttl))
	return err
}

func (s *Store) InviteByHash(ctx context.Context, tokenHash []byte) (*Invite, error) {
	var inv Invite
	err := s.pool.QueryRow(ctx,
		`SELECT email, role, expires_at FROM invites WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()`,
		tokenHash).Scan(&inv.Email, &inv.Role, &inv.ExpiresAt)
	if err != nil {
		return nil, notFound(err)
	}
	return &inv, nil
}

// RedeemInvite atomically consumes the invite and creates the account, its first device and a session.
func (s *Store) RedeemInvite(ctx context.Context, tokenHash []byte, a *Account, d *Device, sess *Session) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		var email, role string
		err := tx.QueryRow(ctx,
			`SELECT email, role FROM invites WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now() FOR UPDATE`,
			tokenHash).Scan(&email, &role)
		if err != nil {
			return notFound(err)
		}
		if email != a.Email {
			return ErrNotFound
		}
		a.Role = role
		_, err = tx.Exec(ctx,
			`INSERT INTO accounts (id, email, name, role, argon2_params, salt, opaque_record, enc_keyset, kem_pub, sign_pub, kem_sig)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			a.ID, a.Email, a.Name, a.Role, a.Argon2Params, a.Salt, a.OpaqueRecord, a.EncKeyset, a.KEMPub, a.SignPub, a.KEMSig)
		if isUniqueViolation(err) {
			return ErrExists
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE invites SET used_at = now() WHERE token_hash = $1`, tokenHash); err != nil {
			return err
		}
		return insertDeviceSession(ctx, tx, d, sess)
	})
}

func (s *Store) CreateDeviceSession(ctx context.Context, d *Device, sess *Session) error {
	return s.tx(ctx, func(tx pgx.Tx) error { return insertDeviceSession(ctx, tx, d, sess) })
}

func insertDeviceSession(ctx context.Context, tx pgx.Tx, d *Device, sess *Session) error {
	if _, err := tx.Exec(ctx,
		`INSERT INTO devices (id, account_id, name, platform) VALUES ($1, $2, $3, $4)`,
		d.ID, d.AccountID, d.Name, d.Platform); err != nil {
		return err
	}
	_, err := tx.Exec(ctx,
		`INSERT INTO sessions (token_hash, account_id, device_id, expires_at) VALUES ($1, $2, $3, $4)`,
		sess.TokenHash, sess.AccountID, sess.DeviceID, sess.ExpiresAt)
	return err
}

// SessionByHash returns a live session and slides its expiry forward.
func (s *Store) SessionByHash(ctx context.Context, tokenHash []byte, ttl time.Duration) (*Session, error) {
	var sess Session
	err := s.pool.QueryRow(ctx, `
		UPDATE sessions s SET expires_at = now() + $2::interval
		FROM accounts a, devices d
		WHERE s.token_hash = $1 AND s.expires_at > now()
		  AND a.id = s.account_id AND d.id = s.device_id AND d.revoked_at IS NULL
		RETURNING s.token_hash, s.account_id, s.device_id, a.role, s.expires_at`,
		tokenHash, ttl).Scan(&sess.TokenHash, &sess.AccountID, &sess.DeviceID, &sess.Role, &sess.ExpiresAt)
	if err != nil {
		return nil, notFound(err)
	}
	s.pool.Exec(ctx, `UPDATE devices SET last_seen_at = now() WHERE id = $1`, sess.DeviceID)
	return &sess, nil
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
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Device, error) {
		var d Device
		err := r.Scan(&d.ID, &d.AccountID, &d.Name, &d.Platform, &d.CreatedAt, &d.LastSeenAt)
		return d, err
	})
}

func (s *Store) RevokeDevice(ctx context.Context, accountID, deviceID string) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`UPDATE devices SET revoked_at = now() WHERE id = $1 AND account_id = $2 AND revoked_at IS NULL`,
			deviceID, accountID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		_, err = tx.Exec(ctx, `DELETE FROM sessions WHERE device_id = $1`, deviceID)
		return err
	})
}

func (s *Store) Audit(ctx context.Context, accountID, deviceID *string, typ string, detail any, ip string) {
	s.pool.Exec(ctx,
		`INSERT INTO audit_events (account_id, device_id, type, detail, ip) VALUES ($1, $2, $3, $4, $5)`,
		accountID, deviceID, typ, detail, ip)
}
