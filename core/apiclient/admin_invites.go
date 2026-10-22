package apiclient

import (
	"context"
	"net/url"

	"github.com/tryhisar/hisar/core/apitypes"
)

// CreateAdminInvite requires a fresh session-bound proof when request.Role is admin.
func (c *Client) CreateAdminInvite(ctx context.Context, request *apitypes.AdminInviteRequest) (*apitypes.AdminInviteCreated, error) {
	var response apitypes.AdminInviteCreated
	return &response, c.sendRequest(ctx, "POST", "/api/v1/admin/invites", nil, request, &response, nil)
}

// ResendAdminInvite replaces an invite, requiring a fresh proof for the admin role.
func (c *Client) ResendAdminInvite(ctx context.Context, inviteID string, request *apitypes.AdminInviteResendRequest) (*apitypes.AdminInviteCreated, error) {
	var response apitypes.AdminInviteCreated
	return &response, c.sendRequest(ctx, "POST", "/api/v1/admin/invites/"+url.PathEscape(inviteID)+"/resend", nil, request, &response, nil)
}
