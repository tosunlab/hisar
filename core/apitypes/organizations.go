package apitypes

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/tryhisar/hisar/core/crypt"
)

const (
	OrganizationRoleAdmin  = "admin"
	OrganizationRoleMember = "member"

	// MaxOrganizationNameLength bounds organization and team names, in characters.
	MaxOrganizationNameLength = 100
)

// IsValidOrganizationRole reports whether role is one an organization member can have.
func IsValidOrganizationRole(role string) bool {
	return role == OrganizationRoleAdmin || role == OrganizationRoleMember
}

// CleanOrganizationName trims an organization or team name and reports whether it is usable:
// 1 to MaxOrganizationNameLength characters, no control characters.
func CleanOrganizationName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > MaxOrganizationNameLength {
		return "", false
	}
	if strings.ContainsFunc(name, unicode.IsControl) {
		return "", false
	}
	return name, true
}

// RankVaultRole orders vault roles so a member of several teams gets the strongest: reader < editor < owner.
// It is 0 for anything else.
func RankVaultRole(role string) int {
	switch role {
	case VaultReader:
		return 1
	case VaultEditor:
		return 2
	case VaultOwner:
		return 3
	}
	return 0
}

type CreateOrganizationRequest struct {
	Name string `json:"name"`
}

type RenameOrganizationRequest struct {
	Name string `json:"name"`
}

// OrganizationSummary is one organization the caller belongs to, with the caller's role in it.
type OrganizationSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Role        string `json:"role"`
	MemberCount int    `json:"memberCount"`
}

// ReceivedOrganizationInvitation is an unexpired invitation to the caller's email.
type ReceivedOrganizationInvitation struct {
	ID               string    `json:"id"`
	OrganizationID   string    `json:"organizationId"`
	OrganizationName string    `json:"organizationName"`
	Role             string    `json:"role"`
	InvitedByName    string    `json:"invitedByName"`
	InvitedByEmail   string    `json:"invitedByEmail"`
	CreatedAt        time.Time `json:"createdAt"`
	ExpiresAt        time.Time `json:"expiresAt"`
}

// OrganizationList is GET /organizations.
type OrganizationList struct {
	Organizations []OrganizationSummary            `json:"organizations"`
	Invitations   []ReceivedOrganizationInvitation `json:"invitations"`
}

// OrganizationMember is a member as every other member of the organization sees them.
type OrganizationMember struct {
	AccountID  string           `json:"accountId"`
	Email      string           `json:"email"`
	Name       string           `json:"name"`
	Role       string           `json:"role"`
	PublicKeys crypt.PublicKeys `json:"publicKeys"`
}

// TeamVault is a vault a team was given, with the role its members get. Vault names are encrypted, so only the ID.
type TeamVault struct {
	VaultID string `json:"vaultId"`
	Role    string `json:"role"`
}

type Team struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	MemberIDs []string    `json:"memberIds"`
	Vaults    []TeamVault `json:"vaults"`
}

// SentOrganizationInvitation is a pending invitation to the organization; only admins see them.
type SentOrganizationInvitation struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	InvitedBy string    `json:"invitedBy"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// Organization is GET /organizations/{id}, for its members.
type Organization struct {
	ID          string                       `json:"id"`
	Name        string                       `json:"name"`
	Role        string                       `json:"role"`
	Members     []OrganizationMember         `json:"members"`
	Teams       []Team                       `json:"teams"`
	Invitations []SentOrganizationInvitation `json:"invitations"`
}

type OrganizationInvitationRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type SetOrganizationRoleRequest struct {
	Role string `json:"role"`
}

type CreateTeamRequest struct {
	Name string `json:"name"`
}

type RenameTeamRequest struct {
	Name string `json:"name"`
}

type GrantTeamVaultRequest struct {
	Role string `json:"role"`
}

// VaultKeyRequest is an account a team entitles to an organization vault that has no wrapped key yet.
// Only the vault's owners see it; their apps wrap the key to Recipient and PUT /vaults/{id}/access/{accountID}.
type VaultKeyRequest struct {
	VaultID       string `json:"vaultId"`
	Role          string `json:"role"`
	KeyGeneration int64  `json:"keyGeneration"`
	Recipient     Member `json:"recipient"`
}

// WaitingVaultKey is an organization vault the caller's teams entitle them to that no owner's app has delivered yet.
type WaitingVaultKey struct {
	VaultID        string `json:"vaultId"`
	OrganizationID string `json:"organizationId"`
	Role           string `json:"role"`
}

// VaultKeyRequestList is GET /vault-key-requests.
type VaultKeyRequestList struct {
	Pending []VaultKeyRequest `json:"pending"`
	Waiting []WaitingVaultKey `json:"waiting"`
}
