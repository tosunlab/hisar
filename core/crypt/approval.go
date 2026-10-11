package crypt

import (
	"crypto/ed25519"
	"crypto/hpke"
	"crypto/sha256"
)

const (
	approvalRequestLabel        = "hisar/v1/approval-request"
	approvalAnswerLabel         = "hisar/v1/approval-answer"
	approvalAnswerEnvelopeLabel = "hisar/v1/approval-answer-envelope"
)

// ApprovalAnswerFields is what the account signing key signs for a phone approval answer.
type ApprovalAnswerFields struct {
	AccountID           string
	RequestID           string
	RequestHash         []byte
	Decision            string
	AnsweringDeviceID   string
	AnsweringDeviceName string
	AnsweredAt          string
	// RememberChoice is empty for an answer without the phone's remember switch, else ApprovalRememberChoice or ApprovalOnceChoice.
	RememberChoice string
}

const (
	ApprovalRememberChoice = "remember"
	ApprovalOnceChoice     = "once"
)

func sealToAccount(accountKEMPublicKey []byte, info, plaintext []byte) ([]byte, error) {
	recipientPublicKey, err := hybridKeyEncapsulation.NewPublicKey(accountKEMPublicKey)
	if err != nil {
		return nil, err
	}
	ciphertext, err := hpke.Seal(recipientPublicKey, hpke.HKDFSHA256(), hpke.ChaCha20Poly1305(), info, plaintext)
	if err != nil {
		return nil, err
	}
	return append([]byte{version1}, ciphertext...), nil
}

func (keyset *Keyset) openFromAccount(info, sealed []byte) ([]byte, error) {
	if len(sealed) < 1 {
		return nil, ErrShortCipher
	}
	if sealed[0] != version1 {
		return nil, ErrBadVersion
	}
	plaintext, err := hpke.Open(keyset.kemPrivateKey, hpke.HKDFSHA256(), hpke.ChaCha20Poly1305(), info, sealed[1:])
	if err != nil {
		return nil, ErrDecrypt
	}
	return plaintext, nil
}

// SealApprovalRequest encrypts a phone approval request to the account's own KEM public key.
func SealApprovalRequest(accountKEMPublicKey []byte, accountID, requestID string, plaintext []byte) ([]byte, error) {
	return sealToAccount(accountKEMPublicKey, encodeContext(approvalRequestLabel, accountID, requestID), plaintext)
}

func (keyset *Keyset) OpenApprovalRequest(accountID, requestID string, sealedRequest []byte) ([]byte, error) {
	return keyset.openFromAccount(encodeContext(approvalRequestLabel, accountID, requestID), sealedRequest)
}

// SealApprovalAnswer encrypts a signed answer, so the server sees neither the decision nor the answering device.
func SealApprovalAnswer(accountKEMPublicKey []byte, accountID, requestID string, plaintext []byte) ([]byte, error) {
	return sealToAccount(accountKEMPublicKey, encodeContext(approvalAnswerEnvelopeLabel, accountID, requestID), plaintext)
}

func (keyset *Keyset) OpenApprovalAnswer(accountID, requestID string, sealedAnswer []byte) ([]byte, error) {
	return keyset.openFromAccount(encodeContext(approvalAnswerEnvelopeLabel, accountID, requestID), sealedAnswer)
}

func HashApprovalRequest(sealedRequest []byte) []byte {
	hash := sha256.Sum256(sealedRequest)
	return hash[:]
}

// encodeApprovalAnswer appends the remember choice only when there is one, so answers without it verify as before.
func encodeApprovalAnswer(fields ApprovalAnswerFields) []byte {
	allField := []string{fields.AccountID, fields.RequestID, string(fields.RequestHash), fields.Decision,
		fields.AnsweringDeviceID, fields.AnsweringDeviceName, fields.AnsweredAt}
	if fields.RememberChoice != "" {
		allField = append(allField, fields.RememberChoice)
	}
	return encodeContext(approvalAnswerLabel, allField...)
}

func (keyset *Keyset) SignApprovalAnswer(fields ApprovalAnswerFields) []byte {
	return ed25519.Sign(keyset.signingPrivateKey, encodeApprovalAnswer(fields))
}

func VerifyApprovalAnswer(signingPublicKey []byte, fields ApprovalAnswerFields, signature []byte) error {
	if len(signingPublicKey) != ed25519.PublicKeySize || !ed25519.Verify(signingPublicKey, encodeApprovalAnswer(fields), signature) {
		return ErrBadSignature
	}
	return nil
}

func (keyset *Keyset) GetSigningPublicKey() []byte {
	return append([]byte(nil), keyset.signingPrivateKey.Public().(ed25519.PublicKey)...)
}
