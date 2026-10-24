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
	}{kit.SecretKey, kit.Path}, err)
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
