package apitypes

// Setup is GET /api/v1/setup: how far setting up a fresh server from the admin dashboard got. State is "needsAdmin",
// "needsChecks", "needsInvitePolicy" or "done".
type Setup struct {
	State string `json:"state"`
}

// SetupChecks is GET /api/v1/admin/setup/checks: the instance checks, as GET /api/v1/admin/health lists them. A POST to
// the same path moves setup on when none failed.
type SetupChecks struct {
	Checks []InstanceCheck `json:"checks"`
}

// SetupInvitePolicyRequest is the body of POST /api/v1/admin/setup/invite-policy, the last setup step: "anyone" or
// "admins".
type SetupInvitePolicyRequest struct {
	InvitePolicy string `json:"invitePolicy"`
}
