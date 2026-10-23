package hisarkit

import (
	"encoding/json"

	"github.com/tryhisar/hisar/core/app"
)

// GetVaultActions returns app.VaultActions as JSON: who may rename, delete or leave the vault, and what deleting it means.
func (hisar *Hisar) GetVaultActions(vaultID string) (string, error) {
	return encodeJSON(hisar.service.GetVaultActions(vaultID))
}

func (hisar *Hisar) RenameVault(vaultID, name string) (string, error) {
	return encodeJSON(hisar.service.RenameVault(vaultID, name))
}

// DeleteVault takes app.VaultDeletionRequest as JSON and returns app.VaultDeletionResult as JSON.
func (hisar *Hisar) DeleteVault(vaultID, requestJSON string) (string, error) {
	var request app.VaultDeletionRequest
	if err := json.Unmarshal([]byte(requestJSON), &request); err != nil {
		return "", err
	}
	result, err := hisar.service.DeleteVault(vaultID, request)
	releaseMemory()
	return encodeJSON(result, err)
}

func (hisar *Hisar) LeaveVault(vaultID string) error {
	return hisar.service.LeaveVault(vaultID)
}

// ListAllRemovedVault returns, once, the vaults another person deleted or removed this user from, as JSON.
func (hisar *Hisar) ListAllRemovedVault() (string, error) {
	return encodeJSON(hisar.service.ListRemovedVaults(), nil)
}

func (hisar *Hisar) GetExportOptions() (string, error) {
	return encodeJSON(hisar.service.GetExportOptions(), nil)
}

// ExportAllData asks for Face ID, or checks the master password when one is given, then writes the export into
// directory and returns app.ExportResult as JSON; the app shares the file and deletes it afterwards.
func (hisar *Hisar) ExportAllData(format, passphrase, password, directory string) (string, error) {
	result, err := app.ExportDataToDirectory(hisar.service, app.ExportRequest{Format: format, Passphrase: passphrase, Password: password}, directory)
	releaseMemory()
	return encodeJSON(result, err)
}

func (hisar *Hisar) DeleteUnreadableItem(vaultID, itemID, revision string) error {
	return hisar.service.DeleteUnreadableItem(vaultID, itemID, revision)
}

// DeleteUnreadableVault and RepairVaultName act only on the unreadable vault record the user reviewed: revision is the
// issue's revision (app.Service.DeleteUnreadableVault).
func (hisar *Hisar) DeleteUnreadableVault(vaultID, revision string) error {
	return hisar.service.DeleteUnreadableVault(vaultID, revision)
}

// RepairVaultName returns app.VaultSummary as JSON.
func (hisar *Hisar) RepairVaultName(vaultID, revision, name string) (string, error) {
	return encodeJSON(hisar.service.RepairVaultName(vaultID, revision, name))
}
