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

type CreateApprovalRequest struct {
	RequestID     string    `json:"requestId"`
	SealedRequest []byte    `json:"sealedRequest"`
	ExpiresAt     time.Time `json:"expiresAt"`
}

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
