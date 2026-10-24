package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/tryhisar/hisar/core/crypt"
)

// CanCreateSecrets is false even after the last account is deleted if this database
// has already enrolled authentication keys or encrypted operational credentials.
func (s *Store) CanCreateSecrets(ctx context.Context) (bool, error) {
	var allowed bool
	err := s.pool.QueryRow(ctx, `SELECT NOT EXISTS (SELECT 1 FROM accounts) AND NOT EXISTS
		(SELECT 1 FROM operational_secrets WHERE fingerprint IS NOT NULL OR settings_version <> 0)`).Scan(&allowed)
	return allowed, err
}

// InitializeOperationalSecrets runs before settings are read or requests are served.
// Enrollment and legacy password encryption commit together under one singleton lock.
func (s *Store) InitializeOperationalSecrets(ctx context.Context, fingerprint, serverHMACKey []byte) error {
	if len(fingerprint) != 32 {
		return errors.New("store: invalid secrets fingerprint")
	}
	key, err := crypt.DeriveServerSettingsKey(serverHMACKey)
	if err != nil {
		return err
	}
	err = s.runInTransaction(ctx, func(tx pgx.Tx) error {
		var enrolled []byte
		var version int
		if err := tx.QueryRow(ctx, `SELECT fingerprint, settings_version FROM operational_secrets WHERE id = 1 FOR UPDATE`).Scan(&enrolled, &version); err != nil {
			return err
		}
		if enrolled != nil && !bytes.Equal(enrolled, fingerprint) {
			return errors.New("store: server secrets do not match this database; restore the matching database and hisar-secrets.json backup together, without replacing or deleting either")
		}
		var document []byte
		err := tx.QueryRow(ctx, `SELECT settings FROM instance_settings WHERE id = 1 FOR UPDATE`).Scan(&document)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if document != nil {
			if version == 0 {
				document, err = transformSMTPPassword(document, key, true, true)
				if err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `UPDATE instance_settings SET settings = $1 WHERE id = 1`, document); err != nil {
					return err
				}
			} else if _, err := transformSMTPPassword(document, key, false, false); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `ALTER TABLE instance_settings VALIDATE CONSTRAINT smtp_password_encrypted`); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE operational_secrets SET fingerprint = $1, settings_version = 1 WHERE id = 1`, fingerprint)
		return err
	})
	if err != nil {
		clear(key)
		return err
	}
	s.settingsKey = key
	return nil
}

type smtpPasswordEnvelope struct {
	Version    int    `json:"version"`
	Ciphertext []byte `json:"ciphertext"`
}

// transformSMTPPassword preserves unknown settings fields. Plaintext is accepted
// only from an in-memory update or the one-time, version-zero startup migration.
func transformSMTPPassword(document, key []byte, seal, migrate bool) ([]byte, error) {
	if len(document) == 0 {
		return document, nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(document, &root); err != nil {
		return nil, errors.New("store: invalid settings document")
	}
	rawSMTP, exists := root["smtp"]
	if !exists {
		return document, nil
	}
	var smtp map[string]json.RawMessage
	if err := json.Unmarshal(rawSMTP, &smtp); err != nil {
		return nil, errors.New("store: invalid stored SMTP settings")
	}
	raw, exists := smtp["password"]
	if !exists || bytes.Equal(bytes.TrimSpace(raw), []byte(`""`)) {
		return document, nil
	}
	if len(key) != crypt.KeySize {
		return nil, errors.New("store: operational secrets must be initialized before accessing an SMTP password")
	}
	var password string
	if err := json.Unmarshal(raw, &password); err == nil {
		if !seal {
			return nil, errors.New("store: plaintext SMTP password after operational-secrets migration")
		}
		sealed, err := crypt.SealSMTPPassword(key, password)
		if err != nil {
			return nil, err
		}
		smtp["password"], err = json.Marshal(smtpPasswordEnvelope{Version: 1, Ciphertext: sealed})
		if err != nil {
			return nil, err
		}
	} else {
		var envelope smtpPasswordEnvelope
		if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Version != 1 || len(envelope.Ciphertext) == 0 {
			return nil, errors.New("store: unsupported encrypted SMTP password envelope")
		}
		if seal && !migrate {
			return nil, errors.New("store: an SMTP password update must contain a string")
		}
		password, err := crypt.OpenSMTPPassword(key, envelope.Ciphertext)
		if err != nil {
			return nil, fmt.Errorf("store: cannot decrypt SMTP password; restore the matching database and server secrets backup: %w", err)
		}
		if seal {
			return document, nil
		}
		smtp["password"], err = json.Marshal(password)
		if err != nil {
			return nil, err
		}
	}
	encodedSMTP, err := json.Marshal(smtp)
	if err != nil {
		return nil, err
	}
	root["smtp"] = encodedSMTP
	return json.Marshal(root)
}
