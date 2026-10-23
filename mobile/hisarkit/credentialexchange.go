package hisarkit

import "github.com/tryhisar/hisar/core/app"

// PrepareCredentialExchangeImport reads the Credential Exchange JSON another app exported to Hisar (on iOS the
// ASExportedCredentialData encoded by JSONEncoder with dates as seconds since 1970, on Android the responseJson of the
// credential transfer) and returns app.ImportPreview as JSON for vaultID.
func (hisar *Hisar) PrepareCredentialExchangeImport(vaultID string, credentialData []byte) (string, error) {
	preview, err := app.PrepareCredentialExchangeImport(hisar.service, vaultID, credentialData)
	clear(credentialData)
	return encodeJSON(preview, err)
}

// PreviewImport returns app.ImportPreview as JSON for another vault.
func (hisar *Hisar) PreviewImport(vaultID string) (string, error) {
	return encodeJSON(hisar.service.PreviewImport(vaultID))
}

// MapImportGroup sends one source group of the prepared import, such as one Credential Exchange collection, to vaultID
// and returns app.ImportPreview as JSON with that vault's current members; StartImport then takes the preview's vaultId,
// which is empty when the groups go to different vaults.
func (hisar *Hisar) MapImportGroup(groupID, vaultID string) (string, error) {
	return encodeJSON(hisar.service.MapImportGroup(groupID, vaultID))
}

// StartImport returns app.ImportProgress as JSON; ContinueImport saves the next batch until IsDone.
func (hisar *Hisar) StartImport(vaultID string, isSkippingDuplicate bool) (string, error) {
	return encodeJSON(hisar.service.StartImport(vaultID, isSkippingDuplicate))
}

func (hisar *Hisar) ContinueImport() (string, error) {
	return encodeJSON(hisar.service.ContinueImport())
}

func (hisar *Hisar) CancelImport() {
	hisar.service.CancelImport()
}
