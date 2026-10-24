package apitypes

import (
	"github.com/tryhisar/hisar/core/crypt"
	"time"
)

const (
	ShareInvitationPending  = "pending"
	ShareInvitationAccepted = "accepted"
)

type ShareInvitationRequest struct {
	Email  string             `json:"email"`
	Role   string             `json:"role"`
	Intent *SignedShareIntent `json:"intent,omitempty"`
}

type ReceivedShareInvitation struct {
	ID             string    `json:"id"`
	VaultID        string    `json:"vaultId"`
	Role           string    `json:"role"`
	InvitedByName  string    `json:"invitedByName"`
	InvitedByEmail string    `json:"invitedByEmail"`
	CreatedAt      time.Time `json:"createdAt"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

// SentShareInvitation is an invitation to a vault the caller owns; Recipient is set only once it was accepted.
type SentShareInvitation struct {
	ID        string    `json:"id"`
	VaultID   string    `json:"vaultId"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	State     string    `json:"state"`
	InvitedBy string    `json:"invitedBy"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	// AcceptedAt is set only once it was accepted.
	AcceptedAt *time.Time         `json:"acceptedAt,omitempty"`
	Recipient  *Member            `json:"recipient,omitempty"`
	Intent     *SignedShareIntent `json:"intent,omitempty"`
}

// WaitingShareInvitation is an invitation the caller accepted that no owner's app has completed yet.
type WaitingShareInvitation struct {
	ID             string    `json:"id"`
	VaultID        string    `json:"vaultId"`
	Role           string    `json:"role"`
	InvitedByName  string    `json:"invitedByName"`
	InvitedByEmail string    `json:"invitedByEmail"`
	AcceptedAt     time.Time `json:"acceptedAt"`
}

// ShareInvitationList: Waiting is missing from servers older than the field.
type ShareInvitationList struct {
	Received []ReceivedShareInvitation `json:"received"`
	Sent     []SentShareInvitation     `json:"sent"`
	Waiting  []WaitingShareInvitation  `json:"waiting"`
}

type SignedShareIntent struct {
	Context   crypt.ShareIntent `json:"context"`
	Signature []byte            `json:"signature"`
}

type CompleteShareRequest struct {
	Intent     SignedShareIntent `json:"intent"`
	WrappedKey []byte            `json:"wrappedKey"`
}
