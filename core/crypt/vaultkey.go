package crypt

import "crypto/hpke"

// WrapVaultKey encrypts a vault key to a recipient's KEM public key.
// recipientID is the recipient's account ID, or "recovery" for the recovery keyset.
func WrapVaultKey(recipientKEM []byte, vaultID, recipientID string, vaultKey []byte) ([]byte, error) {
	if len(vaultKey) != KeySize {
		return nil, ErrBadKeySize
	}
	pub, err := kem.NewPublicKey(recipientKEM)
	if err != nil {
		return nil, err
	}
	ct, err := hpke.Seal(pub, hpke.HKDFSHA256(), hpke.ChaCha20Poly1305(), bind("hisar/v1/vault-key", vaultID, recipientID), vaultKey)
	if err != nil {
		return nil, err
	}
	return append([]byte{version1}, ct...), nil
}

func (ks *Keyset) UnwrapVaultKey(vaultID, recipientID string, wrapped []byte) ([]byte, error) {
	if len(wrapped) < 1 {
		return nil, ErrShortCipher
	}
	if wrapped[0] != version1 {
		return nil, ErrBadVersion
	}
	key, err := hpke.Open(ks.kemPriv, hpke.HKDFSHA256(), hpke.ChaCha20Poly1305(), bind("hisar/v1/vault-key", vaultID, recipientID), wrapped[1:])
	if err != nil || len(key) != KeySize {
		return nil, ErrDecrypt
	}
	return key, nil
}
