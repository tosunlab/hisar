package crypt

import (
	"encoding/binary"
	"errors"

	"golang.org/x/crypto/argon2"
	"golang.org/x/text/unicode/norm"
)

// ProtectPendingFactor protects an interrupted operation independently of its new Secret Key.
// Possession of this envelope permits offline password guesses, unlike normal two-factor account encryption.
func ProtectPendingFactor(password, plaintext []byte, operationID, origin, accountID string) ([]byte, error) {
	salt := GenerateSalt()
	key := pendingPasswordKey(password, salt)
	defer clear(key)
	sealed, err := Seal(key, plaintext, encodeContext("hisar/v1/pending-factor", operationID, origin, accountID))
	if err != nil {
		return nil, err
	}
	encoded := binary.BigEndian.AppendUint16(nil, 1)
	encoded = append(encoded, salt...)
	return append(encoded, sealed...), nil
}

func OpenPendingFactor(password, envelope []byte, operationID, origin, accountID string) ([]byte, error) {
	if len(envelope) < 2+SaltSize || binary.BigEndian.Uint16(envelope[:2]) != 1 {
		return nil, errors.New("crypt: unsupported pending factor envelope")
	}
	key := pendingPasswordKey(password, envelope[2:2+SaltSize])
	defer clear(key)
	return Open(key, envelope[2+SaltSize:], encodeContext("hisar/v1/pending-factor", operationID, origin, accountID))
}

func pendingPasswordKey(password, salt []byte) []byte {
	normalized := norm.NFKD.Append(nil, password...)
	defer clear(normalized)
	params := DefaultArgon2Params()
	return argon2.IDKey(normalized, encodeContext("hisar/v1/pending-factor-password", string(salt)), params.Time, params.MemoryKiB, params.Threads, KeySize)
}

func SealPendingFactorWithKey(key, envelope []byte, operationID, origin, accountID string) ([]byte, error) {
	return Seal(key, envelope, encodeContext("hisar/v1/pending-factor-keychain", operationID, origin, accountID))
}

func OpenPendingFactorWithKey(key, envelope []byte, operationID, origin, accountID string) ([]byte, error) {
	return Open(key, envelope, encodeContext("hisar/v1/pending-factor-keychain", operationID, origin, accountID))
}
