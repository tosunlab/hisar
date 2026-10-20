package crypt

import (
	"crypto/ed25519"
	"crypto/sha256"
	"strconv"
)

// ShareIntent is an immutable authorization. An empty recipient authorizes invitation only, never key delivery.
type ShareIntent struct {
	Version          int    `json:"version"`
	Origin           string `json:"origin"`
	ID               string `json:"id"`
	VaultID          string `json:"vaultId"`
	InvitedBy        string `json:"invitedBy"`
	Email            string `json:"email"`
	Role             string `json:"role"`
	KeyGeneration    int64  `json:"keyGeneration"`
	AccessRevision   int64  `json:"accessRevision"`
	IssuedAt         int64  `json:"issuedAt"`
	ExpiresAt        int64  `json:"expiresAt"`
	RecipientID      string `json:"recipientId"`
	RecipientKeyHash []byte `json:"recipientKeyHash"`
	// ParentHash binds an explicit recipient confirmation to the exact initial invitation.
	ParentHash []byte `json:"parentHash,omitempty"`
}

func encodeShareIntent(intent ShareIntent) []byte {
	return encodeContext("hisar/v1/share-intent", strconv.Itoa(intent.Version), intent.Origin, intent.ID, intent.VaultID,
		intent.InvitedBy, intent.Email, intent.Role, strconv.FormatInt(intent.KeyGeneration, 10), strconv.FormatInt(intent.AccessRevision, 10),
		strconv.FormatInt(intent.IssuedAt, 10), strconv.FormatInt(intent.ExpiresAt, 10), intent.RecipientID, string(intent.RecipientKeyHash), string(intent.ParentHash))
}

func (keyset *Keyset) SignShareIntent(intent ShareIntent) []byte {
	return ed25519.Sign(keyset.signingPrivateKey, encodeShareIntent(intent))
}

func VerifyShareIntent(publicKey []byte, intent ShareIntent, signature []byte) error {
	if len(publicKey) != ed25519.PublicKeySize || !ed25519.Verify(publicKey, encodeShareIntent(intent), signature) {
		return ErrBadSignature
	}
	return nil
}

func HashShareIntent(intent ShareIntent) []byte {
	hash := sha256.Sum256(encodeShareIntent(intent))
	return hash[:]
}

func ShareRecipientKeyHash(signingKey, kemKey []byte) []byte {
	hash := sha256.Sum256(encodeContext("hisar/v1/share-recipient", string(signingKey), string(kemKey)))
	return hash[:]
}
