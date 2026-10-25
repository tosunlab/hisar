// Package pushrelay is the push relay that hisar-server push-relay runs: phones trade their APNs or FCM token for an
// opaque handle, and Hisar servers send a push kind to a handle. Only the relay holds Apple's and Google's keys, so a
// self-hosted server needs neither, never sees a push token, and the relay never learns which account a handle is.
package pushrelay

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/tryhisar/hisar/core/apitypes"
)

const (
	handleVersion = 1
	// maximumTokenLength bounds a push service token; APNs tokens are 64 hex characters, FCM tokens a few hundred.
	maximumTokenLength = 4096
	handleNonceSize    = 12
)

var handleAAD = []byte("hisar push handle v1")

var ErrInvalidHandle = errors.New("pushrelay: invalid handle")

var allPlatformByte = map[apitypes.PushPlatform]byte{
	apitypes.PushPlatformAPNs:        1,
	apitypes.PushPlatformAPNsSandbox: 2,
}

// Sealer turns a platform and token into a handle and back, with AES-256-GCM under the relay's key. A handle is
// base64url(version || nonce || ciphertext) and its plaintext is one platform byte followed by the token.
type Sealer struct {
	aead cipher.AEAD
}

func NewSealer(key []byte) (*Sealer, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("pushrelay: the sealing key must be 32 bytes, not %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Sealer{aead: aead}, nil
}

func (sealer *Sealer) Seal(platform apitypes.PushPlatform, token string) (string, error) {
	platformByte, ok := allPlatformByte[platform]
	if !ok {
		return "", fmt.Errorf("pushrelay: unknown platform %q", platform)
	}
	if token == "" || len(token) > maximumTokenLength {
		return "", errors.New("pushrelay: invalid token")
	}
	nonce := make([]byte, handleNonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	plaintext := append([]byte{platformByte}, token...)
	sealed := append([]byte{handleVersion}, nonce...)
	sealed = sealer.aead.Seal(sealed, nonce, plaintext, handleAAD)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (sealer *Sealer) Open(handle string) (apitypes.PushPlatform, string, error) {
	sealed, err := base64.RawURLEncoding.DecodeString(handle)
	if err != nil || len(sealed) < 1+handleNonceSize+sealer.aead.Overhead()+2 || sealed[0] != handleVersion {
		return "", "", ErrInvalidHandle
	}
	nonce := sealed[1 : 1+handleNonceSize]
	plaintext, err := sealer.aead.Open(nil, nonce, sealed[1+handleNonceSize:], handleAAD)
	if err != nil {
		return "", "", ErrInvalidHandle
	}
	for platform, platformByte := range allPlatformByte {
		if plaintext[0] == platformByte {
			return platform, string(plaintext[1:]), nil
		}
	}
	return "", "", ErrInvalidHandle
}
