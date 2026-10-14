// Package proto defines the JSON wire types of the /api/v1 HTTP API.
package proto

import (
	"time"

	"github.com/tryhisar/hisar/core/crypt"
)

const (
	RoleAdmin  = "admin"
	RoleMember = "member"

	VaultOwner  = "owner"
	VaultEditor = "editor"
	VaultReader = "reader"

	VaultPersonal = "personal"
	VaultShared   = "shared"
)

type Error struct {
	Error string `json:"error"`
}

type PreloginResponse struct {
	Salt   []byte             `json:"salt"`
	Argon2 crypt.Argon2Params `json:"argon2"`
}

type DeviceInfo struct {
	Name     string `json:"name"`
	Platform string `json:"platform"`
}

type RegisterStartRequest struct {
	Invite    string `json:"invite"`
	Email     string `json:"email"`
	AccountID string `json:"accountId"`
	Request   []byte `json:"request"`
}

type RegisterStartResponse struct {
	Response []byte `json:"response"`
}

type RegisterFinishRequest struct {
	Invite     string             `json:"invite"`
	Email      string             `json:"email"`
	Name       string             `json:"name"`
	AccountID  string             `json:"accountId"`
	Record     []byte             `json:"record"`
	Salt       []byte             `json:"salt"`
	Argon2     crypt.Argon2Params `json:"argon2"`
	EncKeyset  []byte             `json:"encKeyset"`
	PublicKeys crypt.PublicKeys   `json:"publicKeys"`
	Device     DeviceInfo         `json:"device"`
}

type LoginStartRequest struct {
	Email string `json:"email"`
	KE1   []byte `json:"ke1"`
}

type LoginStartResponse struct {
	LoginID string `json:"loginId"`
	KE2     []byte `json:"ke2"`
}

type LoginFinishRequest struct {
	LoginID string     `json:"loginId"`
	KE3     []byte     `json:"ke3"`
	Device  DeviceInfo `json:"device"`
}

type SessionResponse struct {
	Token     string `json:"token"`
	AccountID string `json:"accountId"`
	DeviceID  string `json:"deviceId"`
}

type Account struct {
	AccountID  string             `json:"accountId"`
	Email      string             `json:"email"`
	Name       string             `json:"name"`
	Role       string             `json:"role"`
	Argon2     crypt.Argon2Params `json:"argon2"`
	Salt       []byte             `json:"salt"`
	EncKeyset  []byte             `json:"encKeyset"`
	PublicKeys crypt.PublicKeys   `json:"publicKeys"`
}

type Member struct {
	AccountID  string           `json:"accountId"`
	Email      string           `json:"email"`
	Name       string           `json:"name"`
	Role       string           `json:"role"`
	PublicKeys crypt.PublicKeys `json:"publicKeys"`
}

// InviteRequest asks the server to email a sign-up invite; Role may only be empty or "member".
type InviteRequest struct {
	Email string `json:"email"`
	Role  string `json:"role,omitempty"`
}

// InviteResponse is what the operator's hisar-server invite prints; the API never returns it.
type InviteResponse struct {
	Token string `json:"token"`
	URL   string `json:"url"`
}

// InvitationSentResponse is the same for every invite and share invitation, whether or not the email has an account.
type InvitationSentResponse struct {
	Message string `json:"message"`
}

const InvitationSentMessage = "Invitation sent"

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

type Device struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Platform   string    `json:"platform"`
	CreatedAt  time.Time `json:"createdAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`
	Current    bool      `json:"current"`
}

type Vault struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	EncName    []byte `json:"encName"`
	Seq        int64  `json:"seq"`
	Role       string `json:"role"`
	WrappedKey []byte `json:"wrappedKey"`
}

type CreateVaultRequest struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	EncName    []byte `json:"encName"`
	WrappedKey []byte `json:"wrappedKey"`
}

type VaultMember struct {
	AccountID string `json:"accountId"`
	Role      string `json:"role"`
	GrantedBy string `json:"grantedBy"`
}

type SetAccessRequest struct {
	WrappedKey []byte `json:"wrappedKey"`
	Role       string `json:"role"`
}

type Item struct {
	ID        string    `json:"id"`
	Revision  string    `json:"revision"`
	Seq       int64     `json:"seq"`
	Deleted   bool      `json:"deleted"`
	Overview  []byte    `json:"overview"`
	Details   []byte    `json:"details"`
	UpdatedAt time.Time `json:"updatedAt"`
	UpdatedBy string    `json:"updatedBy"`
}

type ChangesResponse struct {
	Items   []Item `json:"items"`
	NextSeq int64  `json:"nextSeq"`
	More    bool   `json:"more"`
}

// PutItemRequest is sent with an If-Match header carrying the base revision (absent for new items).
type PutItemRequest struct {
	Revision string `json:"revision"`
	Deleted  bool   `json:"deleted"`
	Overview []byte `json:"overview"`
	Details  []byte `json:"details"`
}

type PutItemResponse struct {
	Seq int64 `json:"seq"`
}

// Server-sent events on GET /api/v1/events.
const (
	EventVaultChanged  = "vault-changed"
	EventVaultsChanged = "vaults-changed"
	// EventShareInvitationsChanged carries {}: the account's received or sent share invitations changed.
	EventShareInvitationsChanged = "share-invitations-changed"
	MaxEventSize                 = 64 << 10
)

// VaultChangedEvent says a vault reached Seq; Seq is 0 when the server does not know it, and clients sync either way.
type VaultChangedEvent struct {
	VaultID string `json:"vaultID"`
	Seq     int64  `json:"seq"`
}

type ConflictResponse struct {
	Current *Item `json:"current"`
}

type RotatedAccess struct {
	AccountID  string `json:"accountId"`
	Role       string `json:"role"`
	WrappedKey []byte `json:"wrappedKey"`
}

type RotatedItem struct {
	ID           string `json:"id"`
	BaseRevision string `json:"baseRevision"`
	Revision     string `json:"revision"`
	Deleted      bool   `json:"deleted"`
	Overview     []byte `json:"overview"`
	Details      []byte `json:"details"`
}

// RotateVaultRequest replaces the vault key: every member's wrapped key and every item, re-encrypted.
type RotateVaultRequest struct {
	EncName   []byte          `json:"encName"`
	AllAccess []RotatedAccess `json:"access"`
	AllItem   []RotatedItem   `json:"items"`
}

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

const EventShareLinksChanged = "share-links-changed"

type ShareLinkPassphrase struct {
	Salt       []byte `json:"salt"`
	Iterations int    `json:"iterations"`
}

type CreateShareLinkRequest struct {
	Ciphertext       []byte               `json:"ciphertext"`
	EncryptedLabel   []byte               `json:"encryptedLabel"`
	ExpiresInSeconds int64                `json:"expiresInSeconds"`
	MaxViews         int                  `json:"maxViews"`
	Passphrase       *ShareLinkPassphrase `json:"passphrase"`
}

// ShareLink is the creator's view of a link; MaxViews 0 means unlimited.
type ShareLink struct {
	ID             string    `json:"id"`
	EncryptedLabel []byte    `json:"encryptedLabel,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	ExpiresAt      time.Time `json:"expiresAt"`
	MaxViews       int       `json:"maxViews"`
	ViewCount      int       `json:"viewCount"`
	HasPassphrase  bool      `json:"hasPassphrase"`
}

// PublicShareLink is what anyone holding the ID gets; every fetch counts a view.
type PublicShareLink struct {
	Ciphertext []byte               `json:"ciphertext"`
	Passphrase *ShareLinkPassphrase `json:"passphrase"`
	ExpiresAt  time.Time            `json:"expiresAt"`
	MaxViews   int                  `json:"maxViews"`
	ViewCount  int                  `json:"viewCount"`
}

const (
	CredentialChangeMasterPassword = "master-password"
	CredentialChangeSecretKey      = "secret-key"
)

// ReauthenticateRequest carries RegistrationRequest only when the credentials are about to change.
type ReauthenticateRequest struct {
	KE1                 []byte `json:"ke1"`
	RegistrationRequest []byte `json:"registrationRequest,omitempty"`
}

type ReauthenticateResponse struct {
	ReauthenticationID   string `json:"reauthenticationId"`
	KE2                  []byte `json:"ke2"`
	RegistrationResponse []byte `json:"registrationResponse,omitempty"`
}

type ChangeCredentialsRequest struct {
	ReauthenticationID string             `json:"reauthenticationId"`
	KE3                []byte             `json:"ke3"`
	Change             string             `json:"change"`
	Record             []byte             `json:"record"`
	Salt               []byte             `json:"salt"`
	Argon2             crypt.Argon2Params `json:"argon2"`
	EncKeyset          []byte             `json:"encKeyset"`
}

type ChangeCredentialsResponse struct {
	SignedOutDeviceCount int `json:"signedOutDeviceCount"`
}

type DeleteAccountRequest struct {
	ReauthenticationID string `json:"reauthenticationId"`
	KE3                []byte `json:"ke3"`
	Email              string `json:"email"`
}

// AccountDeletionPreview names vaults by ID only: their names are encrypted.
type AccountDeletionPreview struct {
	AllOwnedVault           []DeletionOwnedVault  `json:"ownedVaults"`
	AllMemberVault          []DeletionMemberVault `json:"memberVaults"`
	ShareLinkCount          int                   `json:"shareLinkCount"`
	SentInvitationCount     int                   `json:"sentInvitationCount"`
	ReceivedInvitationCount int                   `json:"receivedInvitationCount"`
	DeviceCount             int                   `json:"deviceCount"`
}

// DeletionOwnedVault has no other owner, so it is deleted with the account.
type DeletionOwnedVault struct {
	VaultID          string `json:"vaultId"`
	Type             string `json:"type"`
	OtherMemberCount int    `json:"otherMemberCount"`
}

// DeletionMemberVault has another owner, so only the account's access is removed.
type DeletionMemberVault struct {
	VaultID string `json:"vaultId"`
	Type    string `json:"type"`
	Role    string `json:"role"`
}
