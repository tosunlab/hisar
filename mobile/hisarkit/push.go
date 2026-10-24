package hisarkit

// SetPushToken takes the push token the OS gave the app ("apns", "apns-sandbox" or "fcm" with the token) and registers
// it for the signed-in account. Call it off the main thread: it reaches the server and the push relay.
func (hisar *Hisar) SetPushToken(platform, token string) error {
	return hisar.service.SetPushToken(platform, token)
}

// SetPushKindEnabled turns one kind of notification ("approval", "security", "sharing") on or off for this phone.
func (hisar *Hisar) SetPushKindEnabled(kind string, isEnabled bool) error {
	return hisar.service.SetPushKindEnabled(kind, isEnabled)
}

// GetPushSettings returns app.PushSettings as JSON: whether notifications can arrive, and the kinds turned on.
func (hisar *Hisar) GetPushSettings() (string, error) {
	return encodeJSON(hisar.service.GetPushSettings(), nil)
}
