package hisarkit

import "github.com/tryhisar/hisar/core/app"

// GetVaultSharing returns app.VaultSharing as JSON after completing accepted shares; a personal vault is refused.
func (hisar *Hisar) GetVaultSharing(vaultID string) (string, error) {
	return encodeJSON(app.GetVaultSharing(hisar.service, vaultID))
}

func (hisar *Hisar) ConfirmShareRecipient(invitationID, reviewToken string) error {
	return hisar.service.ConfirmShareRecipient(invitationID, reviewToken)
}
