package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/session"
)

func TestCancelledMembershipSignupCapability(t *testing.T) {
	for _, kind := range []string{"share", "organization"} {
		t.Run(kind, func(t *testing.T) {
			baseURL, server, _ := setupServer(t)
			ctx := context.Background()
			owner := signUpTestAccount(t, ctx, server, baseURL, "owner@example.com", apitypes.RoleAdmin)
			recorder := attachMailRecorder(server)
			email := "recipient@example.com"
			independent := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, email, apitypes.RoleMember, &owner.AccountID))
			var cancel func() error
			if kind == "share" {
				vault := requireValue[*session.Vault](t)(owner.CreateVault(ctx, "Shared", apitypes.VaultShared))
				if err := owner.InviteToVault(ctx, vault, email, apitypes.VaultReader); err != nil {
					t.Fatal(err)
				}
				list := requireValue[*apitypes.ShareInvitationList](t)(owner.Client.ListShareInvitations(ctx))
				if len(list.Sent) != 1 {
					t.Fatalf("sent share invitations: %+v", list)
				}
				cancel = func() error { return owner.Client.CancelShareInvitation(ctx, list.Sent[0].ID) }
			} else {
				organization := requireValue[*apitypes.OrganizationSummary](t)(owner.Client.CreateOrganization(ctx, &apitypes.CreateOrganizationRequest{Name: "Acme"}))
				if _, err := owner.Client.InviteToOrganization(ctx, organization.ID, &apitypes.OrganizationInvitationRequest{Email: email, Role: apitypes.OrganizationRoleMember}); err != nil {
					t.Fatal(err)
				}
				detail := requireValue[*apitypes.Organization](t)(owner.Client.GetOrganization(ctx, organization.ID))
				if len(detail.Invitations) != 1 {
					t.Fatalf("organization invitations: %+v", detail.Invitations)
				}
				cancel = func() error {
					return owner.Client.CancelOrganizationInvitation(ctx, organization.ID, detail.Invitations[0].ID)
				}
			}
			token := findInviteToken(t, server, recorder, email)
			if response, _ := fetchInvitePage(t, baseURL, token); response.StatusCode != http.StatusOK {
				t.Fatalf("live delivered link: %d", response.StatusCode)
			}
			if err := cancel(); err != nil {
				t.Fatal(err)
			}
			if response, _ := fetchInvitePage(t, baseURL, token); response.StatusCode != http.StatusNotFound {
				t.Errorf("canceled delivered link is live: %d", response.StatusCode)
			}
			if _, err := signUpWithInvite(ctx, baseURL, token, email); !apiclient.IsStatus(err, http.StatusForbidden) {
				t.Errorf("canceled signup capability accepted: %v", err)
			}
			if _, err := signUpWithInvite(ctx, baseURL, independent.Token, email); err != nil {
				t.Fatalf("unlinked same-email same-issuer invitation was revoked: %v", err)
			}
		})
	}
}
