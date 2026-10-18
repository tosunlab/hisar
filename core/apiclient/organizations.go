package apiclient

import (
	"context"
	"net/url"

	"github.com/tryhisar/hisar/core/apitypes"
)

func organizationPath(organizationID string) string {
	return "/api/v1/organizations/" + url.PathEscape(organizationID)
}

func teamPath(organizationID, teamID string) string {
	return organizationPath(organizationID) + "/teams/" + url.PathEscape(teamID)
}

func (c *Client) ListOrganizations(ctx context.Context) (*apitypes.OrganizationList, error) {
	var response apitypes.OrganizationList
	return &response, c.sendRequest(ctx, "GET", "/api/v1/organizations", nil, nil, &response, nil)
}

func (c *Client) CreateOrganization(ctx context.Context, request *apitypes.CreateOrganizationRequest) (*apitypes.OrganizationSummary, error) {
	var response apitypes.OrganizationSummary
	return &response, c.sendRequest(ctx, "POST", "/api/v1/organizations", nil, request, &response, nil)
}

func (c *Client) GetOrganization(ctx context.Context, organizationID string) (*apitypes.Organization, error) {
	var response apitypes.Organization
	return &response, c.sendRequest(ctx, "GET", organizationPath(organizationID), nil, nil, &response, nil)
}

func (c *Client) RenameOrganization(ctx context.Context, organizationID string, request *apitypes.RenameOrganizationRequest) error {
	return c.sendRequest(ctx, "PUT", organizationPath(organizationID)+"/name", nil, request, nil, nil)
}

func (c *Client) DeleteOrganization(ctx context.Context, organizationID string) error {
	return c.sendRequest(ctx, "DELETE", organizationPath(organizationID), nil, nil, nil, nil)
}

func (c *Client) InviteToOrganization(ctx context.Context, organizationID string, request *apitypes.OrganizationInvitationRequest) (*apitypes.InvitationSentResponse, error) {
	var response apitypes.InvitationSentResponse
	return &response, c.sendRequest(ctx, "POST", organizationPath(organizationID)+"/invitations", nil, request, &response, nil)
}

func (c *Client) CancelOrganizationInvitation(ctx context.Context, organizationID, invitationID string) error {
	return c.sendRequest(ctx, "DELETE", organizationPath(organizationID)+"/invitations/"+url.PathEscape(invitationID), nil, nil, nil, nil)
}

func (c *Client) AcceptOrganizationInvitation(ctx context.Context, invitationID string) error {
	return c.sendRequest(ctx, "POST", "/api/v1/organization-invitations/"+url.PathEscape(invitationID)+"/accept", nil, nil, nil, nil)
}

func (c *Client) DeclineOrganizationInvitation(ctx context.Context, invitationID string) error {
	return c.sendRequest(ctx, "POST", "/api/v1/organization-invitations/"+url.PathEscape(invitationID)+"/decline", nil, nil, nil, nil)
}

func (c *Client) SetOrganizationMemberRole(ctx context.Context, organizationID, accountID string, request *apitypes.SetOrganizationRoleRequest) error {
	return c.sendRequest(ctx, "PUT", organizationPath(organizationID)+"/members/"+url.PathEscape(accountID), nil, request, nil, nil)
}

// RemoveOrganizationMember removes accountID from the organization; with the caller's own ID it leaves it.
func (c *Client) RemoveOrganizationMember(ctx context.Context, organizationID, accountID string) error {
	return c.sendRequest(ctx, "DELETE", organizationPath(organizationID)+"/members/"+url.PathEscape(accountID), nil, nil, nil, nil)
}

func (c *Client) CreateTeam(ctx context.Context, organizationID string, request *apitypes.CreateTeamRequest) (*apitypes.Team, error) {
	var response apitypes.Team
	return &response, c.sendRequest(ctx, "POST", organizationPath(organizationID)+"/teams", nil, request, &response, nil)
}

func (c *Client) RenameTeam(ctx context.Context, organizationID, teamID string, request *apitypes.RenameTeamRequest) error {
	return c.sendRequest(ctx, "PUT", teamPath(organizationID, teamID)+"/name", nil, request, nil, nil)
}

func (c *Client) DeleteTeam(ctx context.Context, organizationID, teamID string) error {
	return c.sendRequest(ctx, "DELETE", teamPath(organizationID, teamID), nil, nil, nil, nil)
}

func (c *Client) AddTeamMember(ctx context.Context, organizationID, teamID, accountID string) error {
	return c.sendRequest(ctx, "PUT", teamPath(organizationID, teamID)+"/members/"+url.PathEscape(accountID), nil, nil, nil, nil)
}

func (c *Client) RemoveTeamMember(ctx context.Context, organizationID, teamID, accountID string) error {
	return c.sendRequest(ctx, "DELETE", teamPath(organizationID, teamID)+"/members/"+url.PathEscape(accountID), nil, nil, nil, nil)
}

// GiveVaultToTeam gives the team the vault with request.Role, or changes the role it has.
func (c *Client) GiveVaultToTeam(ctx context.Context, organizationID, teamID, vaultID string, request *apitypes.GrantTeamVaultRequest) error {
	return c.sendRequest(ctx, "PUT", teamPath(organizationID, teamID)+"/vaults/"+url.PathEscape(vaultID), nil, request, nil, nil)
}

func (c *Client) TakeVaultFromTeam(ctx context.Context, organizationID, teamID, vaultID string) error {
	return c.sendRequest(ctx, "DELETE", teamPath(organizationID, teamID)+"/vaults/"+url.PathEscape(vaultID), nil, nil, nil, nil)
}

func (c *Client) ListVaultKeyRequests(ctx context.Context) (*apitypes.VaultKeyRequestList, error) {
	var response apitypes.VaultKeyRequestList
	return &response, c.sendRequest(ctx, "GET", "/api/v1/vault-key-requests", nil, nil, &response, nil)
}
