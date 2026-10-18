package apitypes

import "time"

// AdminInviteList is GET /api/v1/admin/invites: the sign-up invites nobody has used that have not expired, newest
// first. The setup invite is not listed.
type AdminInviteList struct {
	Invites []AdminInvite `json:"invites"`
}

// AdminInvite is one pending sign-up invite. Its ID names it in the dashboard's revoke and resend calls; it is not the
// invite code.
type AdminInvite struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
	// CreatedBy is nil for an invite from hisar-server invite.
	CreatedBy *AdminInviteCreator `json:"createdBy"`
	CreatedAt time.Time           `json:"createdAt"`
	ExpiresAt time.Time           `json:"expiresAt"`
}

// AdminInviteCreator is the account that sent an invite.
type AdminInviteCreator struct {
	AccountID string `json:"accountId"`
	Name      string `json:"name"`
	Email     string `json:"email"`
}

// AdminInviteRequest is the body of POST /api/v1/admin/invites; Role is admin or member.
type AdminInviteRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// AdminInviteCreated answers POST /api/v1/admin/invites and POST /api/v1/admin/invites/{id}/resend. The link is in no
// other answer, so the dashboard shows it once.
type AdminInviteCreated struct {
	Invite AdminInvite `json:"invite"`
	Link   string      `json:"link"`
	// IsEmailed is false when email is not set up or sending failed; EmailError then holds the SMTP server's error.
	IsEmailed  bool   `json:"emailed"`
	EmailError string `json:"emailError,omitempty"`
}
