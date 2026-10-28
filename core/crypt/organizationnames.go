package crypt

const organizationNamesLabel = "hisar/v1/organization-names"

// SealOrganizationNames seals this device's organization names with K_user, bound to the account ID.
func SealOrganizationNames(userKey []byte, accountID string, plaintext []byte) ([]byte, error) {
	return Seal(userKey, plaintext, encodeContext(organizationNamesLabel, accountID))
}

func OpenOrganizationNames(userKey []byte, accountID string, ciphertext []byte) ([]byte, error) {
	return Open(userKey, ciphertext, encodeContext(organizationNamesLabel, accountID))
}
