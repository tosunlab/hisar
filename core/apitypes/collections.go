package apitypes

import "encoding/json"

const CollectionPageVersion = "1"
const CollectionPageHeader = "Hisar-Collection-Pages"
const CollectionPageBytes = 8 << 20
const CollectionPageRows = 100
const CollectionChangedMessage = "the collection changed during traversal; reload it before continuing"

// CollectionRecord flattens nested lists so even a single team's edges can span pages.
// Field names are the existing response's JSON field names. An empty Field denotes a root array.
type CollectionRecord struct {
	Field  string          `json:"field"`
	Parent string          `json:"parent,omitempty"`
	Key    string          `json:"key"`
	Value  json.RawMessage `json:"value"`
}
type CollectionPage struct {
	Version     string             `json:"version"`
	Records     []CollectionRecord `json:"records"`
	NextCursor  string             `json:"nextCursor,omitempty"`
	Snapshot    string             `json:"snapshot,omitempty"`
	PlanVersion string             `json:"planVersion,omitempty"`
}
