// Package localstate persists a device's session state and keeps the Secret Key in the OS keychain.
package localstate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/proto"
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

	// BiometricUnlock is the account unlock key sealed with a key kept in the OS keychain.
	BiometricUnlock      []byte    `json:"biometricUnlock,omitempty"`
	LastPasswordUnlockAt time.Time `json:"lastPasswordUnlockAt,omitzero"`

	AllPairedBrowser []PairedBrowser `json:"pairedBrowsers,omitempty"`
	// IsExactHostMatch limits browser autofill to the item's exact host instead of its registrable domain.
	IsExactHostMatch bool `json:"exactHostMatch,omitempty"`
	// IsAutomaticUpdateCheckDisabled is a device preference of the desktop app, so Reset keeps it.
	IsAutomaticUpdateCheckDisabled bool `json:"automaticUpdateCheckDisabled,omitempty"`
	// IsCommandLineIntegrationDisabled turns off the desktop app's CLI socket; a device preference that Reset keeps.
	IsCommandLineIntegrationDisabled bool `json:"commandLineIntegrationDisabled,omitempty"`
	// AutoLockSeconds is the desktop app's chosen auto-lock delay (0: the default); a device preference that Reset keeps.
	AutoLockSeconds int `json:"autoLockSeconds,omitempty"`

	path string
}

type PairedBrowser struct {
	InstanceID string    `json:"instanceId"`
	Browser    string    `json:"browser"`
	TokenHash  []byte    `json:"tokenHash"`
	PairedAt   time.Time `json:"pairedAt"`
	LastUsedAt time.Time `json:"lastUsedAt"`
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

// Reset forgets the account, session, pins and seqs but keeps the file location and the update preference.
func (s *State) Reset() {
	*s = State{
		path: s.path, AllPin: map[string][]byte{}, AllSeq: map[string]int64{},
		IsAutomaticUpdateCheckDisabled: s.IsAutomaticUpdateCheckDisabled, IsCommandLineIntegrationDisabled: s.IsCommandLineIntegrationDisabled,
		AutoLockSeconds: s.AutoLockSeconds,
	}
}

// IsMemberKeyChanged reports whether a signing key differs from the one pinned for the account.
func (s *State) IsMemberKeyChanged(accountID string, signingPublicKey []byte) bool {
	pinned, found := s.AllPin[accountID]
	return found && !bytes.Equal(pinned, signingPublicKey)
}

// CheckMemberPin trusts a member's signing key on first use and refuses it if it changes later.
// The caller saves the state.
func (s *State) CheckMemberPin(member *proto.Member) error {
	if s.IsMemberKeyChanged(member.AccountID, member.PublicKeys.Sign) {
		return fmt.Errorf("the signing key of %s changed since you last shared with them; stopping", member.Email)
	}
	s.AllPin[member.AccountID] = append([]byte(nil), member.PublicKeys.Sign...)
	return nil
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
