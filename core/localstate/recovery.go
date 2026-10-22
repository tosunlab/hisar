package localstate

import (
	"encoding/base64"
	"errors"

	"github.com/zalando/go-keyring"
)

func StoreRecoveryKey(operationID string, key []byte) error {
	return classifyKeyringError(keyring.Set(keyringService, "pending-recovery/"+operationID, base64.RawStdEncoding.EncodeToString(key)), checkSecretService)
}

func LoadRecoveryKey(operationID string) ([]byte, error) {
	value, err := keyring.Get(keyringService, "pending-recovery/"+operationID)
	if err != nil {
		return nil, err
	}
	return base64.RawStdEncoding.DecodeString(value)
}

func DeleteRecoveryKey(operationID string) error {
	err := keyring.Delete(keyringService, "pending-recovery/"+operationID)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
