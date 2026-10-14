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

func GenerateKey() []byte {
	key := make([]byte, KeySize)
	rand.Read(key)
	return key
}

// Seal output: version(1) || nonce(24) || ciphertext+tag.
func Seal(key, plaintext, associatedData []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, ErrBadKeySize
	}
	cipher, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	headerSize := 1 + cipher.NonceSize()
	sealed := make([]byte, headerSize, headerSize+len(plaintext)+cipher.Overhead())
	sealed[0] = version1
	rand.Read(sealed[1:headerSize])
	return cipher.Seal(sealed, sealed[1:headerSize], plaintext, associatedData), nil
}

func Open(key, ciphertext, associatedData []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, ErrBadKeySize
	}
	cipher, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	headerSize := 1 + cipher.NonceSize()
	if len(ciphertext) < headerSize+cipher.Overhead() {
		return nil, ErrShortCipher
	}
	if ciphertext[0] != version1 {
		return nil, ErrBadVersion
	}
	plaintext, err := cipher.Open(nil, ciphertext[1:headerSize], ciphertext[headerSize:], associatedData)
	if err != nil {
		return nil, ErrDecrypt
	}
	return plaintext, nil
}

// encodeContext length-prefixes each field so that field boundaries are unambiguous.
func encodeContext(label string, fields ...string) []byte {
	encoded := []byte(label)
	for _, field := range fields {
		encoded = binary.BigEndian.AppendUint32(encoded, uint32(len(field)))
		encoded = append(encoded, field...)
	}
	return encoded
}
