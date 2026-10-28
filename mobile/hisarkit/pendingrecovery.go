package hisarkit

import (
	"encoding/base64"

	"github.com/tryhisar/hisar/core/app"
)

func (adapter secretKeyStoreAdapter) StoreRecoveryKey(id string, key []byte) error {
	return adapter.store.StoreSecretKey("pending-recovery/"+id, base64.RawStdEncoding.EncodeToString(key))
}

func (adapter secretKeyStoreAdapter) LoadRecoveryKey(id string) ([]byte, error) {
	value, err := adapter.store.LoadSecretKey("pending-recovery/" + id)
	if err != nil {
		return nil, err
	}
	return base64.RawStdEncoding.DecodeString(value)
}

func (adapter secretKeyStoreAdapter) DeleteRecoveryKey(id string) error {
	return adapter.store.DeleteSecretKey("pending-recovery/" + id)
}

func (hisar *Hisar) ListPendingRecovery() (string, error) {
	return encodeJSON(hisar.service.ListPendingRecovery())
}

func (hisar *Hisar) ResumePendingRecovery(operationID, password string) (string, error) {
	defer releaseMemory()
	return encodeJSON(hisar.service.ResumePendingRecovery(operationID, password))
}

func (hisar *Hisar) ExportPendingRecoveryEmergencyKit(operationID, password, directory string) (string, error) {
	defer releaseMemory()
	kit, err := app.ExportPendingRecoveryEmergencyKitToDirectory(hisar.service, operationID, password, directory)
	return encodeJSON(struct {
		SecretKey string `json:"secretKey"`
		Path      string `json:"path"`
		ExportID  string `json:"exportId"`
	}{kit.SecretKey, kit.Path, kit.ExportID}, err)
}

// RecordPendingRecoveryEmergencyKitDelivery records the delivery the platform confirmed for exportId of
// ExportPendingRecoveryEmergencyKit: method is "file", "print" or "share".
func (hisar *Hisar) RecordPendingRecoveryEmergencyKitDelivery(exportID, method string) error {
	return hisar.service.RecordPendingRecoveryEmergencyKitDelivery(exportID, method)
}

func (hisar *Hisar) AcknowledgePendingRecovery(operationID, password string) error {
	defer releaseMemory()
	return hisar.service.AcknowledgePendingRecovery(operationID, password)
}

func (hisar *Hisar) ResumePendingRecoveryWithSecretKey(operationID, password, secretKey, priorSecretKey string) (string, error) {
	defer releaseMemory()
	return encodeJSON(hisar.service.ResumePendingRecoveryWithSecretKey(operationID, password, secretKey, priorSecretKey))
}

func (hisar *Hisar) DiscardPreparedRecovery(operationID, password string) error {
	defer releaseMemory()
	return hisar.service.DiscardPreparedRecovery(operationID, password)
}

// ExportNewAccountEmergencyKit writes the owed Emergency Kit into directory while this unlock is authorized and returns
// {"secretKey", "emergencyKitPath", "exportId"} as JSON; the app deletes the file once it is shared. Writing it records
// nothing: RecordNewAccountEmergencyKitDelivery records the delivery the platform confirmed for exportId.
func (hisar *Hisar) ExportNewAccountEmergencyKit(directory string) (string, error) {
	kit, err := app.ExportNewAccountEmergencyKitToDirectory(hisar.service, directory)
	if err != nil {
		return "", err
	}
	return encodeJSON(emergencyKitResult{SecretKey: kit.SecretKey, EmergencyKitPath: kit.Path, ExportID: kit.ExportID}, nil)
}

// RecordNewAccountEmergencyKitDelivery records the delivery the platform confirmed for exportId of
// ExportNewAccountEmergencyKit or ReplaceSecretKey: method is "file", "print" or "share".
func (hisar *Hisar) RecordNewAccountEmergencyKitDelivery(exportID, method string) error {
	return hisar.service.RecordNewAccountEmergencyKitDelivery(exportID, method)
}

// AuthorizeNewAccountEmergencyKit lets this unlock save the owed Emergency Kit after a biometric unlock.
func (hisar *Hisar) AuthorizeNewAccountEmergencyKit(password string) error {
	defer releaseMemory()
	return hisar.service.AuthorizeNewAccountEmergencyKit(password)
}

// AuthorizeNewAccountEmergencyKitWithBiometric is AuthorizeNewAccountEmergencyKit with the biometric check.
func (hisar *Hisar) AuthorizeNewAccountEmergencyKitWithBiometric() error {
	return hisar.service.AuthorizeNewAccountEmergencyKitWithBiometric()
}

// RevealNewAccountSecretKey returns app.RevealedSecretKey ({"secretKey", "qrPayload", "qrCode"}) as JSON.
func (hisar *Hisar) RevealNewAccountSecretKey() (string, error) {
	return encodeJSON(hisar.service.RevealNewAccountSecretKey())
}

// PostponeNewAccountEmergencyKit is Save later.
func (hisar *Hisar) PostponeNewAccountEmergencyKit() error {
	return hisar.service.PostponeNewAccountEmergencyKit()
}

// HideEmergencyKitReminder is the reminder's Not now, until the next lock.
func (hisar *Hisar) HideEmergencyKitReminder() {
	hisar.service.HideEmergencyKitReminder()
}

// AcknowledgeNewAccountRecovery ends the pending sign-up operation once its kit was saved, for "I saved my Emergency Kit".
func (hisar *Hisar) AcknowledgeNewAccountRecovery() error {
	return hisar.service.AcknowledgeNewAccountRecovery()
}
