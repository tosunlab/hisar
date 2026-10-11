package hisarkit

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/tryhisar/hisar/desktop/vault"
)

const emergencyKitFileName = "Hisar Emergency Kit.pdf"

var errMacApprovalDisabled = errors.New("approving Mac requests is turned off in Settings on this iPhone")

// SetAutoLock takes 30, 60, 300, 600 or 3600 seconds and returns vault.Status as JSON.
func (hisar *Hisar) SetAutoLock(seconds int) (string, error) {
	return encodeJSON(hisar.service.SetAutoLock(seconds))
}

// ListAllDevice returns the account's signed-in devices ([]vault.Device) as JSON, this device first.
func (hisar *Hisar) ListAllDevice() (string, error) {
	return encodeJSON(hisar.service.ListAllDevice())
}

// RevokeDevice signs out another device; this iPhone signs out with SignOut.
func (hisar *Hisar) RevokeDevice(deviceID string) error {
	return hisar.service.RevokeDevice(deviceID)
}

type emergencyKitResult struct {
	SecretKey        string `json:"secretKey"`
	EmergencyKitPath string `json:"emergencyKitPath"`
}

// ExportEmergencyKit checks the master password, writes the Emergency Kit PDF into directory and returns
// {"secretKey", "emergencyKitPath"} as JSON; the app deletes the file once it is shared.
func (hisar *Hisar) ExportEmergencyKit(password, directory string) (string, error) {
	defer releaseMemory()
	export, err := vault.ExportEmergencyKit(hisar.service, password)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(directory, emergencyKitFileName)
	if err := os.WriteFile(path, export.Document, 0o600); err != nil {
		return "", errors.New("could not write the Emergency Kit")
	}
	return encodeJSON(emergencyKitResult{SecretKey: export.SecretKey, EmergencyKitPath: path}, nil)
}

// CopySecretReference copies hisar://<vault>/<item>/<field> and returns it; fieldName is a built-in field or a custom field's label.
func (hisar *Hisar) CopySecretReference(vaultID, itemID, fieldName string) (string, error) {
	return hisar.service.CopySecretReference(vaultID, itemID, fieldName)
}

// The Mac's switch for sending requests to phones is the same state field; on the phone it means this phone answers none.
func (hisar *Hisar) IsMacApprovalAllowed() bool {
	return vault.CreateApprovalPreference(hisar.service).IsPhoneApproval()
}

func (hisar *Hisar) SetMacApprovalAllowed(isAllowed bool) error {
	return vault.CreateApprovalPreference(hisar.service).SetPhoneApproval(isAllowed)
}

// GetTheme returns this iPhone's vault.Theme as JSON; the AutoFill extension reads the same state file.
func (hisar *Hisar) GetTheme() string {
	theme, _ := encodeJSON(hisar.service.GetTheme(), nil)
	return theme
}

// SetTheme takes "system", "light" or "dark" and "neutral", "blue", "purple" or "green" and returns vault.Theme as JSON.
func (hisar *Hisar) SetTheme(appearance, color string) (string, error) {
	return encodeJSON(hisar.service.SetTheme(appearance, color))
}
