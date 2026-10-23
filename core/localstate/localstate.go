// Package localstate persists a device's session state and keeps the Secret Key in the OS keychain.
package localstate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
)

const keyringService = "hisar"

// SecretKeyEnvironmentVariable overrides the keychain, for headless machines and scripting.
const SecretKeyEnvironmentVariable = "HISAR_SECRET_KEY"

// ErrNoSecretService means Linux has no keyring to keep the Secret Key: no Secret Service on the session D-Bus.
var ErrNoSecretService = errors.New("no keyring is running to keep the Secret Key: start GNOME Keyring or KWallet " +
	"(any Secret Service), or set " + SecretKeyEnvironmentVariable + " to your Secret Key")

// classifyKeyringError names a missing Secret Service, which go-keyring reports only as a D-Bus error.
func classifyKeyringError(err error, checkSecretService func() error) error {
	if err == nil || errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	if checkSecretService() != nil {
		return fmt.Errorf("%w (%v)", ErrNoSecretService, err)
	}
	return err
}

type State struct {
	Server              string            `json:"server"`
	Email               string            `json:"email"`
	AccountID           string            `json:"accountId"`
	Token               string            `json:"token"`
	DeviceID            string            `json:"deviceId"`
	Pins                map[string][]byte `json:"pins,omitempty"`
	Seqs                map[string]int64  `json:"seqs,omitempty"`
	RecoveryOperationID string            `json:"recoveryOperationId,omitempty"`

	// BiometricUnlock is the account unlock key sealed with a key kept in the OS keychain.
	BiometricUnlock      []byte    `json:"biometricUnlock,omitempty"`
	LastPasswordUnlockAt time.Time `json:"lastPasswordUnlockAt,omitzero"`

	PairedBrowsers []PairedBrowser `json:"pairedBrowsers,omitempty"`
	// IsExactHostMatch limits browser autofill to the item's exact host instead of its registrable domain.
	IsExactHostMatch bool `json:"exactHostMatch,omitempty"`
	// AgentVaultIDs lists the vaults coding agents may see through hisar mcp; every other vault is invisible to them.
	AgentVaultIDs []string `json:"agentVaults,omitempty"`

	// Settings are this device's preferences; Reset keeps them. Embedded, so state.json keeps its flat keys.
	Settings

	path string
}

// Settings are device preferences of the apps that outlive signing out.
type Settings struct {
	// IsAutomaticUpdateCheckDisabled turns off the desktop app's automatic update check.
	IsAutomaticUpdateCheckDisabled bool `json:"automaticUpdateCheckDisabled,omitempty"`
	// IsCommandLineIntegrationDisabled turns off the desktop app's CLI socket.
	IsCommandLineIntegrationDisabled bool `json:"commandLineIntegrationDisabled,omitempty"`
	// IsCompactSSHApproval shows SSH approvals as a card at the top right.
	IsCompactSSHApproval bool `json:"compactSSHApproval,omitempty"`
	// AutoLockSeconds is the chosen auto-lock delay (0: the default).
	AutoLockSeconds int `json:"autoLockSeconds,omitempty"`
	// IsPhoneApprovalDisabled stops sending approval dialogs to the account's phones.
	IsPhoneApprovalDisabled bool `json:"phoneApprovalDisabled,omitempty"`
	// SigningRememberSeconds is how long an approved commit signature may be remembered (0: always ask).
	SigningRememberSeconds int `json:"signingRememberSeconds,omitempty"`
	// SSHRememberSeconds is how long a remembered SSH approval lasts (0: the default, 1 hour; -1: until Hisar locks).
	SSHRememberSeconds int `json:"sshRememberSeconds,omitempty"`
	// IsOpenAtLoginConfigured records that the desktop app registered its login item once.
	IsOpenAtLoginConfigured bool `json:"openAtLoginConfigured,omitempty"`
	// IsOpenAtLoginDisabled is set when the user turned "Open Hisar at login" off.
	IsOpenAtLoginDisabled bool `json:"openAtLoginDisabled,omitempty"`
	// Appearance ("system", "light", "dark") and Color ("neutral", "blue", "purple", "green") are this device's theme;
	// empty means the defaults.
	Appearance string `json:"appearance,omitempty"`
	Color      string `json:"color,omitempty"`
	// IsWebsiteIconsDisabled stops this device from fetching website icons for logins.
	IsWebsiteIconsDisabled bool `json:"websiteIconsDisabled,omitempty"`
	// LastServer is the server this device last signed in to; signing out keeps it, so the sign-in form opens on it
	// rather than the default server. No email or other account data is kept.
	LastServer string `json:"lastServer,omitempty"`
	// AgentHosts are the apps a coding agent harness ran in. The desktop app answers their callers without a terminal
	// and without a harness one connection at a time, and never as launchers.
	AgentHosts []AgentHost `json:"agentHosts,omitempty"`
}

// AgentHost is an app as the desktop app verified its code signature: bundle path and Team ID.
type AgentHost struct {
	Path   string `json:"path"`
	TeamID string `json:"teamId"`
}

type PairedBrowser struct {
	InstanceID string    `json:"instanceId"`
	Browser    string    `json:"browser"`
	TokenHash  []byte    `json:"tokenHash"`
	PairedAt   time.Time `json:"pairedAt"`
	LastUsedAt time.Time `json:"lastUsedAt"`
}

func (s *State) GetSeq(vaultID string) int64      { return s.Seqs[vaultID] }
func (s *State) SetSeq(vaultID string, seq int64) { s.Seqs[vaultID] = seq }

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
	encodedState, err := readPrivateFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if len(encodedState) > 0 {
		if err := json.Unmarshal(encodedState, state); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	if state.Pins == nil {
		state.Pins = map[string][]byte{}
	}
	if state.Seqs == nil {
		state.Seqs = map[string]int64{}
	}
	return state, nil
}

// readPrivateFile reads the state and first makes a file others can read readable only by the user, since it holds the
// bearer token; Save would only replace it at the next change.
func readPrivateFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Mode().Perm()&0o077 != 0 {
		if err := file.Chmod(0o600); err != nil {
			return nil, fmt.Errorf("%s: make it readable only by you: %w", path, err)
		}
	}
	return io.ReadAll(file)
}

// Save writes a new file readable only by the user and renames it over the state, so it never writes through a file
// another process put in the directory.
func (s *State) Save() error {
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	encodedState, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	temporaryFile, err := os.CreateTemp(directory, filepath.Base(s.path)+".*.tmp")
	if err != nil {
		return err
	}
	_, err = temporaryFile.Write(encodedState)
	if err == nil {
		err = temporaryFile.Sync()
	}
	if closeErr := temporaryFile.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(temporaryFile.Name(), s.path)
	}
	if err != nil {
		os.Remove(temporaryFile.Name())
	}
	return err
}

// Reset forgets the account, session, pins and seqs but keeps the file location and the device preferences.
// Reset forgets the account and keeps the device settings, remembering the server the account was on as LastServer
// (a state already signed out keeps the one it has).
func (s *State) Reset() {
	settings := s.Settings
	if s.Server != "" {
		settings.LastServer = s.Server
	}
	*s = State{path: s.path, Pins: map[string][]byte{}, Seqs: map[string]int64{}, Settings: settings}
}

// IsMemberKeyChanged reports whether a signing key differs from the one pinned for the account.
func (s *State) IsMemberKeyChanged(accountID string, signingPublicKey []byte) bool {
	pinned, found := s.Pins[accountID]
	return found && !bytes.Equal(pinned, signingPublicKey)
}

// CheckMemberPin trusts a member's signing key on first use and refuses it if it changes later.
// The caller saves the state.
func (s *State) CheckMemberPin(member *apitypes.Member) error {
	if s.IsMemberKeyChanged(member.AccountID, member.PublicKeys.Sign) {
		return fmt.Errorf("the signing key of %s changed since you last shared with them; stopping", member.Email)
	}
	s.Pins[member.AccountID] = append([]byte(nil), member.PublicKeys.Sign...)
	return nil
}

func (s *State) IsLoggedIn() bool {
	return s.Token != "" && s.AccountID != ""
}

// LoadSecretKey reads the Secret Key from SecretKeyEnvironmentVariable, or else the keychain.
func LoadSecretKey(accountID string) (crypt.SecretKey, error) {
	encodedSecretKey := os.Getenv(SecretKeyEnvironmentVariable)
	if encodedSecretKey == "" {
		var err error
		if encodedSecretKey, err = keyring.Get(keyringService, accountID); err != nil {
			if err := classifyKeyringError(err, checkSecretService); errors.Is(err, ErrNoSecretService) {
				return crypt.SecretKey{}, err
			}
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
	if os.Getenv(SecretKeyEnvironmentVariable) != "" {
		return nil
	}
	return classifyKeyringError(keyring.Set(keyringService, secretKey.AccountID, secretKey.Encode()), checkSecretService)
}

func DeleteSecretKey(accountID string) error {
	if os.Getenv(SecretKeyEnvironmentVariable) != "" {
		return nil
	}
	err := keyring.Delete(keyringService, accountID)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return classifyKeyringError(err, checkSecretService)
}
