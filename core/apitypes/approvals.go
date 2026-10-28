package apitypes

import (
	"time"
)

// Phone approvals: the server relays sealed requests and answers between the devices of one account.
const (
	EventApprovalRequested = "approval-requested"
	EventApprovalAnswered  = "approval-answered"
	EventApprovalResolved  = "approval-resolved"
)

type ApprovalEvent struct {
	RequestID string `json:"requestId"`
}

// CreateApprovalRequest is the body of POST /api/v1/approvals. AllPushMessage, from a Mac that read the phones' keys
// (GET /api/v1/push/keys), are the push messages it encrypted to each phone; the server sends a phone its own
// while P256DH is still the key the phone gets pushes with, and the kind alone otherwise.
type CreateApprovalRequest struct {
	RequestID      string                `json:"requestId"`
	SealedRequest  []byte                `json:"sealedRequest"`
	ExpiresAt      time.Time             `json:"expiresAt"`
	AllPushMessage []ApprovalPushMessage `json:"pushMessages,omitempty"`
}

// ApprovalPushMessage is a PushMessage encrypted to one phone's key (RFC 8291, base64url).
type ApprovalPushMessage struct {
	DeviceID string `json:"deviceId"`
	P256DH   string `json:"p256dh"`
	Message  string `json:"message"`
}

// MaximumApprovalPushMessages bounds how many phones one request names.
const MaximumApprovalPushMessages = 16

type PendingApproval struct {
	RequestID         string    `json:"requestId"`
	SealedRequest     []byte    `json:"sealedRequest"`
	RequesterDeviceID string    `json:"requesterDeviceId"`
	CreatedAt         time.Time `json:"createdAt"`
	ExpiresAt         time.Time `json:"expiresAt"`
}

// Approval is one request with its answer; SealedAnswer is nil until a device answered.
type Approval struct {
	RequestID     string    `json:"requestId"`
	SealedRequest []byte    `json:"sealedRequest"`
	SealedAnswer  []byte    `json:"sealedAnswer"`
	IsResolved    bool      `json:"isResolved"`
	ExpiresAt     time.Time `json:"expiresAt"`
}

type AnswerApprovalRequest struct {
	SealedAnswer []byte `json:"sealedAnswer"`
}
