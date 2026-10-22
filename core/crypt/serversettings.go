package crypt

import (
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
