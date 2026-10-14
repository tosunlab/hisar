package apitypes

import (
	"time"
)

const (
	ShareInvitationPending  = "pending"
	ShareInvitationAccepted = "accepted"
)

type ShareInvitationRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
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
	AcceptedAt *time.Time `json:"acceptedAt,omitempty"`
	Recipient  *Member    `json:"recipient,omitempty"`
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

// ShareInvitationList: AllWaiting is missing from servers older than the field.
type ShareInvitationList struct {
	AllReceived []ReceivedShareInvitation `json:"received"`
	AllSent     []SentShareInvitation     `json:"sent"`
	AllWaiting  []WaitingShareInvitation  `json:"waiting"`
}
