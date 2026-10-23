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
	// NeedsRotation remains set after owner removal until an owner's app commits the replacement key generation.
	NeedsRotation bool `json:"needsRotation,omitempty"`
	// KeyGeneration counts the vault's keys: 1 at creation, one more after every rotation. Missing from older servers.
	KeyGeneration  int64 `json:"keyGeneration,omitempty"`
	AccessRevision int64 `json:"accessRevision,omitempty"`
	// IsPersonalExtra identifies a preserved legacy duplicate, never the designated primary.
	IsPersonalExtra bool `json:"isPersonalExtra,omitempty"`
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

// RotateVaultRequest is the bounded legacy aggregate route. AccessRevision and MetadataRevision bind its snapshot.
type RotateVaultRequest struct {
	EncName          []byte          `json:"encName"`
	Accesses         []RotatedAccess `json:"access"`
	Items            []RotatedItem   `json:"items"`
	KeyGeneration    int64           `json:"keyGeneration,omitempty"`
	AccessRevision   int64           `json:"accessRevision,omitempty"`
	MetadataRevision int64           `json:"metadataRevision,omitempty"`
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

// DeleteVaultRequest deletes items or supplies a bounded legacy move. KeyGeneration identifies the source;
// MoveToKeyGeneration identifies the destination. Rotated vaults require explicit generations.
type DeleteVaultRequest struct {
	MoveToVaultID       string      `json:"moveToVaultId,omitempty"`
	MovedItems          []MovedItem `json:"movedItems,omitempty"`
	IsDeletingItems     bool        `json:"deleteItems,omitempty"`
	MoveToKeyGeneration int64       `json:"moveToKeyGeneration,omitempty"`
	KeyGeneration       int64       `json:"keyGeneration,omitempty"`
}

// VaultReview is the vault record a recovery action was reviewed against: its sealed name, the caller's wrapped key and
// the key generation, as the server listed them. The server refuses the action (409) once any of them changed, so a
// record another device repaired needs a new review.
type VaultReview struct {
	EncName       []byte `json:"encName"`
	WrappedKey    []byte `json:"wrappedKey"`
	KeyGeneration int64  `json:"keyGeneration"`
}

// RepairVaultNameRequest replaces an unreadable sealed name with EncName, sealed with the reviewed key generation.
type RepairVaultNameRequest struct {
	Review  VaultReview `json:"review"`
	EncName []byte      `json:"encName"`
}

// DeleteUnreadableVaultRequest deletes a vault and every item in it, only while it is still as reviewed.
type DeleteUnreadableVaultRequest struct {
	Review VaultReview `json:"review"`
}
