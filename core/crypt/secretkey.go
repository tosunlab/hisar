package crypt

import (
	"crypto/rand"
	"encoding/base32"
	"errors"
	"strings"
)

const (
	secretKeyPrefix  = "H1"
	accountIDLen     = 6
	secretKeyBytes   = 16
	secretKeyCharLen = 26
)

var (
	crockford          = base32.NewEncoding("0123456789ABCDEFGHJKMNPQRSTVWXYZ").WithPadding(base32.NoPadding)
	ErrBadSecretKey    = errors.New("crypt: invalid secret key")
	ErrBadAccountID    = errors.New("crypt: invalid account ID")
	crockfordNormalize = strings.NewReplacer("O", "0", "I", "1", "L", "1", "-", "", " ", "")
)

type SecretKey struct {
	AccountID string
	key       [secretKeyBytes]byte
}

func NewAccountID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return crockford.EncodeToString(b)[:accountIDLen]
}

func ValidAccountID(id string) bool {
	if len(id) != accountIDLen {
		return false
	}
	for _, c := range id {
		if !strings.ContainsRune("0123456789ABCDEFGHJKMNPQRSTVWXYZ", c) {
			return false
		}
	}
	return true
}

func NewSecretKey(accountID string) (SecretKey, error) {
	if !ValidAccountID(accountID) {
		return SecretKey{}, ErrBadAccountID
	}
	sk := SecretKey{AccountID: accountID}
	rand.Read(sk.key[:])
	return sk, nil
}

// Encode returns the display form H1-AAAAAA-XXXXX-XXXXX-XXXXX-XXXXX-XXXXXX.
func (sk SecretKey) Encode() string {
	k := crockford.EncodeToString(sk.key[:])
	return strings.Join([]string{secretKeyPrefix, sk.AccountID, k[0:5], k[5:10], k[10:15], k[15:20], k[20:26]}, "-")
}

func ParseSecretKey(s string) (SecretKey, error) {
	s = crockfordNormalize.Replace(strings.ToUpper(strings.TrimSpace(s)))
	if len(s) != len(secretKeyPrefix)+accountIDLen+secretKeyCharLen || !strings.HasPrefix(s, secretKeyPrefix) {
		return SecretKey{}, ErrBadSecretKey
	}
	s = s[len(secretKeyPrefix):]
	sk := SecretKey{AccountID: s[:accountIDLen]}
	if !ValidAccountID(sk.AccountID) {
		return SecretKey{}, ErrBadSecretKey
	}
	enc := s[accountIDLen:]
	raw, err := crockford.DecodeString(enc)
	if err != nil || len(raw) != secretKeyBytes || crockford.EncodeToString(raw) != enc {
		return SecretKey{}, ErrBadSecretKey
	}
	copy(sk.key[:], raw)
	return sk, nil
}
