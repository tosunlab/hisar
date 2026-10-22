package apitypes

// VaultTransitionProtocol selects bounded staging and atomic activation, without a legacy fallback.
const VaultTransitionProtocol = 1
const VaultTransitionPageBytes = 8 << 20
const MaxVaultWrappedKeyBytes = 4096

// VaultSnapshot binds ciphertext and authority to one vault state.
type VaultSnapshot struct {
	KeyGeneration    int64 `json:"keyGeneration"`
	ItemSeq          int64 `json:"itemSeq"`
	AccessRevision   int64 `json:"accessRevision"`
	MetadataRevision int64 `json:"metadataRevision"`
}

type BeginVaultTransitionRequest struct {
	Protocol            int    `json:"protocol"`
	ID                  string `json:"id"`
	Kind                string `json:"kind"`
	KeyGeneration       int64  `json:"keyGeneration"`
	TargetID            string `json:"targetId,omitempty"`
	TargetKeyGeneration int64  `json:"targetKeyGeneration,omitempty"`
}

// VaultTransition is also the durable receipt used after an uncertain commit response.
type VaultTransition struct {
	Protocol   int            `json:"protocol"`
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	State      string         `json:"state"`
	Source     VaultSnapshot  `json:"source"`
	SourceName []byte         `json:"sourceName"`
	TargetID   string         `json:"targetId,omitempty"`
	Target     *VaultSnapshot `json:"target,omitempty"`
	ItemCount  int64          `json:"itemCount"`
}

type StageVaultItemsRequest struct {
	Items []RotatedItem `json:"items"`
}
type StageVaultAccessRequest struct {
	Accesses []RotatedAccess `json:"access"`
}
type CommitVaultTransitionRequest struct {
	EncName []byte `json:"encName,omitempty"`
}

// VaultTransitionPage includes tombstones for rotation and only live items for deletion moves.
type VaultTransitionPage struct {
	Items      []Item `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
}
