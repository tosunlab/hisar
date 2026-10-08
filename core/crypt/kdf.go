package crypt

import (
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"
	"golang.org/x/text/unicode/norm"
)

const SaltSize = 16

type Argon2Params struct {
	Time      uint32 `json:"t"`
	MemoryKiB uint32 `json:"m"`
	Threads   uint8  `json:"p"`
}

var (
	DefaultArgon2Params = Argon2Params{Time: 24, MemoryKiB: 64 * 1024, Threads: 4}

	// Enforced client-side: the server supplies the params and must not be able to weaken them.
	MinArgon2Params = Argon2Params{Time: 2, MemoryKiB: 32 * 1024, Threads: 1}
	MaxArgon2Params = Argon2Params{Time: 64, MemoryKiB: 1024 * 1024, Threads: 16}

	ErrBadSalt = errors.New("crypt: salt must be 16 bytes")
)

func (params Argon2Params) Validate() error {
	minimum, maximum := MinArgon2Params, MaxArgon2Params
	if params.Time < minimum.Time || params.Time > maximum.Time ||
		params.MemoryKiB < minimum.MemoryKiB || params.MemoryKiB > maximum.MemoryKiB ||
		params.Threads < minimum.Threads || params.Threads > maximum.Threads {
		return fmt.Errorf("crypt: argon2 params %+v outside allowed range", params)
	}
	return nil
}

func GenerateSalt() []byte {
	salt := make([]byte, SaltSize)
	rand.Read(salt)
	return salt
}

type DerivedKeys struct {
	AccountUnlockKey []byte
	AuthKey          []byte
}

func (derivedKeys *DerivedKeys) Wipe() {
	clear(derivedKeys.AccountUnlockKey)
	clear(derivedKeys.AuthKey)
}

func DeriveAllKey(password, salt []byte, params Argon2Params, secretKey SecretKey) (*DerivedKeys, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}
	if len(salt) != SaltSize {
		return nil, ErrBadSalt
	}
	if !IsValidAccountID(secretKey.AccountID) {
		return nil, ErrBadAccountID
	}

	// Append into a fresh slice: norm.Bytes may return its input, and normalizedPassword is wiped below.
	normalizedPassword := norm.NFKD.Append(nil, password...)
	passwordKey := argon2.IDKey(normalizedPassword, salt, params.Time, params.MemoryKiB, params.Threads, 32)
	clear(normalizedPassword)
	secretKeyMaterial, err := hkdf.Key(sha256.New, secretKey.key[:], []byte(secretKey.AccountID), "hisar/v1/secret-key", 32)
	if err != nil {
		return nil, err
	}

	inputKeyMaterial := make([]byte, 0, len(passwordKey)+len(secretKeyMaterial))
	inputKeyMaterial = append(append(inputKeyMaterial, passwordKey...), secretKeyMaterial...)
	defer clear(inputKeyMaterial)
	clear(passwordKey)
	clear(secretKeyMaterial)

	accountUnlockKey, err := hkdf.Key(sha256.New, inputKeyMaterial, nil, "hisar/v1/auk", 32)
	if err != nil {
		return nil, err
	}
	authKey, err := hkdf.Key(sha256.New, inputKeyMaterial, nil, "hisar/v1/auth", 32)
	if err != nil {
		return nil, err
	}
	return &DerivedKeys{AccountUnlockKey: accountUnlockKey, AuthKey: authKey}, nil
}
