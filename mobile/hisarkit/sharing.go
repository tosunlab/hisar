package hisarkit

import "github.com/tryhisar/hisar/desktop/vault"

// GetVaultSharing returns vault.VaultSharing as JSON after completing accepted shares; a personal vault is refused.
func (hisar *Hisar) GetVaultSharing(vaultID string) (string, error) {
	return encodeJSON(vault.GetVaultSharing(hisar.service, vaultID))
}
