package apitypes

import (
	"github.com/tryhisar/hisar/core/crypt"
)

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

type DeleteAccountRequest struct {
	ReauthenticationID string `json:"reauthenticationId"`
	KE3                []byte `json:"ke3"`
	Email              string `json:"email"`
}

// AccountDeletionPreview names vaults by ID only: their names are encrypted.
type AccountDeletionPreview struct {
	OwnedVaults             []DeletionOwnedVault  `json:"ownedVaults"`
	MemberVaults            []DeletionMemberVault `json:"memberVaults"`
	ShareLinkCount          int                   `json:"shareLinkCount"`
	SentInvitationCount     int                   `json:"sentInvitationCount"`
	ReceivedInvitationCount int                   `json:"receivedInvitationCount"`
	DeviceCount             int                   `json:"deviceCount"`
	// Organizations is missing from servers older than organizations.
	Organizations []DeletionOrganization `json:"organizations,omitempty"`
}

// DeletionOrganization is an organization the account is a member of. IsDeleted: the account is its last member.
// NewAdminName names the member who becomes admin when the account is its last admin.
type DeletionOrganization struct {
	OrganizationID string `json:"organizationId"`
	Name           string `json:"name"`
	IsDeleted      bool   `json:"deleted"`
	NewAdminName   string `json:"newAdminName,omitempty"`
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
