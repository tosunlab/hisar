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
	AllPin    map[string][]byte `json:"pins,omitempty"`
	AllSeq    map[string]int64  `json:"seqs,omitempty"`

	path string
}

func (s *state) GetSeq(vaultID string) int64      { return s.AllSeq[vaultID] }
func (s *state) SetSeq(vaultID string, seq int64) { s.AllSeq[vaultID] = seq }

func getStatePath() (string, error) {
	if path := os.Getenv("HISAR_STATE"); path != "" {
		return path, nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "hisar", "state.json"), nil
}

func loadState() (*state, error) {
	path, err := getStatePath()
	if err != nil {
		return nil, err
	}
	currentState := &state{path: path}
	encodedState, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if len(encodedState) > 0 {
		if err := json.Unmarshal(encodedState, currentState); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	if currentState.AllPin == nil {
		currentState.AllPin = map[string][]byte{}
	}
	if currentState.AllSeq == nil {
		currentState.AllSeq = map[string]int64{}
	}
	return currentState, nil
}

func (s *state) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	encodedState, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	temporaryPath := s.path + ".tmp"
	if err := os.WriteFile(temporaryPath, encodedState, 0o600); err != nil {
		return err
	}
	return os.Rename(temporaryPath, s.path)
}

func (s *state) requireLogin() error {
	if s.Token == "" || s.AccountID == "" {
		return errors.New("not logged in; run `hisar login` or `hisar signup <invite-link>`")
	}
	return nil
}

// HISAR_SECRET_KEY overrides the keychain, for headless machines and scripting.
func loadSecretKey(accountID string) (crypt.SecretKey, error) {
	encodedSecretKey := os.Getenv("HISAR_SECRET_KEY")
	if encodedSecretKey == "" {
		var err error
		if encodedSecretKey, err = keyring.Get(keyringService, accountID); err != nil {
			return crypt.SecretKey{}, fmt.Errorf("Secret Key not found in keychain (set HISAR_SECRET_KEY or log in again): %w", err)
		}
	}
	secretKey, err := crypt.ParseSecretKey(encodedSecretKey)
	if err != nil {
		return secretKey, err
	}
	if secretKey.AccountID != accountID {
		return secretKey, errors.New("Secret Key belongs to a different account")
	}
	return secretKey, nil
}

func storeSecretKey(secretKey crypt.SecretKey) error {
	if os.Getenv("HISAR_SECRET_KEY") != "" {
		return nil
	}
	return keyring.Set(keyringService, secretKey.AccountID, secretKey.Encode())
}
