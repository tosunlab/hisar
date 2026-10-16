package apiclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/apitypes"
)

func TestOrganizationRequests(t *testing.T) {
	var requests []string
	fakeClient := startFakeServer(t, func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		requests = append(requests, strings.TrimSpace(request.Method+" "+request.URL.Path+" "+string(body)))
		switch request.Method + " " + request.URL.Path {
		case "POST /api/v1/organizations":
			writer.WriteHeader(http.StatusCreated)
			json.NewEncoder(writer).Encode(apitypes.OrganizationSummary{ID: "org-1", Name: "Acme", Role: apitypes.OrganizationRoleAdmin, MemberCount: 1})
		case "POST /api/v1/organizations/org-1/teams":
			writer.WriteHeader(http.StatusCreated)
			json.NewEncoder(writer).Encode(apitypes.Team{ID: "team-1", Name: "Backend"})
		case "POST /api/v1/organizations/org-1/invitations":
			writer.WriteHeader(http.StatusAccepted)
			json.NewEncoder(writer).Encode(apitypes.InvitationSentResponse{Message: apitypes.InvitationSentMessage})
		case "GET /api/v1/vault-key-requests":
			json.NewEncoder(writer).Encode(apitypes.VaultKeyRequestList{Pending: []apitypes.VaultKeyRequest{{VaultID: "vault-1", KeyGeneration: 2}}})
		case "GET /api/v1/organizations", "GET /api/v1/organizations/org-1":
			writer.Write([]byte(`{"id":"org-1","organizations":[{"id":"org-1"}]}`))
		default:
			writer.WriteHeader(http.StatusNoContent)
		}
	})
	ctx := context.Background()
	if list, err := fakeClient.ListOrganizations(ctx); err != nil || len(list.Organizations) != 1 {
		t.Fatalf("list: %+v %v", list, err)
	}
	if created, err := fakeClient.CreateOrganization(ctx, &apitypes.CreateOrganizationRequest{Name: "Acme"}); err != nil || created.ID != "org-1" || created.MemberCount != 1 {
		t.Fatalf("create: %+v %v", created, err)
	}
	if organization, err := fakeClient.GetOrganization(ctx, "org-1"); err != nil || organization.ID != "org-1" {
		t.Fatalf("get: %+v %v", organization, err)
	}
	if response, err := fakeClient.InviteToOrganization(ctx, "org-1", &apitypes.OrganizationInvitationRequest{Email: "bob@example.com", Role: apitypes.OrganizationRoleMember}); err != nil || response.Message != apitypes.InvitationSentMessage {
		t.Fatalf("invite: %+v %v", response, err)
	}
	if team, err := fakeClient.CreateTeam(ctx, "org-1", &apitypes.CreateTeamRequest{Name: "Backend"}); err != nil || team.ID != "team-1" {
		t.Fatalf("create team: %+v %v", team, err)
	}
	if keyRequests, err := fakeClient.ListVaultKeyRequests(ctx); err != nil || len(keyRequests.Pending) != 1 || keyRequests.Pending[0].KeyGeneration != 2 {
		t.Fatalf("key requests: %+v %v", keyRequests, err)
	}
	for _, send := range []func() error{
		func() error {
			return fakeClient.RenameOrganization(ctx, "org-1", &apitypes.RenameOrganizationRequest{Name: "Acme Inc"})
		},
		func() error { return fakeClient.DeleteOrganization(ctx, "org-1") },
		func() error { return fakeClient.CancelOrganizationInvitation(ctx, "org-1", "invitation-1") },
		func() error { return fakeClient.AcceptOrganizationInvitation(ctx, "invitation-1") },
		func() error { return fakeClient.DeclineOrganizationInvitation(ctx, "invitation-1") },
		func() error {
			return fakeClient.SetOrganizationMemberRole(ctx, "org-1", "account-1", &apitypes.SetOrganizationRoleRequest{Role: apitypes.OrganizationRoleAdmin})
		},
		func() error { return fakeClient.RemoveOrganizationMember(ctx, "org-1", "account-1") },
		func() error {
			return fakeClient.RenameTeam(ctx, "org-1", "team-1", &apitypes.RenameTeamRequest{Name: "Frontend"})
		},
		func() error { return fakeClient.DeleteTeam(ctx, "org-1", "team-1") },
		func() error { return fakeClient.AddTeamMember(ctx, "org-1", "team-1", "account-1") },
		func() error { return fakeClient.RemoveTeamMember(ctx, "org-1", "team-1", "account-1") },
		func() error {
			return fakeClient.GiveVaultToTeam(ctx, "org-1", "team-1", "vault-1", &apitypes.GrantTeamVaultRequest{Role: apitypes.VaultEditor})
		},
		func() error { return fakeClient.TakeVaultFromTeam(ctx, "org-1", "team-1", "vault-1") },
	} {
		if err := send(); err != nil {
			t.Fatal(err)
		}
	}
	expected := []string{
		"GET /api/v1/organizations",
		`POST /api/v1/organizations {"name":"Acme"}`,
		"GET /api/v1/organizations/org-1",
		`POST /api/v1/organizations/org-1/invitations {"email":"bob@example.com","role":"member"}`,
		`POST /api/v1/organizations/org-1/teams {"name":"Backend"}`,
		"GET /api/v1/vault-key-requests",
		`PUT /api/v1/organizations/org-1/name {"name":"Acme Inc"}`,
		"DELETE /api/v1/organizations/org-1",
		"DELETE /api/v1/organizations/org-1/invitations/invitation-1",
		"POST /api/v1/organization-invitations/invitation-1/accept",
		"POST /api/v1/organization-invitations/invitation-1/decline",
		`PUT /api/v1/organizations/org-1/members/account-1 {"role":"admin"}`,
		"DELETE /api/v1/organizations/org-1/members/account-1",
		`PUT /api/v1/organizations/org-1/teams/team-1/name {"name":"Frontend"}`,
		"DELETE /api/v1/organizations/org-1/teams/team-1",
		"PUT /api/v1/organizations/org-1/teams/team-1/members/account-1",
		"DELETE /api/v1/organizations/org-1/teams/team-1/members/account-1",
		`PUT /api/v1/organizations/org-1/teams/team-1/vaults/vault-1 {"role":"editor"}`,
		"DELETE /api/v1/organizations/org-1/teams/team-1/vaults/vault-1",
	}
	if strings.Join(requests, "\n") != strings.Join(expected, "\n") {
		t.Errorf("requests:\n%s", strings.Join(requests, "\n"))
	}
}
