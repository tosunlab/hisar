package crypt

import "crypto/ed25519"

// A Mac encrypts an approval's preview to each of the account's phones, and signs it; each phone signs the key it gets
// pushes with. Both signatures are the account's, so the server can neither hand the Mac a key of its own nor make a
// phone show a preview no device of the account wrote.
const (
	pushKeyLabel         = "hisar/v1/push-key"
	approvalPreviewLabel = "hisar/v1/approval-preview"
)

func encodePushKey(accountID, deviceID string, publicKey, authSecret []byte) []byte {
	return encodeContext(pushKeyLabel, accountID, deviceID, string(publicKey), string(authSecret))
}

// SignPushKey signs the public key and authentication secret (RFC 8291) a phone of the account gets pushes with.
func (keyset *Keyset) SignPushKey(accountID, deviceID string, publicKey, authSecret []byte) []byte {
	return ed25519.Sign(keyset.signingPrivateKey, encodePushKey(accountID, deviceID, publicKey, authSecret))
}

// VerifyPushKey checks a phone's push key against the account's signing key before anything is encrypted to it.
func VerifyPushKey(signingPublicKey []byte, accountID, deviceID string, publicKey, authSecret, signature []byte) error {
	if len(signingPublicKey) != ed25519.PublicKeySize || !ed25519.Verify(signingPublicKey, encodePushKey(accountID, deviceID, publicKey, authSecret), signature) {
		return ErrBadSignature
	}
	return nil
}

func encodeApprovalPreview(accountID string, preview []byte) []byte {
	return encodeContext(approvalPreviewLabel, accountID, string(preview))
}

// SignApprovalPreview signs the preview of an approval request, the bytes a phone's notification shows.
func (keyset *Keyset) SignApprovalPreview(accountID string, preview []byte) []byte {
	return ed25519.Sign(keyset.signingPrivateKey, encodeApprovalPreview(accountID, preview))
}

// VerifyApprovalPreview is what a phone checks before its notification shows a preview.
func VerifyApprovalPreview(signingPublicKey []byte, accountID string, preview, signature []byte) error {
	if len(signingPublicKey) != ed25519.PublicKeySize || !ed25519.Verify(signingPublicKey, encodeApprovalPreview(accountID, preview), signature) {
		return ErrBadSignature
	}
	return nil
}
