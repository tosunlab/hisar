package session

import "github.com/tryhisar/hisar/core/apitypes"

// Reasons a vault can't be deleted or left; the apps show them as they are.
const (
	PersonalVaultDeleteReason = "Your Personal vault can't be deleted. It is the vault Hisar keeps for you alone, and logins saved from the browser and the iPhone go there."
	OnlyVaultDeleteReason     = "Your only vault can't be deleted. Create another vault first."
	NotOwnerDeleteReason      = "Only an owner of this vault can delete it. You can leave it instead."
	PersonalVaultLeaveReason  = "You can't leave your Personal vault."
	OnlyOwnerLeaveReason      = "You are its only owner. Delete the vault, or make someone else an owner first."
	// NotOrganizationVaultOwnerDeleteReason has no "leave instead": organization vaults can't be left.
	NotOrganizationVaultOwnerDeleteReason = "Only an owner of this vault can delete it."
	// OrganizationVaultLeaveReason: an organization vault's members get it through its teams (§5.11).
	OrganizationVaultLeaveReason = "You have this vault through your teams in its organization. To give up access, ask an organization admin to change them."
)

// VaultPermissions says whether the user may rename, delete or leave a vault, and why not.
type VaultPermissions struct {
	CanRename           bool
	CanDelete           bool
	DeleteBlockedReason string
	CanLeave            bool
	LeaveBlockedReason  string
}

// GetVaultPermissions applies the vault menu's rules to a vault of vaultType where the user has role,
// given how many vaults the user can open and how many owners the vault has.
func GetVaultPermissions(vaultType, role string, vaultCount, ownerCount int) VaultPermissions {
	permissions := VaultPermissions{CanRename: role == apitypes.VaultOwner}
	switch {
	case vaultType == apitypes.VaultPersonal:
		permissions.DeleteBlockedReason = PersonalVaultDeleteReason
	case role != apitypes.VaultOwner && vaultType == apitypes.VaultOrganization:
		permissions.DeleteBlockedReason = NotOrganizationVaultOwnerDeleteReason
	case role != apitypes.VaultOwner:
		permissions.DeleteBlockedReason = NotOwnerDeleteReason
	case vaultCount <= 1:
		permissions.DeleteBlockedReason = OnlyVaultDeleteReason
	default:
		permissions.CanDelete = true
	}
	switch {
	case vaultType == apitypes.VaultPersonal:
		permissions.LeaveBlockedReason = PersonalVaultLeaveReason
	case vaultType == apitypes.VaultOrganization:
		permissions.LeaveBlockedReason = OrganizationVaultLeaveReason
	case role == apitypes.VaultOwner && ownerCount <= 1:
		permissions.LeaveBlockedReason = OnlyOwnerLeaveReason
	default:
		permissions.CanLeave = true
	}
	return permissions
}
