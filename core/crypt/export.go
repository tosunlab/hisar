package crypt

import (
	"crypto/hkdf"
	"crypto/sha256"

	"golang.org/x/crypto/argon2"
	"golang.org/x/text/unicode/norm"
)

const exportKeyInfo = "hisar/v1/export-key"

// DeriveExportKey stretches an export passphrase; the params come from the file, so they are checked like the server's.
func DeriveExportKey(passphrase, salt []byte, params Argon2Params) ([]byte, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}
	if len(salt) != SaltSize {
		return nil, ErrBadSalt
	}
	normalizedPassphrase := norm.NFKD.Append(nil, passphrase...)
	stretchedKey := argon2.IDKey(normalizedPassphrase, salt, params.Time, params.MemoryKiB, params.Threads, 32)
	clear(normalizedPassphrase)
	defer clear(stretchedKey)
	return hkdf.Key(sha256.New, stretchedKey, nil, exportKeyInfo, KeySize)
}

func SealExport(exportKey, plaintext []byte) ([]byte, error) {
	return Seal(exportKey, plaintext, encodeContext("hisar/v1/export"))
}

func OpenExport(exportKey, ciphertext []byte) ([]byte, error) {
	return Open(exportKey, ciphertext, encodeContext("hisar/v1/export"))
}
