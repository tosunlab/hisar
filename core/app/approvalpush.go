package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/approval"
	"github.com/tryhisar/hisar/core/crypt"
)

// approvalPushPreview is a request's preview message, signed by the account, with what checks the phones' keys: the
// account and its public signing key. plaintext is nil when the preview could not be made.
type approvalPushPreview struct {
	accountID        string
	signingPublicKey []byte
	plaintext        []byte
}

// createApprovalPushPreview signs the request's preview while the keyset is at hand; it needs the mutex.
func createApprovalPushPreview(keyset *crypt.Keyset, accountID string, request approval.Request) approvalPushPreview {
	preview := approvalPushPreview{accountID: accountID, signingPublicKey: keyset.GetSigningPublicKey()}
	pushApproval, err := approval.SignPushPreview(keyset, accountID, approval.NewPushPreview(request))
	if err != nil {
		return preview
	}
	preview.plaintext, _ = json.Marshal(apitypes.PushMessage{Kind: apitypes.PushKindApproval, Approval: pushApproval})
	return preview
}

// buildApprovalPushMessages encrypts the preview to each phone of the account whose push key the account signed. The
// server then sends a phone its own message: it can neither read the preview nor give the Mac a key of its own. A
// server older than previews answers 404, and the phones get the kind alone.
func buildApprovalPushMessages(ctx context.Context, client *apiclient.Client, preview approvalPushPreview) []apitypes.ApprovalPushMessage {
	if preview.plaintext == nil {
		return nil
	}
	keys, err := client.ListPushKeys(ctx)
	if err != nil {
		if !apiclient.IsStatus(err, http.StatusNotFound) {
			log.Printf("phone approval: listing the phones' push keys: %v", err)
		}
		return nil
	}
	allMessage := []apitypes.ApprovalPushMessage{}
	for _, key := range keys.AllKey {
		publicKey, publicKeyErr := decodeBase64URL(key.P256DH)
		authSecret, authErr := decodeBase64URL(key.Auth)
		signature, signatureErr := decodeBase64URL(key.Signature)
		if publicKeyErr != nil || authErr != nil || signatureErr != nil ||
			!isPushKeySigned(preview, key.DeviceID, publicKey, authSecret, signature) {
			log.Printf("phone approval: a phone's push key does not carry the account's signature; it gets no preview")
			continue
		}
		message, err := crypt.EncryptWebPush(publicKey, authSecret, preview.plaintext)
		if err != nil {
			continue
		}
		allMessage = append(allMessage, apitypes.ApprovalPushMessage{DeviceID: key.DeviceID, P256DH: key.P256DH, Message: base64.RawURLEncoding.EncodeToString(message)})
		if len(allMessage) == apitypes.MaximumApprovalPushMessages {
			break
		}
	}
	return allMessage
}

// isPushKeySigned checks the account's signature over a phone's key and device ID. iPhone 0.1.16 signs its key over an
// empty device ID; that signature still binds the key to the account, so the phone gets previews until it updates.
func isPushKeySigned(preview approvalPushPreview, deviceID string, publicKey, authSecret, signature []byte) bool {
	return crypt.VerifyPushKey(preview.signingPublicKey, preview.accountID, deviceID, publicKey, authSecret, signature) == nil ||
		crypt.VerifyPushKey(preview.signingPublicKey, preview.accountID, "", publicKey, authSecret, signature) == nil
}

// ApprovalPushVerifier is what a phone's notification checks an approval preview with while Hisar is locked: the
// signed-in account and its public signing key, base64url. Both are empty before the first unlock.
type ApprovalPushVerifier struct {
	AccountID        string `json:"accountId"`
	SigningPublicKey string `json:"signingPublicKey"`
}

// GetApprovalPushVerifier is for the iPhone app, which hands it to its notification service extension. Not a method,
// so the desktop UI cannot call it.
func GetApprovalPushVerifier(s *Service) ApprovalPushVerifier {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if !s.state.IsLoggedIn() || len(s.state.SigningPublicKey) == 0 {
		return ApprovalPushVerifier{}
	}
	return ApprovalPushVerifier{AccountID: s.state.AccountID, SigningPublicKey: base64.RawURLEncoding.EncodeToString(s.state.SigningPublicKey)}
}

var errNoApprovalPreview = errors.New("the push has no approval preview this phone can trust")

// OpenApprovalPush reads the approval preview in a decrypted push message (an Android phone's Web Push message), once
// the account's signature over it checks out and its request has not expired. It works while Hisar is locked. Not a
// method, so the desktop UI cannot call it.
func OpenApprovalPush(s *Service, message string) (approval.PushPreview, error) {
	var pushMessage apitypes.PushMessage
	if err := json.Unmarshal([]byte(message), &pushMessage); err != nil || pushMessage.Kind != apitypes.PushKindApproval || pushMessage.Approval == nil {
		return approval.PushPreview{}, errNoApprovalPreview
	}
	s.mutex.Lock()
	accountID, signingPublicKey := s.state.AccountID, s.state.SigningPublicKey
	now := s.config.Now()
	s.mutex.Unlock()
	preview, err := approval.OpenPushPreview(signingPublicKey, accountID, pushMessage.Approval, now)
	if err != nil {
		return approval.PushPreview{}, errNoApprovalPreview
	}
	return preview, nil
}
