package crypt

type ItemPart string

const (
	Overview ItemPart = "overview"
	Details  ItemPart = "details"
)

func itemAAD(vaultID, itemID, revision string, part ItemPart) []byte {
	return bind("hisar/v1/item", vaultID, itemID, revision, string(part))
}

func SealItemPart(vaultKey []byte, vaultID, itemID, revision string, part ItemPart, plaintext []byte) ([]byte, error) {
	return Seal(vaultKey, plaintext, itemAAD(vaultID, itemID, revision, part))
}

func OpenItemPart(vaultKey []byte, vaultID, itemID, revision string, part ItemPart, ciphertext []byte) ([]byte, error) {
	return Open(vaultKey, ciphertext, itemAAD(vaultID, itemID, revision, part))
}

func SealVaultName(vaultKey []byte, vaultID string, name []byte) ([]byte, error) {
	return Seal(vaultKey, name, bind("hisar/v1/vault-name", vaultID))
}

func OpenVaultName(vaultKey []byte, vaultID string, ciphertext []byte) ([]byte, error) {
	return Open(vaultKey, ciphertext, bind("hisar/v1/vault-name", vaultID))
}
