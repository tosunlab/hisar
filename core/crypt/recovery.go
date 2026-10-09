package crypt

import "crypto/ed25519"

// RecoveryRecipientID is the recipient ID vault keys are wrapped to for the recovery keyset.
const RecoveryRecipientID = "recovery"

// SignRecoveryKeyset returns the recovery keyset's KEM public key signed by this (admin) keyset.
// It verifies with PublicKeys.Verify(RecoveryRecipientID).
func (keyset *Keyset) SignRecoveryKeyset(recoveryKeyset *Keyset) PublicKeys {
	kemPublicKey := recoveryKeyset.kemPrivateKey.PublicKey().Bytes()
	return PublicKeys{
		KEM:    kemPublicKey,
		Sign:   keyset.signingPrivateKey.Public().(ed25519.PublicKey),
		KEMSig: ed25519.Sign(keyset.signingPrivateKey, encodeContext("hisar/v1/kem-key", RecoveryRecipientID, string(kemPublicKey))),
	}
}

func (keyset *Keyset) GetKEMPublicKey() []byte {
	return keyset.kemPrivateKey.PublicKey().Bytes()
}

// EncryptRecoveryKeyset seals the recovery keyset with the creating admin's user key.
func (keyset *Keyset) EncryptRecoveryKeyset(adminUserKey []byte, adminAccountID string) ([]byte, error) {
	plaintext := keyset.encode()
	defer clear(plaintext)
	return Seal(adminUserKey, plaintext, encodeContext("hisar/v1/recovery-keyset", adminAccountID))
}

func DecryptRecoveryKeyset(adminUserKey []byte, adminAccountID string, ciphertext []byte) (*Keyset, error) {
	plaintext, err := Open(adminUserKey, ciphertext, encodeContext("hisar/v1/recovery-keyset", adminAccountID))
	if err != nil {
		return nil, err
	}
	defer clear(plaintext)
	return decodeKeyset(plaintext)
}
