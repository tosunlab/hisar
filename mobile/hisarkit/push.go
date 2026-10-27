package hisarkit

// SetPushToken takes the push token the OS gave the app ("apns" or "apns-sandbox" with the token) and registers
// it for the signed-in account. Call it off the main thread: it reaches the server and the push relay.
func (hisar *Hisar) SetPushToken(platform, token string) error {
	return hisar.service.SetPushToken(platform, token)
}

// SetPushPayloadKey takes the public key and authentication secret, base64url, that the iPhone's notification service
// extension opens push details with. Call it before SetPushToken, off the main thread: it may reach the server.
func (hisar *Hisar) SetPushPayloadKey(p256dh, auth string) error {
	return hisar.service.SetPushPayloadKey(p256dh, auth)
}

// SetPushKindEnabled turns one kind of notification ("approval", "security", "sharing") on or off for this phone.
func (hisar *Hisar) SetPushKindEnabled(kind string, isEnabled bool) error {
	return hisar.service.SetPushKindEnabled(kind, isEnabled)
}

// GetPushSettings returns app.PushSettings as JSON: whether notifications can arrive, and the kinds turned on.
func (hisar *Hisar) GetPushSettings() (string, error) {
	return encodeJSON(hisar.service.GetPushSettings(), nil)
}

// SetWebPushEndpoint takes the Web Push subscription an Android phone's UnifiedPush distributor gave the app: the
// endpoint URL, and the phone's public key and authentication secret in base64url. Call it off the main thread: it
// reaches the server.
func (hisar *Hisar) SetWebPushEndpoint(endpoint, p256dh, auth string) error {
	return hisar.service.SetWebPushEndpoint(endpoint, p256dh, auth)
}

// RemoveWebPushEndpoint forgets the subscription once the distributor ended it, and tells the server. Call it off the
// main thread.
func (hisar *Hisar) RemoveWebPushEndpoint() error {
	return hisar.service.RemoveWebPushEndpoint()
}
