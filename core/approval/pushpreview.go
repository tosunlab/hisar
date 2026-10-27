package approval

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
)

// maximumPreviewFieldLength bounds each text of a preview, so a push holding it stays within the 4 KB APNs takes.
const maximumPreviewFieldLength = 160

// ErrInvalidPreview means a push's preview did not verify, or no longer describes a waiting request.
var ErrInvalidPreview = errors.New("approval: the push preview is not valid")

// PushPreview is what a phone's notification says about a request before Hisar is open: the request's lines that make
// the phones' approval banner title and summary. Its JSON keys are those of Request and Description.
type PushPreview struct {
	RequestID           string    `json:"requestId"`
	RequesterDeviceName string    `json:"requesterDeviceName"`
	ExpiresAt           time.Time `json:"expiresAt"`
	Kind                string    `json:"kind"`
	AppName             string    `json:"appName"`
	Purpose             string    `json:"purpose"`
	KeyName             string    `json:"keyName"`
	IsForwarded         bool      `json:"isForwarded"`
	ForwardedFromHost   string    `json:"forwardedFromHost"`
}

// NewPushPreview takes the request's lines a notification shows, each clipped.
func NewPushPreview(request Request) PushPreview {
	return PushPreview{
		RequestID:           request.RequestID,
		RequesterDeviceName: clipPreviewField(request.RequesterDeviceName),
		ExpiresAt:           request.ExpiresAt,
		Kind:                request.Kind,
		AppName:             clipPreviewField(request.AppName),
		Purpose:             clipPreviewField(request.Purpose),
		KeyName:             clipPreviewField(request.KeyName),
		IsForwarded:         request.IsForwarded,
		ForwardedFromHost:   clipPreviewField(request.ForwardedFromHost),
	}
}

func clipPreviewField(text string) string {
	if runes := []rune(text); len(runes) > maximumPreviewFieldLength {
		return string(runes[:maximumPreviewFieldLength-1]) + "…"
	}
	return text
}

// SignPushPreview is the approval part of the push message a Mac encrypts to each phone.
func SignPushPreview(keyset *crypt.Keyset, accountID string, preview PushPreview) (*apitypes.PushApproval, error) {
	encoded, err := json.Marshal(preview)
	if err != nil {
		return nil, err
	}
	return &apitypes.PushApproval{
		Preview:   base64.RawURLEncoding.EncodeToString(encoded),
		Signature: base64.RawURLEncoding.EncodeToString(keyset.SignApprovalPreview(accountID, encoded)),
	}, nil
}

// OpenPushPreview checks a push's preview against the account's signing key and returns it while its request waits.
func OpenPushPreview(signingPublicKey []byte, accountID string, pushApproval *apitypes.PushApproval, now time.Time) (PushPreview, error) {
	var preview PushPreview
	if pushApproval == nil {
		return preview, ErrInvalidPreview
	}
	encoded, previewErr := base64.RawURLEncoding.DecodeString(pushApproval.Preview)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(pushApproval.Signature)
	if previewErr != nil || signatureErr != nil || crypt.VerifyApprovalPreview(signingPublicKey, accountID, encoded, signature) != nil {
		return preview, ErrInvalidPreview
	}
	if err := json.Unmarshal(encoded, &preview); err != nil || !now.Before(preview.ExpiresAt) || strings.TrimSpace(preview.Kind) == "" {
		return PushPreview{}, ErrInvalidPreview
	}
	return preview, nil
}
