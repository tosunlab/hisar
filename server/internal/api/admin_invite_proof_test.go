package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/session"
)

func TestAdminInviteIssuanceRequiresFreshSessionBoundProof(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	admin := signUpAccountWithRole(t, server, baseURL, "admin@example.com", apitypes.RoleAdmin)
	client := admin.session.Client
	request := &apitypes.AdminInviteRequest{Email: "new-admin@example.com", Role: apitypes.RoleAdmin}
	if _, err := client.CreateAdminInvite(ctx, request); !apiclient.IsStatus(err, http.StatusForbidden) {
		t.Fatalf("bearer-only admin invite: %v", err)
	}
	request.Proof = proveAdmin(t, admin)
	created, err := client.CreateAdminInvite(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if created.Link == "" || created.IsEmailed {
		t.Fatalf("SMTP-free admin invitation unavailable: %+v", created)
	}
	request.Email = "replayed@example.com"
	if _, err := client.CreateAdminInvite(ctx, request); !apiclient.IsStatus(err, http.StatusForbidden) {
		t.Errorf("replayed creation proof: %v", err)
	}
	if _, err := client.ResendAdminInvite(ctx, created.Invite.ID, &apitypes.AdminInviteResendRequest{}); !apiclient.IsStatus(err, http.StatusForbidden) {
		t.Errorf("bearer-only admin resend: %v", err)
	}
	if _, err := client.ResendAdminInvite(ctx, created.Invite.ID, &apitypes.AdminInviteResendRequest{Proof: request.Proof}); !apiclient.IsStatus(err, http.StatusForbidden) {
		t.Errorf("creation proof reused for resend: %v", err)
	}

	otherSession, err := session.LogIn(ctx, apiclient.New(baseURL), admin.session.Email, admin.password, admin.secretKey, testDevice)
	if err != nil {
		t.Fatal(err)
	}
	request.Proof = proveAdmin(t, admin)
	if _, err := otherSession.Client.CreateAdminInvite(ctx, request); !apiclient.IsStatus(err, http.StatusForbidden) {
		t.Errorf("proof from another session: %v", err)
	}
	request.Proof = proveAdmin(t, admin)
	request.Proof.KE3 = []byte("invalid")
	if _, err := client.CreateAdminInvite(ctx, request); !apiclient.IsStatus(err, http.StatusForbidden) {
		t.Errorf("invalid OPAQUE proof: %v", err)
	}

	resendProof := proveAdmin(t, admin)
	resent, err := client.ResendAdminInvite(ctx, created.Invite.ID, &apitypes.AdminInviteResendRequest{Proof: resendProof})
	if err != nil {
		t.Fatal(err)
	}
	if response, _ := fetchInvitePage(t, baseURL, getInviteToken(t, baseURL, created.Link)); response.StatusCode != http.StatusNotFound {
		t.Errorf("replaced admin link is live: %d", response.StatusCode)
	}
	request.Email = "unrelated@example.com"
	request.Proof = resendProof
	if _, err := client.CreateAdminInvite(ctx, request); !apiclient.IsStatus(err, http.StatusForbidden) {
		t.Errorf("resend proof reused for creation: %v", err)
	}
	if _, err := signUpWithInvite(ctx, baseURL, getInviteToken(t, baseURL, resent.Link), resent.Invite.Email); err != nil {
		t.Fatalf("proven SMTP-free admin signup: %v", err)
	}

	request.Role = apitypes.RoleMember
	request.Proof = apitypes.ReauthenticationProof{}
	member, err := client.CreateAdminInvite(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ResendAdminInvite(ctx, member.Invite.ID, &apitypes.AdminInviteResendRequest{}); err != nil {
		t.Errorf("member resend now requires factors: %v", err)
	}
}
