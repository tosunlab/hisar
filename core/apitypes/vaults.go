package apitypes

type Vault struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	EncName    []byte `json:"encName"`
	Seq        int64  `json:"seq"`
	Role       string `json:"role"`
	WrappedKey []byte `json:"wrappedKey"`
	// OrganizationID is set for organization vaults only.
	OrganizationID string `json:"organizationId,omitempty"`
	// NeedsRotation is set when someone lost access through a team and an owner's app has not rotated the key since.
	NeedsRotation bool `json:"needsRotation,omitempty"`
	// KeyGeneration counts the vault's keys: 1 at creation, one more after every rotation. Missing from older servers.
	KeyGeneration  int64 `json:"keyGeneration,omitempty"`
	AccessRevision int64 `json:"accessRevision,omitempty"`
}

// CreateVaultRequest: an organization vault names its organization and the team that is given it as owner;
// the caller must be a member of that team.
type CreateVaultRequest struct {
	ID             string `json:"id"`
	Type           string `json:"type"`
	EncName        []byte `json:"encName"`
	WrappedKey     []byte `json:"wrappedKey"`
	OrganizationID string `json:"organizationId,omitempty"`
	TeamID         string `json:"teamId,omitempty"`
}

type VaultMember struct {
	AccountID string `json:"accountId"`
	Role      string `json:"role"`
	GrantedBy string `json:"grantedBy"`
}

// SetAccessRequest identifies the key being wrapped. Omission is accepted only for unrotated nonorganization vaults.
type SetAccessRequest struct {
	WrappedKey    []byte `json:"wrappedKey"`
	Role          string `json:"role"`
	KeyGeneration int64  `json:"keyGeneration,omitempty"`
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

// RotateVaultRequest replaces the vault key: every member's wrapped key and every item, re-encrypted. KeyGeneration
// identifies the key being replaced. Omission is accepted only for unrotated nonorganization vaults.
type RotateVaultRequest struct {
	EncName       []byte          `json:"encName"`
	Accesses      []RotatedAccess `json:"access"`
	Items         []RotatedItem   `json:"items"`
	KeyGeneration int64           `json:"keyGeneration,omitempty"`
}

// RenameVaultRequest identifies the key used for the name. Omission is accepted only for unrotated nonorganization vaults.
type RenameVaultRequest struct {
	EncName       []byte `json:"encName"`
	KeyGeneration int64  `json:"keyGeneration,omitempty"`
}

// MovedItem is a live item re-encrypted under the target vault's key; BaseRevision is its revision in the deleted vault.
type MovedItem struct {
	ID           string `json:"id"`
	BaseRevision string `json:"baseRevision"`
	Revision     string `json:"revision"`
	Overview     []byte `json:"overview"`
	Details      []byte `json:"details"`
}

// DeleteVaultRequest: a vault with live items needs either MoveToVaultID with every live item in MovedItems,
// or IsDeletingItems. KeyGeneration identifies the source; MoveToKeyGeneration identifies the destination.
// Rotated vaults require explicit generations.
type DeleteVaultRequest struct {
	MoveToVaultID       string      `json:"moveToVaultId,omitempty"`
	MovedItems          []MovedItem `json:"movedItems,omitempty"`
	IsDeletingItems     bool        `json:"deleteItems,omitempty"`
	MoveToKeyGeneration int64       `json:"moveToKeyGeneration,omitempty"`
	KeyGeneration       int64       `json:"keyGeneration,omitempty"`
}
