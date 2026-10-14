package apitypes

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
	EncName  []byte          `json:"encName"`
	Accesses []RotatedAccess `json:"access"`
	Items    []RotatedItem   `json:"items"`
}

type RenameVaultRequest struct {
	EncName []byte `json:"encName"`
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
// or IsDeletingItems.
type DeleteVaultRequest struct {
	MoveToVaultID   string      `json:"moveToVaultId,omitempty"`
	MovedItems      []MovedItem `json:"movedItems,omitempty"`
	IsDeletingItems bool        `json:"deleteItems,omitempty"`
}
