package apitypes

import (
	"encoding/json"
	"time"
)

// AuditPageSize is how many events one page of GET /api/v1/admin/audit holds.
const AuditPageSize = 100

// AuditEvent is one row of the server's audit log. AccountEmail is null when the account was deleted; Detail is the
// JSON the event recorded, or null.
type AuditEvent struct {
	ID           int64           `json:"id"`
	AccountID    *string         `json:"accountId"`
	AccountEmail *string         `json:"accountEmail"`
	DeviceID     *string         `json:"deviceId"`
	Type         string          `json:"type"`
	Detail       json.RawMessage `json:"detail"`
	IP           string          `json:"ip"`
	CreatedAt    time.Time       `json:"createdAt"`
}

// AuditPage is the answer of GET /api/v1/admin/audit: events newest first, and the cursor of the next page, empty on
// the last one.
type AuditPage struct {
	Events     []AuditEvent `json:"events"`
	NextCursor string       `json:"nextCursor"`
}

// AuditType is one type of audit event with the label the admin dashboard shows.
type AuditType struct {
	Type  string `json:"type"`
	Label string `json:"label"`
}

// AuditTypeGroup is one group of the dashboard's type filter; the group's ID filters by all of its types.
type AuditTypeGroup struct {
	ID    string      `json:"id"`
	Label string      `json:"label"`
	Types []AuditType `json:"types"`
}

// AuditTypesResponse is the answer of GET /api/v1/admin/audit/types.
type AuditTypesResponse struct {
	Groups []AuditTypeGroup `json:"groups"`
}
