package crypt

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"
)

func decodeBase64URL(t *testing.T, text string) []byte {
	t.Helper()
	decoded, err := base64.RawURLEncoding.DecodeString(text)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func TestWebPushVector(t *testing.T) {
	senderKey, err := ecdh.P256().NewPrivateKey(decodeBase64URL(t, vectorWebPushSenderKey))
	if err != nil {
		t.Fatal(err)
	}
	userAgentKey, err := ecdh.P256().NewPrivateKey(decodeBase64URL(t, vectorWebPushUserAgentKey))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(userAgentKey.PublicKey().Bytes(), decodeBase64URL(t, vectorWebPushUserAgentPub)) {
		t.Fatal("the vector's user agent key pair does not match")
	}
	authSecret := decodeBase64URL(t, vectorWebPushAuthSecret)
	message, err := encryptWebPush(decodeBase64URL(t, vectorWebPushUserAgentPub), authSecret, []byte(vectorWebPushPlaintext), senderKey, decodeBase64URL(t, vectorWebPushSalt))
	if err != nil {
		t.Fatal(err)
	}
	if got := base64.RawURLEncoding.EncodeToString(message); got != vectorWebPushMessage {
		t.Fatalf("message = %s", got)
	}
	plaintext, err := DecryptWebPush(userAgentKey, authSecret, decodeBase64URL(t, vectorWebPushMessage))
	if err != nil || string(plaintext) != vectorWebPushPlaintext {
		t.Fatalf("DecryptWebPush = %q, %v", plaintext, err)
	}
}

func TestWebPushOnlyTheSubscriptionReadsIt(t *testing.T) {
	userAgentKey, _ := ecdh.P256().GenerateKey(rand.Reader)
	otherKey, _ := ecdh.P256().GenerateKey(rand.Reader)
	authSecret := make([]byte, WebPushAuthSecretSize)
	rand.Read(authSecret)
	message, err := EncryptWebPush(userAgentKey.PublicKey().Bytes(), authSecret, []byte(`{"kind":"approval"}`))
	if err != nil {
		t.Fatal(err)
	}
	if plaintext, err := DecryptWebPush(userAgentKey, authSecret, message); err != nil || string(plaintext) != `{"kind":"approval"}` {
		t.Fatalf("DecryptWebPush = %q, %v", plaintext, err)
	}
	if _, err := DecryptWebPush(otherKey, authSecret, message); !errors.Is(err, ErrDecrypt) {
		t.Errorf("another key read it: %v", err)
	}
	tampered := bytes.Clone(message)
	tampered[len(tampered)-1] ^= 1
	if _, err := DecryptWebPush(userAgentKey, authSecret, tampered); !errors.Is(err, ErrDecrypt) {
		t.Errorf("a tampered message opened: %v", err)
	}
	for _, keys := range [][2][]byte{{make([]byte, 65), authSecret}, {userAgentKey.PublicKey().Bytes(), authSecret[:8]}, {[]byte("short"), authSecret}} {
		if _, err := EncryptWebPush(keys[0], keys[1], []byte("x")); !errors.Is(err, ErrBadWebPushSubscription) {
			t.Errorf("EncryptWebPush with bad keys = %v", err)
		}
	}
}
