package crypt

import (
	"encoding/hex"
	"errors"
	"testing"
)

func createVectorPushKey() (publicKey, authSecret []byte) {
	publicKey = make([]byte, 65)
	for index := range publicKey {
		publicKey[index] = byte(index)
	}
	return publicKey, []byte("0123456789abcdef")
}

func TestPushKeySignatureVector(t *testing.T) {
	keyset := createVectorKeyset(t)
	publicKey, authSecret := createVectorPushKey()
	if got := hex.EncodeToString(encodePushKey("ABC123", "phone-1", publicKey, authSecret)); got != vectorPushKeyMessage {
		t.Errorf("signed message = %s", got)
	}
	signature := keyset.SignPushKey("ABC123", "phone-1", publicKey, authSecret)
	if got := hex.EncodeToString(signature); got != vectorPushKeySignature {
		t.Errorf("signature = %s", got)
	}
	if err := VerifyPushKey(keyset.GetSigningPublicKey(), "ABC123", "phone-1", publicKey, authSecret, signature); err != nil {
		t.Fatal(err)
	}
	// A key the server swapped in, or one moved to another device, does not verify.
	otherKey := append([]byte(nil), publicKey...)
	otherKey[1] ^= 1
	for name, err := range map[string]error{
		"another key":    VerifyPushKey(keyset.GetSigningPublicKey(), "ABC123", "phone-1", otherKey, authSecret, signature),
		"another device": VerifyPushKey(keyset.GetSigningPublicKey(), "ABC123", "phone-2", publicKey, authSecret, signature),
	} {
		if !errors.Is(err, ErrBadSignature) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestApprovalPreviewSignatureVector(t *testing.T) {
	keyset := createVectorKeyset(t)
	preview := []byte(`{"requestId":"req-1","kind":"ssh"}`)
	if got := hex.EncodeToString(encodeApprovalPreview("ABC123", preview)); got != vectorApprovalPreviewMessage {
		t.Errorf("signed message = %s", got)
	}
	signature := keyset.SignApprovalPreview("ABC123", preview)
	if got := hex.EncodeToString(signature); got != vectorApprovalPreviewSignature {
		t.Errorf("signature = %s", got)
	}
	if err := VerifyApprovalPreview(keyset.GetSigningPublicKey(), "ABC123", preview, signature); err != nil {
		t.Fatal(err)
	}
	if err := VerifyApprovalPreview(keyset.GetSigningPublicKey(), "OTHER1", preview, signature); !errors.Is(err, ErrBadSignature) {
		t.Errorf("another account: %v", err)
	}
	if err := VerifyApprovalPreview(keyset.GetSigningPublicKey(), "ABC123", []byte(`{"requestId":"req-1","kind":"cli"}`), signature); !errors.Is(err, ErrBadSignature) {
		t.Errorf("a changed preview: %v", err)
	}
}
