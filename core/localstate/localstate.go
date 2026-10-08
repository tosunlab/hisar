// Package localstate persists a device's session state and keeps the Secret Key in the OS keychain.
package localstate

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

type State struct {
	Server    string            `json:"server"`
	Email     string            `json:"email"`
	AccountID string            `json:"accountId"`
	Token     string            `json:"token"`
	DeviceID  string            `json:"deviceId"`
	AllPin    map[string][]byte `json:"pins,omitempty"`
	AllSeq    map[string]int64  `json:"seqs,omitempty"`

	path string
}

func (s *State) GetSeq(vaultID string) int64      { return s.AllSeq[vaultID] }
func (s *State) SetSeq(vaultID string, seq int64) { s.AllSeq[vaultID] = seq }

// GetPath returns the value of environmentVariable if set, otherwise <user config dir>/hisar/<fileName>.
func GetPath(environmentVariable, fileName string) (string, error) {
	if path := os.Getenv(environmentVariable); path != "" {
		return path, nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "hisar", fileName), nil
}

func Load(path string) (*State, error) {
	state := &State{path: path}
	encodedState, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if len(encodedState) > 0 {
		if err := json.Unmarshal(encodedState, state); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	if state.AllPin == nil {
		state.AllPin = map[string][]byte{}
	}
	if state.AllSeq == nil {
		state.AllSeq = map[string]int64{}
	}
	return state, nil
}

func (s *State) Save() error {
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

// Reset forgets the account, session, pins and seqs but keeps the file location.
func (s *State) Reset() {
	*s = State{path: s.path, AllPin: map[string][]byte{}, AllSeq: map[string]int64{}}
}

func (s *State) IsLoggedIn() bool {
	return s.Token != "" && s.AccountID != ""
}

// HISAR_SECRET_KEY overrides the keychain, for headless machines and scripting.
func LoadSecretKey(accountID string) (crypt.SecretKey, error) {
	encodedSecretKey := os.Getenv("HISAR_SECRET_KEY")
	if encodedSecretKey == "" {
		var err error
		if encodedSecretKey, err = keyring.Get(keyringService, accountID); err != nil {
			return crypt.SecretKey{}, fmt.Errorf("Secret Key not found in keychain: %w", err)
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

func StoreSecretKey(secretKey crypt.SecretKey) error {
	if os.Getenv("HISAR_SECRET_KEY") != "" {
		return nil
	}
	return keyring.Set(keyringService, secretKey.AccountID, secretKey.Encode())
}

func DeleteSecretKey(accountID string) error {
	if os.Getenv("HISAR_SECRET_KEY") != "" {
		return nil
	}
	err := keyring.Delete(keyringService, accountID)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
