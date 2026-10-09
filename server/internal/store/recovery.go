package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

type RecoveryKeyset struct {
	KEMPub    []byte
	SignPub   []byte
	KEMSig    []byte
	EncKeyset []byte
	CreatedBy string
	CreatedAt time.Time
}

// RecoveryVault is a vault the account can access; WrappedKey is its recovery wrapping, nil when it has none.
type RecoveryVault struct {
	VaultID    string
	Type       string
	WrappedKey []byte
}

// PendingRecovery holds the credentials an account submitted with a recovery link, applied when an admin completes it.
type PendingRecovery struct {
	AccountID    string
	Email        string
	Name         string
	Argon2Params json.RawMessage
	Salt         []byte
	OpaqueRecord []byte
	EncKeyset    []byte
	KEMPub       []byte
	SignPub      []byte
	KEMSig       []byte
	SubmittedAt  time.Time
	AllVault     []RecoveryVault
}

type VaultKey struct {
	VaultID    string
	WrappedKey []byte
}

func (s *Store) CreateRecoveryKeyset(ctx context.Context, recoveryKeyset *RecoveryKeyset) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO recovery_keyset (kem_pub, sign_pub, kem_sig, enc_keyset, created_by) VALUES ($1, $2, $3, $4, $5)`,
		recoveryKeyset.KEMPub, recoveryKeyset.SignPub, recoveryKeyset.KEMSig, recoveryKeyset.EncKeyset, recoveryKeyset.CreatedBy)
	if isUniqueViolation(err) {
		return ErrExists
	}
	return err
}

func (s *Store) GetRecoveryKeyset(ctx context.Context) (*RecoveryKeyset, error) {
	var recoveryKeyset RecoveryKeyset
	err := s.pool.QueryRow(ctx, `SELECT kem_pub, sign_pub, kem_sig, enc_keyset, created_by, created_at FROM recovery_keyset`).Scan(
		&recoveryKeyset.KEMPub, &recoveryKeyset.SignPub, &recoveryKeyset.KEMSig, &recoveryKeyset.EncKeyset, &recoveryKeyset.CreatedBy, &recoveryKeyset.CreatedAt)
	if err != nil {
		return nil, convertNoRowsToNotFound(err)
	}
	return &recoveryKeyset, nil
}

func (s *Store) SetVaultRecovery(ctx context.Context, vaultID string, wrappedKey []byte) error {
	return s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		if _, err := transaction.Exec(ctx, `
			INSERT INTO vault_recovery (vault_id, wrapped_key) VALUES ($1, $2)
			ON CONFLICT (vault_id) DO UPDATE SET wrapped_key = $2, updated_at = now()`, vaultID, wrappedKey); err != nil {
			return err
		}
		_, err := transaction.Exec(ctx, `UPDATE vaults SET recovery_opt_out = false WHERE id = $1`, vaultID)
		return err
	})
}

// OptOutVaultRecovery removes the vault's recovery wrapping and remembers that its owner opted out.
func (s *Store) OptOutVaultRecovery(ctx context.Context, vaultID string) error {
	return s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		if _, err := transaction.Exec(ctx, `DELETE FROM vault_recovery WHERE vault_id = $1`, vaultID); err != nil {
			return err
		}
		_, err := transaction.Exec(ctx, `UPDATE vaults SET recovery_opt_out = true WHERE id = $1`, vaultID)
		return err
	})
}

// CreateRecoveryToken replaces any earlier token and pending submission of the account.
func (s *Store) CreateRecoveryToken(ctx context.Context, accountID string, tokenHash []byte, createdBy string, ttl time.Duration) error {
	return s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		if _, err := transaction.Exec(ctx, `DELETE FROM account_recovery WHERE account_id = $1`, accountID); err != nil {
			return err
		}
		_, err := transaction.Exec(ctx, `
			INSERT INTO recovery_tokens (account_id, token_hash, created_by, expires_at) VALUES ($1, $2, $3, $4)
			ON CONFLICT (account_id) DO UPDATE SET token_hash = $2, created_by = $3, created_at = now(), expires_at = $4, used_at = NULL`,
			accountID, tokenHash, createdBy, time.Now().Add(ttl))
		return err
	})
}

// GetRecoveryTokenAccount returns the account of an unused, unexpired recovery token.
func (s *Store) GetRecoveryTokenAccount(ctx context.Context, tokenHash []byte) (*Account, error) {
	return scanAccount(s.pool.QueryRow(ctx, `
		SELECT `+accountColumnList+` FROM accounts WHERE id = (
			SELECT account_id FROM recovery_tokens WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now())`, tokenHash))
}

// SubmitRecovery consumes the token and stores the account's new credentials as pending.
func (s *Store) SubmitRecovery(ctx context.Context, tokenHash []byte, pending *PendingRecovery) error {
	return s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		var accountID string
		err := transaction.QueryRow(ctx,
			`SELECT account_id FROM recovery_tokens WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now() FOR UPDATE`,
			tokenHash).Scan(&accountID)
		if err != nil {
			return convertNoRowsToNotFound(err)
		}
		if accountID != pending.AccountID {
			return ErrNotFound
		}
		if _, err := transaction.Exec(ctx, `
			INSERT INTO account_recovery (account_id, argon2_params, salt, opaque_record, enc_keyset, kem_pub, sign_pub, kem_sig)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (account_id) DO UPDATE SET argon2_params = $2, salt = $3, opaque_record = $4, enc_keyset = $5,
				kem_pub = $6, sign_pub = $7, kem_sig = $8, submitted_at = now()`,
			pending.AccountID, pending.Argon2Params, pending.Salt, pending.OpaqueRecord, pending.EncKeyset, pending.KEMPub, pending.SignPub, pending.KEMSig); err != nil {
			return err
		}
		_, err = transaction.Exec(ctx, `UPDATE recovery_tokens SET used_at = now() WHERE token_hash = $1`, tokenHash)
		return err
	})
}

func (s *Store) ListAllPendingRecovery(ctx context.Context) ([]PendingRecovery, error) {
	allRow, err := s.pool.Query(ctx, `
		SELECT p.account_id, a.email, a.name, p.argon2_params, p.salt, p.opaque_record, p.enc_keyset, p.kem_pub, p.sign_pub, p.kem_sig, p.submitted_at
		FROM account_recovery p JOIN accounts a ON a.id = p.account_id ORDER BY p.submitted_at`)
	if err != nil {
		return nil, err
	}
	allPendingRecovery, err := pgx.CollectRows(allRow, func(row pgx.CollectableRow) (PendingRecovery, error) {
		var pending PendingRecovery
		err := row.Scan(&pending.AccountID, &pending.Email, &pending.Name, &pending.Argon2Params, &pending.Salt, &pending.OpaqueRecord,
			&pending.EncKeyset, &pending.KEMPub, &pending.SignPub, &pending.KEMSig, &pending.SubmittedAt)
		return pending, err
	})
	if err != nil {
		return nil, err
	}
	for index := range allPendingRecovery {
		pending := &allPendingRecovery[index]
		if pending.AllVault, err = s.listAllRecoveryVault(ctx, pending.AccountID); err != nil {
			return nil, err
		}
	}
	return allPendingRecovery, nil
}

func (s *Store) listAllRecoveryVault(ctx context.Context, accountID string) ([]RecoveryVault, error) {
	allRow, err := s.pool.Query(ctx, `
		SELECT v.id, v.type, r.wrapped_key
		FROM vault_access a JOIN vaults v ON v.id = a.vault_id LEFT JOIN vault_recovery r ON r.vault_id = v.id
		WHERE a.account_id = $1 ORDER BY v.created_at`, accountID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(allRow, func(row pgx.CollectableRow) (RecoveryVault, error) {
		var recoveryVault RecoveryVault
		err := row.Scan(&recoveryVault.VaultID, &recoveryVault.Type, &recoveryVault.WrappedKey)
		return recoveryVault, err
	})
}

// CompleteRecovery applies the pending credentials, re-grants allVaultKey, removes access to every other vault
// and signs the account out everywhere, in one transaction. It returns the vaults the account lost.
// ErrConflict means allVaultKey names a vault the account cannot access.
// Recovering the admin who created the recovery keyset deletes it, because it was sealed with their old keys.
func (s *Store) CompleteRecovery(ctx context.Context, accountID string, allVaultKey []VaultKey) (allUnrecoverableVaultID []string, err error) {
	err = s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		var pending PendingRecovery
		err := transaction.QueryRow(ctx, `
			SELECT argon2_params, salt, opaque_record, enc_keyset, kem_pub, sign_pub, kem_sig FROM account_recovery
			WHERE account_id = $1 FOR UPDATE`, accountID).Scan(
			&pending.Argon2Params, &pending.Salt, &pending.OpaqueRecord, &pending.EncKeyset, &pending.KEMPub, &pending.SignPub, &pending.KEMSig)
		if err != nil {
			return convertNoRowsToNotFound(err)
		}
		if _, err := transaction.Exec(ctx, `
			UPDATE accounts SET argon2_params = $2, salt = $3, opaque_record = $4, enc_keyset = $5, kem_pub = $6, sign_pub = $7, kem_sig = $8
			WHERE id = $1`,
			accountID, pending.Argon2Params, pending.Salt, pending.OpaqueRecord, pending.EncKeyset, pending.KEMPub, pending.SignPub, pending.KEMSig); err != nil {
			return err
		}

		allRecoveredVaultID := make([]string, 0, len(allVaultKey))
		for _, vaultKey := range allVaultKey {
			commandTag, err := transaction.Exec(ctx,
				`UPDATE vault_access SET wrapped_key = $3 WHERE vault_id = $1 AND account_id = $2`, vaultKey.VaultID, accountID, vaultKey.WrappedKey)
			if err != nil {
				return err
			}
			if commandTag.RowsAffected() == 0 {
				return ErrConflict
			}
			allRecoveredVaultID = append(allRecoveredVaultID, vaultKey.VaultID)
		}
		allRow, err := transaction.Query(ctx, `
			DELETE FROM vault_access WHERE account_id = $1 AND NOT (vault_id::text = ANY ($2::text[])) RETURNING vault_id::text`,
			accountID, allRecoveredVaultID)
		if err != nil {
			return err
		}
		if allUnrecoverableVaultID, err = pgx.CollectRows(allRow, pgx.RowTo[string]); err != nil {
			return err
		}

		for _, statement := range []string{
			`DELETE FROM sessions WHERE account_id = $1`,
			`DELETE FROM devices WHERE account_id = $1`,
			`DELETE FROM account_recovery WHERE account_id = $1`,
			`DELETE FROM recovery_tokens WHERE account_id = $1`,
		} {
			if _, err := transaction.Exec(ctx, statement, accountID); err != nil {
				return err
			}
		}
		commandTag, err := transaction.Exec(ctx, `DELETE FROM recovery_keyset WHERE created_by = $1`, accountID)
		if err != nil {
			return err
		}
		if commandTag.RowsAffected() > 0 {
			_, err = transaction.Exec(ctx, `DELETE FROM vault_recovery`)
		}
		return err
	})
	return allUnrecoverableVaultID, err
}
