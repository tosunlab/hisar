package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
)

// Web Push message encryption (RFC 8291) in the aes128gcm content coding (RFC 8188), as one record: what a Hisar server
// sends to an Android phone's UnifiedPush endpoint, readable only by the phone that made the subscription.
const (
	WebPushAuthSecretSize = 16
	webPushSaltSize       = 16
	webPushRecordSize     = 4096
	// webPushPublicKeySize is an uncompressed P-256 point.
	webPushPublicKeySize = 65
	webPushHeaderSize    = webPushSaltSize + 4 + 1 + webPushPublicKeySize
	// MaximumWebPushPlaintext fits one record: the record size less the padding delimiter and the GCM tag.
	MaximumWebPushPlaintext = webPushRecordSize - 1 - 16
)

var ErrBadWebPushSubscription = errors.New("crypt: invalid Web Push subscription keys")

// EncryptWebPush encrypts plaintext to a subscription's public key (p256dh) and authentication secret.
func EncryptWebPush(userAgentPublicKey, authSecret, plaintext []byte) ([]byte, error) {
	senderKey, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	salt := make([]byte, webPushSaltSize)
	rand.Read(salt)
	return encryptWebPush(userAgentPublicKey, authSecret, plaintext, senderKey, salt)
}

func encryptWebPush(userAgentPublicKey, authSecret, plaintext []byte, senderKey *ecdh.PrivateKey, salt []byte) ([]byte, error) {
	if len(plaintext) > MaximumWebPushPlaintext {
		return nil, errors.New("crypt: Web Push message too long")
	}
	userAgentKey, err := ParseWebPushPublicKey(userAgentPublicKey)
	if err != nil {
		return nil, err
	}
	if len(authSecret) != WebPushAuthSecretSize {
		return nil, ErrBadWebPushSubscription
	}
	senderPublicKey := senderKey.PublicKey().Bytes()
	aead, nonce, err := deriveWebPushCipher(senderKey, userAgentKey, userAgentPublicKey, senderPublicKey, authSecret, salt)
	if err != nil {
		return nil, err
	}
	header := make([]byte, 0, webPushHeaderSize)
	header = append(header, salt...)
	header = binary.BigEndian.AppendUint32(header, webPushRecordSize)
	header = append(header, byte(len(senderPublicKey)))
	header = append(header, senderPublicKey...)
	// 0x02 marks the last record; no further padding.
	padded := append(append(make([]byte, 0, len(plaintext)+1), plaintext...), 0x02)
	return aead.Seal(header, nonce, padded, nil), nil
}

// DecryptWebPush opens a message EncryptWebPush made for the subscription of userAgentKey and authSecret; it is what the
// phone does, here for tests.
func DecryptWebPush(userAgentKey *ecdh.PrivateKey, authSecret, message []byte) ([]byte, error) {
	if len(message) < webPushHeaderSize+16 || message[webPushSaltSize+4] != webPushPublicKeySize || len(authSecret) != WebPushAuthSecretSize {
		return nil, ErrDecrypt
	}
	salt := message[:webPushSaltSize]
	senderPublicKey := message[webPushSaltSize+5 : webPushHeaderSize]
	senderKey, err := ParseWebPushPublicKey(senderPublicKey)
	if err != nil {
		return nil, ErrDecrypt
	}
	aead, nonce, err := deriveWebPushCipher(userAgentKey, senderKey, userAgentKey.PublicKey().Bytes(), senderPublicKey, authSecret, salt)
	if err != nil {
		return nil, ErrDecrypt
	}
	padded, err := aead.Open(nil, nonce, message[webPushHeaderSize:], nil)
	if err != nil || len(padded) == 0 {
		return nil, ErrDecrypt
	}
	for index := len(padded) - 1; index >= 0; index-- {
		switch padded[index] {
		case 0:
			continue
		case 0x02:
			return padded[:index], nil
		}
		break
	}
	return nil, ErrDecrypt
}

// ParseWebPushPublicKey reads a subscription's p256dh: an uncompressed P-256 point.
func ParseWebPushPublicKey(publicKey []byte) (*ecdh.PublicKey, error) {
	if len(publicKey) != webPushPublicKeySize {
		return nil, ErrBadWebPushSubscription
	}
	key, err := ecdh.P256().NewPublicKey(publicKey)
	if err != nil {
		return nil, ErrBadWebPushSubscription
	}
	return key, nil
}

// deriveWebPushCipher is RFC 8291 §3.4 and RFC 8188 §2.2: the shared secret mixed with the authentication secret, then
// the content encryption key and nonce from the salt.
func deriveWebPushCipher(privateKey *ecdh.PrivateKey, peerKey *ecdh.PublicKey, userAgentPublicKey, senderPublicKey, authSecret, salt []byte) (cipher.AEAD, []byte, error) {
	sharedSecret, err := privateKey.ECDH(peerKey)
	if err != nil {
		return nil, nil, err
	}
	keyInfo := append(append([]byte("WebPush: info\x00"), userAgentPublicKey...), senderPublicKey...)
	inputKey, err := hkdf.Key(sha256.New, sharedSecret, authSecret, string(keyInfo), 32)
	if err != nil {
		return nil, nil, err
	}
	contentKey, err := hkdf.Key(sha256.New, inputKey, salt, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, nil, err
	}
	nonce, err := hkdf.Key(sha256.New, inputKey, salt, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, nil, err
	}
	block, err := aes.NewCipher(contentKey)
	if err != nil {
		return nil, nil, err
	}
	aead, err := cipher.NewGCM(block)
	return aead, nonce, err
}
