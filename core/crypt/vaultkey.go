package crypt

import "crypto/hpke"

// WrapVaultKey encrypts a vault key to a recipient's KEM public key.
// recipientID is the recipient's account ID, or "recovery" for the recovery keyset.
func WrapVaultKey(recipientKEMPublicKey []byte, vaultID, recipientID string, vaultKey []byte) ([]byte, error) {
	if len(vaultKey) != KeySize {
		return nil, ErrBadKeySize
	}
	recipientPublicKey, err := hybridKeyEncapsulation.NewPublicKey(recipientKEMPublicKey)
	if err != nil {
		return nil, err
	}
	ciphertext, err := hpke.Seal(recipientPublicKey, hpke.HKDFSHA256(), hpke.ChaCha20Poly1305(), encodeContext("hisar/v1/vault-key", vaultID, recipientID), vaultKey)
	if err != nil {
		return nil, err
	}
	return append([]byte{version1}, ciphertext...), nil
}

func (keyset *Keyset) UnwrapVaultKey(vaultID, recipientID string, wrappedKey []byte) ([]byte, error) {
	if len(wrappedKey) < 1 {
		return nil, ErrShortCipher
	}
	if wrappedKey[0] != version1 {
		return nil, ErrBadVersion
	}
	vaultKey, err := hpke.Open(keyset.kemPrivateKey, hpke.HKDFSHA256(), hpke.ChaCha20Poly1305(), encodeContext("hisar/v1/vault-key", vaultID, recipientID), wrappedKey[1:])
	if err != nil || len(vaultKey) != KeySize {
		return nil, ErrDecrypt
	}
	return vaultKey, nil
}
