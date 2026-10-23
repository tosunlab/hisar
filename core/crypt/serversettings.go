package crypt

import (
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/binary"
)

func ServerSecretsFingerprint(opaqueKeyMaterial, serverHMACKey []byte) []byte {
	hash := sha256.New()
	hash.Write([]byte("hisar/v1/server-secrets-fingerprint"))
	var size [8]byte
	for _, field := range [][]byte{opaqueKeyMaterial, serverHMACKey} {
		binary.BigEndian.PutUint64(size[:], uint64(len(field)))
		hash.Write(size[:])
		hash.Write(field)
	}
	return hash.Sum(nil)
}

// DeriveServerSettingsKey separates operational credentials from authentication HMACs.
func DeriveServerSettingsKey(serverHMACKey []byte) ([]byte, error) {
	if len(serverHMACKey) != KeySize {
		return nil, ErrBadKeySize
	}
	return hkdf.Key(sha256.New, serverHMACKey, nil, "hisar/v1/server-settings-key", KeySize)
}

func SealSMTPPassword(key []byte, password string) ([]byte, error) {
	return Seal(key, []byte(password), []byte("hisar/v1/server-settings/smtp.password"))
}

func OpenSMTPPassword(key, ciphertext []byte) (string, error) {
	plaintext, err := Open(key, ciphertext, []byte("hisar/v1/server-settings/smtp.password"))
	if err != nil {
		return "", err
	}
	defer clear(plaintext)
	return string(plaintext), nil
}
