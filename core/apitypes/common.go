package apitypes

import "strings"

const (
	RoleAdmin  = "admin"
	RoleMember = "member"

	VaultOwner  = "owner"
	VaultEditor = "editor"
	VaultReader = "reader"

	VaultPersonal = "personal"
	VaultShared   = "shared"
	// VaultOrganization belongs to an organization; its members get it through the teams it was given (§5.11).
	VaultOrganization = "organization"
)

type Error struct {
	Error string `json:"error"`
}

type DeviceInfo struct {
	Name     string `json:"name"`
	Platform string `json:"platform"`
}

// IsValidVaultRole reports whether role is one a vault member can have.
func IsValidVaultRole(role string) bool {
	return role == VaultOwner || role == VaultEditor || role == VaultReader
}

// NormalizeEmail is the form emails are stored and compared in.
func NormalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

// CanWriteItems reports whether a member with role may add and change items, and so receive moved items.
func CanWriteItems(role string) bool { return role == VaultOwner || role == VaultEditor }
