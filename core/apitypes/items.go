package apitypes

import (
	"time"
)

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

// MoveItemRequest moves a live item to another vault in one step, keeping its ID. BaseRevision is its current revision in
// the source vault; Tombstone is the deleted revision left there (Deleted true) and Moved the item re-encrypted under the
// target vault's key (Deleted false), each with its own new revision.
type MoveItemRequest struct {
	TargetVaultID string         `json:"targetVaultId"`
	BaseRevision  string         `json:"baseRevision"`
	Tombstone     PutItemRequest `json:"tombstone"`
	Moved         PutItemRequest `json:"moved"`
}

// MoveItemResponse holds the new change sequence numbers of the source and target vaults.
type MoveItemResponse struct {
	SourceSeq int64 `json:"sourceSeq"`
	TargetSeq int64 `json:"targetSeq"`
}

type ConflictResponse struct {
	Current *Item `json:"current"`
}
