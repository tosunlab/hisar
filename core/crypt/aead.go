package crypt

import (
	"crypto/rand"
	"encoding/binary"
	"errors"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	KeySize  = chacha20poly1305.KeySize
	version1 = byte(1)
)

var (
	ErrDecrypt     = errors.New("crypt: decryption failed")
	ErrBadKeySize  = errors.New("crypt: key must be 32 bytes")
	ErrBadVersion  = errors.New("crypt: unsupported ciphertext version")
	ErrShortCipher = errors.New("crypt: ciphertext too short")
)

func NewKey() []byte {
	k := make([]byte, KeySize)
	rand.Read(k)
	return k
}

// Seal output: version(1) || nonce(24) || ciphertext+tag.
func Seal(key, plaintext, aad []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, ErrBadKeySize
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	hdr := 1 + aead.NonceSize()
	out := make([]byte, hdr, hdr+len(plaintext)+aead.Overhead())
	out[0] = version1
	rand.Read(out[1:hdr])
	return aead.Seal(out, out[1:hdr], plaintext, aad), nil
}

func Open(key, ciphertext, aad []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, ErrBadKeySize
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	hdr := 1 + aead.NonceSize()
	if len(ciphertext) < hdr+aead.Overhead() {
		return nil, ErrShortCipher
	}
	if ciphertext[0] != version1 {
		return nil, ErrBadVersion
	}
	pt, err := aead.Open(nil, ciphertext[1:hdr], ciphertext[hdr:], aad)
	if err != nil {
		return nil, ErrDecrypt
	}
	return pt, nil
}

// Length-prefixed so that field boundaries are unambiguous.
func bind(label string, fields ...string) []byte {
	b := []byte(label)
	for _, f := range fields {
		b = binary.BigEndian.AppendUint32(b, uint32(len(f)))
		b = append(b, f...)
	}
	return b
}
