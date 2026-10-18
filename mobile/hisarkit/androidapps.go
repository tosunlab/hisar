package hisarkit

import "github.com/tryhisar/hisar/core/app"

// ListAllLoginForAndroidApp returns the logins linked to an Android app ([]browserprotocol.Login) as JSON. The app is
// its package name and the hex SHA-256 of its signing certificate, with or without colons; a login matches only both.
func (hisar *Hisar) ListAllLoginForAndroidApp(packageName, certificateSHA256 string) (string, error) {
	return encodeJSON(app.ListLoginsForAndroidApp(hisar.service, packageName, certificateSHA256))
}

// GetLoginForAndroidApp returns {"username", "password", "hasOneTimePassword"} as JSON after checking the login is
// linked to the app again.
func (hisar *Hisar) GetLoginForAndroidApp(vaultID, itemID, packageName, certificateSHA256 string) (string, error) {
	return encodeJSON(app.GetLoginForAndroidApp(hisar.service, vaultID, itemID, packageName, certificateSHA256))
}

// GetOneTimeCodeForAndroidApp returns app.OneTimeCode as JSON after checking the login is linked to the app, as
// GetLoginForAndroidApp does.
func (hisar *Hisar) GetOneTimeCodeForAndroidApp(vaultID, itemID, packageName, certificateSHA256 string) (string, error) {
	return encodeJSON(app.GetOneTimeCodeForAndroidApp(hisar.service, vaultID, itemID, packageName, certificateSHA256))
}

// AddAndroidApp links a login to an Android app ("Always use for this app") and returns the item as app.ItemDetail JSON.
func (hisar *Hisar) AddAndroidApp(vaultID, itemID, packageName, certificateSHA256 string) (string, error) {
	return encodeJSON(hisar.service.AddAndroidApp(vaultID, itemID, packageName, certificateSHA256))
}

// RemoveAndroidApp unlinks a login from an Android app and returns the item as app.ItemDetail JSON.
func (hisar *Hisar) RemoveAndroidApp(vaultID, itemID, packageName, certificateSHA256 string) (string, error) {
	return encodeJSON(hisar.service.RemoveAndroidApp(vaultID, itemID, packageName, certificateSHA256))
}

// ListAllLoginWebsite returns the hosts of the logins' websites ([]app.LoginWebsite: {"host", "registrableDomain"}) as
// JSON, for Android to check which of them vouch for an app in their Digital Asset Links.
func (hisar *Hisar) ListAllLoginWebsite() (string, error) {
	return encodeJSON(app.ListLoginWebsites(hisar.service))
}
