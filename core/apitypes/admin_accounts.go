package apitypes

import "time"

// The states of an account. A suspended account keeps its data but cannot sign in until an admin reactivates it.
const (
	AccountStateActive    = "active"
	AccountStateSuspended = "suspended"
)

// AccountSuspendedMessage is the answer, 403, to the sign-in of a suspended account with the right credentials.
const AccountSuspendedMessage = "This account is suspended. Contact the admin of the Hisar server."

// AdminAccountPageSize is how many accounts one page of GET /api/v1/admin/accounts holds.
const AdminAccountPageSize = 50

// AdminAccount is an account as the admin dashboard lists it. LastSeenAt is the latest activity of any of its devices,
// nil when it has none.
type AdminAccount struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	Name        string     `json:"name"`
	Role        string     `json:"role"`
	State       string     `json:"state"`
	CreatedAt   time.Time  `json:"createdAt"`
	LastSeenAt  *time.Time `json:"lastSeenAt"`
	DeviceCount int        `json:"deviceCount"`
	VaultCount  int        `json:"vaultCount"`
}

// AdminAccountPage is GET /api/v1/admin/accounts?query=&cursor=: accounts sorted by email. NextCursor, when set, is the
// cursor of the next page.
type AdminAccountPage struct {
	Accounts   []AdminAccount `json:"accounts"`
	NextCursor string         `json:"nextCursor,omitempty"`
}

// AdminDevice is a device of an account, revoked ones included.
type AdminDevice struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Platform   string     `json:"platform"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastSeenAt time.Time  `json:"lastSeenAt"`
	RevokedAt  *time.Time `json:"revokedAt"`
}

// AdminAccountDetail is GET /api/v1/admin/accounts/{id}.
type AdminAccountDetail struct {
	Account AdminAccount  `json:"account"`
	Devices []AdminDevice `json:"devices"`
}

// AdminRoleChangeRequest is the body of PUT /api/v1/admin/accounts/{id}/role; Proof re-authenticates the admin.
type AdminRoleChangeRequest struct {
	Role  string                `json:"role"`
	Proof ReauthenticationProof `json:"proof"`
}

// AdminAccountDeleteRequest is the body of POST /api/v1/admin/accounts/{id}/delete: the email of the account to delete,
// typed by the admin, and Proof, which re-authenticates the admin.
type AdminAccountDeleteRequest struct {
	Email       string                `json:"email"`
	Proof       ReauthenticationProof `json:"proof"`
	PlanVersion string                `json:"planVersion"`
}
