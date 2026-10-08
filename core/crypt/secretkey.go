package crypt

import (
	"crypto/rand"
	"encoding/base32"
	"errors"
	"strings"
)

const (
	secretKeyPrefix    = "H1"
	accountIDLength    = 6
	secretKeyByteCount = 16
	secretKeyCharCount = 26
	crockfordAlphabet  = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
)

var (
	crockford          = base32.NewEncoding(crockfordAlphabet).WithPadding(base32.NoPadding)
	ErrBadSecretKey    = errors.New("crypt: invalid secret key")
	ErrBadAccountID    = errors.New("crypt: invalid account ID")
	crockfordNormalize = strings.NewReplacer("O", "0", "I", "1", "L", "1", "-", "", " ", "")
)

type SecretKey struct {
	AccountID string
	key       [secretKeyByteCount]byte
}

func GenerateAccountID() string {
	random := make([]byte, 4)
	rand.Read(random)
	return crockford.EncodeToString(random)[:accountIDLength]
}

func IsValidAccountID(accountID string) bool {
	if len(accountID) != accountIDLength {
		return false
	}
	for _, character := range accountID {
		if !strings.ContainsRune(crockfordAlphabet, character) {
			return false
		}
	}
	return true
}

func GenerateSecretKey(accountID string) (SecretKey, error) {
	if !IsValidAccountID(accountID) {
		return SecretKey{}, ErrBadAccountID
	}
	secretKey := SecretKey{AccountID: accountID}
	rand.Read(secretKey.key[:])
	return secretKey, nil
}

// Encode returns the display form H1-AAAAAA-XXXXX-XXXXX-XXXXX-XXXXX-XXXXXX.
func (secretKey SecretKey) Encode() string {
	encodedKey := crockford.EncodeToString(secretKey.key[:])
	return strings.Join([]string{
		secretKeyPrefix, secretKey.AccountID,
		encodedKey[0:5], encodedKey[5:10], encodedKey[10:15], encodedKey[15:20], encodedKey[20:26],
	}, "-")
}

func ParseSecretKey(displayForm string) (SecretKey, error) {
	normalized := crockfordNormalize.Replace(strings.ToUpper(strings.TrimSpace(displayForm)))
	if len(normalized) != len(secretKeyPrefix)+accountIDLength+secretKeyCharCount || !strings.HasPrefix(normalized, secretKeyPrefix) {
		return SecretKey{}, ErrBadSecretKey
	}
	normalized = normalized[len(secretKeyPrefix):]
	secretKey := SecretKey{AccountID: normalized[:accountIDLength]}
	if !IsValidAccountID(secretKey.AccountID) {
		return SecretKey{}, ErrBadSecretKey
	}
	encodedKey := normalized[accountIDLength:]
	decodedKey, err := crockford.DecodeString(encodedKey)
	if err != nil || len(decodedKey) != secretKeyByteCount || crockford.EncodeToString(decodedKey) != encodedKey {
		return SecretKey{}, ErrBadSecretKey
	}
	copy(secretKey.key[:], decodedKey)
	return secretKey, nil
}
