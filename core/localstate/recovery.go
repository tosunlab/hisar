package localstate

import (
	"encoding/base64"
	"errors"
	"os"

	"github.com/zalando/go-keyring"
)

// errHeadlessRecovery keeps a pending recovery operation's key out of the keychain while SecretKeyEnvironmentVariable
// stands in for it, as it does for the Secret Key: a headless run then takes the saved Emergency Kit path.
var errHeadlessRecovery = errors.New("localstate: " + SecretKeyEnvironmentVariable + " is set, so pending recovery does not use the keychain")

func StoreRecoveryKey(operationID string, key []byte) error {
	if os.Getenv(SecretKeyEnvironmentVariable) != "" {
		return errHeadlessRecovery
	}
	return classifyKeyringError(keyring.Set(keyringService, "pending-recovery/"+operationID, base64.RawStdEncoding.EncodeToString(key)), checkSecretService)
}

func LoadRecoveryKey(operationID string) ([]byte, error) {
	if os.Getenv(SecretKeyEnvironmentVariable) != "" {
		return nil, errHeadlessRecovery
	}
	value, err := keyring.Get(keyringService, "pending-recovery/"+operationID)
	if err != nil {
		return nil, err
	}
	return base64.RawStdEncoding.DecodeString(value)
}

func DeleteRecoveryKey(operationID string) error {
	if os.Getenv(SecretKeyEnvironmentVariable) != "" {
		return nil
	}
	err := keyring.Delete(keyringService, "pending-recovery/"+operationID)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
