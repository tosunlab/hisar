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

func (p Argon2Params) Validate() error {
	lo, hi := MinArgon2Params, MaxArgon2Params
	if p.Time < lo.Time || p.Time > hi.Time ||
		p.MemoryKiB < lo.MemoryKiB || p.MemoryKiB > hi.MemoryKiB ||
		p.Threads < lo.Threads || p.Threads > hi.Threads {
		return fmt.Errorf("crypt: argon2 params %+v outside allowed range", p)
	}
	return nil
}

func NewSalt() []byte {
	s := make([]byte, SaltSize)
	rand.Read(s)
	return s
}

type DerivedKeys struct {
	AUK     []byte
	AuthKey []byte
}

func (d *DerivedKeys) Wipe() {
	clear(d.AUK)
	clear(d.AuthKey)
}

func DeriveKeys(password, salt []byte, p Argon2Params, sk SecretKey) (*DerivedKeys, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if len(salt) != SaltSize {
		return nil, ErrBadSalt
	}
	if !ValidAccountID(sk.AccountID) {
		return nil, ErrBadAccountID
	}

	// Append into a fresh slice: norm.Bytes may return its input, and pw is wiped below.
	pw := norm.NFKD.Append(nil, password...)
	kPw := argon2.IDKey(pw, salt, p.Time, p.MemoryKiB, p.Threads, 32)
	clear(pw)
	kSk, err := hkdf.Key(sha256.New, sk.key[:], []byte(sk.AccountID), "hisar/v1/secret-key", 32)
	if err != nil {
		return nil, err
	}

	ikm := make([]byte, 0, len(kPw)+len(kSk))
	ikm = append(append(ikm, kPw...), kSk...)
	defer clear(ikm)
	clear(kPw)
	clear(kSk)

	auk, err := hkdf.Key(sha256.New, ikm, nil, "hisar/v1/auk", 32)
	if err != nil {
		return nil, err
	}
	authKey, err := hkdf.Key(sha256.New, ikm, nil, "hisar/v1/auth", 32)
	if err != nil {
		return nil, err
	}
	return &DerivedKeys{AUK: auk, AuthKey: authKey}, nil
}
