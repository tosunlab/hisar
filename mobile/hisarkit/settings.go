package hisarkit

import "github.com/tryhisar/hisar/core/app"

// SetAutoLock takes 30, 60, 300, 600 or 3600 seconds and returns app.Status as JSON.
func (hisar *Hisar) SetAutoLock(seconds int) (string, error) {
	return encodeJSON(hisar.service.SetAutoLock(seconds))
}

// ListAllDevice returns the account's signed-in devices ([]app.Device) as JSON, this device first.
func (hisar *Hisar) ListAllDevice() (string, error) {
	return encodeJSON(hisar.service.ListDevices())
}

// RevokeDevice signs out another device; this phone signs out with SignOut.
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
	kit, err := app.ExportEmergencyKitToDirectory(hisar.service, password, directory)
	if err != nil {
		return "", err
	}
	return encodeJSON(emergencyKitResult{SecretKey: kit.SecretKey, EmergencyKitPath: kit.Path}, nil)
}

// CopySecretReference copies hisar://<vault>/<item>/<field> and returns it; fieldName is a built-in field or a custom field's label.
func (hisar *Hisar) CopySecretReference(vaultID, itemID, fieldName string) (string, error) {
	return hisar.service.CopySecretReference(vaultID, itemID, fieldName)
}

// The Mac's switch for sending requests to phones is the same state field; on the phone it means this phone answers none.
func (hisar *Hisar) IsMacApprovalAllowed() bool {
	return app.NewApprovalPreference(hisar.service).IsPhoneApproval()
}

func (hisar *Hisar) SetMacApprovalAllowed(isAllowed bool) error {
	return app.NewApprovalPreference(hisar.service).SetPhoneApproval(isAllowed)
}

func (hisar *Hisar) IsExactHostMatch() bool {
	return app.IsExactHostMatch(hisar.service)
}

func (hisar *Hisar) SetExactHostMatch(isExactHostMatch bool) error {
	return app.SetExactHostMatch(hisar.service, isExactHostMatch)
}

// GetTheme returns this phone's app.Theme as JSON; the AutoFill extension reads the same state file.
func (hisar *Hisar) GetTheme() string {
	theme, _ := encodeJSON(hisar.service.GetTheme(), nil)
	return theme
}

// SetTheme takes "system", "light" or "dark" and "neutral", "blue", "purple" or "green" and returns app.Theme as JSON.
func (hisar *Hisar) SetTheme(appearance, color string) (string, error) {
	return encodeJSON(hisar.service.SetTheme(appearance, color))
}
