package apitypes

import "time"

// AdminHealth is the answer of GET /api/v1/admin/health.
type AdminHealth struct {
	Version string `json:"version"`
	// LatestVersion is the newest server release when it is newer than Version, else empty.
	LatestVersion string              `json:"latestVersion"`
	Database      AdminHealthDatabase `json:"database"`
	Counts        AdminHealthCounts   `json:"counts"`
	// StorageBytes is the size of the server's database on disk.
	StorageBytes int64           `json:"storageBytes"`
	Checks       []InstanceCheck `json:"checks"`
}

// AdminHealthDatabase is the schema's state: OK when every migration the server knows is applied.
type AdminHealthDatabase struct {
	OK         bool             `json:"ok"`
	Migrations []AdminMigration `json:"migrations"`
}

// AdminMigration is one schema migration; AppliedAt is nil while it is pending.
type AdminMigration struct {
	Name      string     `json:"name"`
	AppliedAt *time.Time `json:"appliedAt"`
}

// AdminHealthCounts counts every account, the suspended ones among them, live items, share links that have not expired
// and sign-up invites still waiting to be used.
type AdminHealthCounts struct {
	Accounts          int64 `json:"accounts"`
	SuspendedAccounts int64 `json:"suspendedAccounts"`
	Vaults            int64 `json:"vaults"`
	Items             int64 `json:"items"`
	ShareLinks        int64 `json:"shareLinks"`
	PendingInvites    int64 `json:"pendingInvites"`
}

// The outcomes of an instance check.
const (
	CheckStatusOK      = "ok"
	CheckStatusWarning = "warning"
	CheckStatusFailed  = "failed"
)

// InstanceCheck is the outcome of one check of how the server is set up.
type InstanceCheck struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	Message string `json:"message"`
}
