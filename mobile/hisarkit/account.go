package hisarkit

import (
	"errors"

	"github.com/tryhisar/hisar/core/app"
)

type secretKeyReplacement struct {
	app.CredentialChangeResult
	EmergencyKitPath string `json:"emergencyKitPath"`
}

// ChangeMasterPassword returns app.CredentialChangeResult as JSON; every other device is signed out.
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
	if result.PendingOperationID != "" {
		return encodeJSON(replacement, nil)
	}
	kit, kitErr := app.ExportNewSecretKeyEmergencyKitToDirectory(hisar.service, result, directory)
	if kitErr != nil {
		replacement.LocalError = "Your Secret Key changed, but its Emergency Kit could not be saved. Resume recovery delivery."
	} else {
		replacement.EmergencyKitPath = kit.Path
	}
	return encodeJSON(replacement, nil)
}

// PreviewAccountDeletion returns app.AccountDeletionPreview as JSON.
func (hisar *Hisar) PreviewAccountDeletion() (string, error) {
	return encodeJSON(hisar.service.PreviewAccountDeletion())
}

// DeleteAccount returns the signed-out app.Status as JSON; the app then removes its AutoFill identities.
func (hisar *Hisar) DeleteAccount(password, email string) (string, error) {
	defer releaseMemory()
	if password == "" {
		return "", errors.New("enter your master password")
	}
	return encodeJSON(hisar.service.DeleteAccount(password, email))
}

func (hisar *Hisar) DeleteAccountWithPlan(password, email, planVersion string) (string, error) {
	defer releaseMemory()
	if password == "" {
		return "", errors.New("enter your master password")
	}
	return encodeJSON(hisar.service.DeleteAccountWithPlan(password, email, planVersion))
}

func (hisar *Hisar) DismissSignedOutReason() string {
	encoded, _ := encodeJSON(hisar.service.DismissSignedOutReason(), nil)
	return encoded
}
