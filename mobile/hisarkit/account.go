package hisarkit

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/tryhisar/hisar/desktop/vault"
)

type secretKeyReplacement struct {
	vault.CredentialChangeResult
	EmergencyKitPath string `json:"emergencyKitPath"`
}

// ChangeMasterPassword returns vault.CredentialChangeResult as JSON; every other device is signed out.
func (hisar *Hisar) ChangeMasterPassword(currentPassword, newPassword string) (string, error) {
	defer releaseMemory()
	return encodeJSON(hisar.service.ChangeMasterPassword(currentPassword, newPassword))
}

// ReplaceSecretKey also writes the new Emergency Kit into directory; the app shares it and then deletes the file.
func (hisar *Hisar) ReplaceSecretKey(password, directory string) (string, error) {
	defer releaseMemory()
	result, err := hisar.service.ReplaceSecretKey(password)
	if err != nil {
		return "", err
	}
	replacement := secretKeyReplacement{CredentialChangeResult: result}
	export, err := vault.ExportNewSecretKeyEmergencyKit(hisar.service)
	if err == nil {
		err = os.MkdirAll(directory, 0o700)
	}
	if err == nil {
		path := filepath.Join(directory, emergencyKitFileName)
		if os.WriteFile(path, export.Document, 0o600) == nil {
			replacement.EmergencyKitPath = path
		}
	}
	return encodeJSON(replacement, nil)
}

// PreviewAccountDeletion returns vault.AccountDeletionPreview as JSON.
func (hisar *Hisar) PreviewAccountDeletion() (string, error) {
	return encodeJSON(hisar.service.PreviewAccountDeletion())
}

// DeleteAccount returns the signed-out vault.Status as JSON; the app then removes its AutoFill identities.
func (hisar *Hisar) DeleteAccount(password, email string) (string, error) {
	defer releaseMemory()
	if password == "" {
		return "", errors.New("enter your master password")
	}
	return encodeJSON(hisar.service.DeleteAccount(password, email))
}

func (hisar *Hisar) DismissSignedOutReason() string {
	encoded, _ := encodeJSON(hisar.service.DismissSignedOutReason(), nil)
	return encoded
}
