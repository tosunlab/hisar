package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"

	"github.com/tryhisar/hisar/core/crypt"
)

const keyringService = "hisar"

type state struct {
	Server    string            `json:"server"`
	Email     string            `json:"email"`
	AccountID string            `json:"accountId"`
	Token     string            `json:"token"`
	DeviceID  string            `json:"deviceId"`
	Pins      map[string][]byte `json:"pins,omitempty"`
	Seqs      map[string]int64  `json:"seqs,omitempty"`

	path string
}

func (s *state) Seq(vaultID string) int64         { return s.Seqs[vaultID] }
func (s *state) SetSeq(vaultID string, seq int64) { s.Seqs[vaultID] = seq }

func statePath() (string, error) {
	if p := os.Getenv("HISAR_STATE"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "hisar", "state.json"), nil
}

func loadState() (*state, error) {
	p, err := statePath()
	if err != nil {
		return nil, err
	}
	st := &state{path: p}
	b, err := os.ReadFile(p)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, st); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
	}
	if st.Pins == nil {
		st.Pins = map[string][]byte{}
	}
	if st.Seqs == nil {
		st.Seqs = map[string]int64{}
	}
	return st, nil
}

func (s *state) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *state) requireLogin() error {
	if s.Token == "" || s.AccountID == "" {
		return errors.New("not logged in; run `hisar login` or `hisar signup <invite-link>`")
	}
	return nil
}

// HISAR_SECRET_KEY overrides the keychain, for headless machines and scripting.
func loadSecretKey(accountID string) (crypt.SecretKey, error) {
	v := os.Getenv("HISAR_SECRET_KEY")
	if v == "" {
		var err error
		if v, err = keyring.Get(keyringService, accountID); err != nil {
			return crypt.SecretKey{}, fmt.Errorf("Secret Key not found in keychain (set HISAR_SECRET_KEY or log in again): %w", err)
		}
	}
	sk, err := crypt.ParseSecretKey(v)
	if err != nil {
		return sk, err
	}
	if sk.AccountID != accountID {
		return sk, errors.New("Secret Key belongs to a different account")
	}
	return sk, nil
}

func storeSecretKey(sk crypt.SecretKey) error {
	if os.Getenv("HISAR_SECRET_KEY") != "" {
		return nil
	}
	return keyring.Set(keyringService, sk.AccountID, sk.Encode())
}
