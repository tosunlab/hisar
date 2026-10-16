package crypt

const accountPreferencesLabel = "hisar/v1/account-preferences"

// SealAccountPreferences seals the account's preferences record with K_user, bound to the account ID.
func SealAccountPreferences(userKey []byte, accountID string, plaintext []byte) ([]byte, error) {
	return Seal(userKey, plaintext, encodeContext(accountPreferencesLabel, accountID))
}

func OpenAccountPreferences(userKey []byte, accountID string, ciphertext []byte) ([]byte, error) {
	return Open(userKey, ciphertext, encodeContext(accountPreferencesLabel, accountID))
}
