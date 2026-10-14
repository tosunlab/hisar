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

type ConflictResponse struct {
	Current *Item `json:"current"`
}
