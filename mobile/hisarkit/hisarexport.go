package hisarkit

import "github.com/tryhisar/hisar/core/app"

// PrepareHisarExportImport reads a Hisar export (.hisar, or plain .json) the file picker opened and returns
// app.ImportPreview as JSON for vaultID. An encrypted export comes back with isPassphraseRequired; OpenImportFile then
// opens it. MapImportGroup, StartImport and ContinueImport work as for Credential Exchange.
func (hisar *Hisar) PrepareHisarExportImport(vaultID, fileName string, content []byte) (string, error) {
	preview, err := app.PrepareHisarExportImport(hisar.service, vaultID, fileName, content)
	clear(content)
	return encodeJSON(preview, err)
}

// OpenImportFile opens the encrypted Hisar export PrepareHisarExportImport read with its export passphrase and returns
// app.ImportPreview as JSON; after a wrong passphrase the export still waits for the right one.
func (hisar *Hisar) OpenImportFile(passphrase string) (string, error) {
	return encodeJSON(hisar.service.OpenImportFile(passphrase))
}
