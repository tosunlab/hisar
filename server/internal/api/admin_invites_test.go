package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/session"
)

const adminInvitesPath = "/api/v1/admin/invites"

func createAdminInvite(t *testing.T, baseURL, token, email, role string) apitypes.AdminInviteCreated {
	t.Helper()
	status, body := sendRawRequest(t, baseURL, token, "POST", adminInvitesPath, apitypes.AdminInviteRequest{Email: email, Role: role})
	if status != http.StatusCreated {
		t.Fatalf("create %s invite for %s: %d %s", role, email, status, body)
	}
	return decodeJSON[apitypes.AdminInviteCreated](t, body)
}

func listAdminInvites(t *testing.T, baseURL, token string) []apitypes.AdminInvite {
	t.Helper()
	status, body := sendRawRequest(t, baseURL, token, "GET", adminInvitesPath, nil)
	if status != http.StatusOK {
		t.Fatalf("list invites: %d %s", status, body)
	}
	return decodeJSON[apitypes.AdminInviteList](t, body).Invites
}

func decodeJSON[T any](t *testing.T, body string) T {
	t.Helper()
	var value T
	if err := json.Unmarshal([]byte(body), &value); err != nil {
		t.Fatalf("%v: %s", err, body)
	}
	return value
}

func getInviteToken(t *testing.T, link string) string {
	t.Helper()
	match := inviteTokenPattern.FindStringSubmatch(link)
	if match == nil || !strings.HasPrefix(link, "https://hisar.test/invite/") {
		t.Fatalf("invite link %q", link)
	}
	return match[1]
}

func signUpWithInvite(ctx context.Context, baseURL, token, email string) (*session.Session, error) {
	signedUp, _, err := session.SignUp(ctx, apiclient.New(baseURL), token, email, email, []byte(email+" password"), testDevice)
	return signedUp, err
}

func countAuditEvents(t *testing.T, databaseURL, eventType string) int {
	t.Helper()
	return queryValue[int](t, databaseURL, `SELECT count(*) FROM audit_events WHERE type = $1`, eventType)
}

func TestAdminInvitesCreateWithEachRole(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	admin := signUpTestAccount(t, ctx, server, baseURL, "admin@example.com", apitypes.RoleAdmin)

	for _, role := range []string{apitypes.RoleMember, apitypes.RoleAdmin} {
		email := role + "-invitee@example.com"
		created := createAdminInvite(t, baseURL, admin.Client.Token, " "+strings.ToUpper(email)+" ", role)
		if created.IsEmailed || created.EmailError != "" || created.Invite.Email != email || created.Invite.Role != role ||
			created.Invite.CreatedBy == nil || created.Invite.CreatedBy.AccountID != admin.AccountID || created.Invite.CreatedBy.Email != "admin@example.com" {
			t.Errorf("created without email: %+v", created)
		}
		response, body := fetchInvitePage(t, baseURL, getInviteToken(t, created.Link))
		if response.StatusCode != http.StatusOK || !strings.Contains(body, email) {
			t.Errorf("invite page of %s: %d", email, response.StatusCode)
		}
		signedUp, err := signUpWithInvite(ctx, baseURL, getInviteToken(t, created.Link), email)
		if err != nil {
			t.Fatal(err)
		}
		account := requireValue[*apitypes.Account](t)(signedUp.Client.GetAccount(ctx))
		if account.Role != role {
			t.Errorf("%s signed up as %s", email, account.Role)
		}
	}
	if count := countAuditEvents(t, databaseURL, "admin_invite_created"); count != 2 {
		t.Errorf("%d admin_invite_created events", count)
	}

	for _, request := range []apitypes.AdminInviteRequest{{Email: "x@example.com", Role: "owner"}, {Email: "x@example.com"}, {Email: "not an email", Role: apitypes.RoleMember}} {
		if status, body := sendRawRequest(t, baseURL, admin.Client.Token, "POST", adminInvitesPath, request); status != http.StatusBadRequest {
			t.Errorf("%+v: %d %s", request, status, body)
		}
	}
}

func TestAdminInvitesRefuseAnExistingAccount(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	admin := signUpTestAccount(t, ctx, server, baseURL, "admin@example.com", apitypes.RoleAdmin)

	status, body := sendRawRequest(t, baseURL, admin.Client.Token, "POST", adminInvitesPath, apitypes.AdminInviteRequest{Email: "Admin@Example.com", Role: apitypes.RoleMember})
	if status != http.StatusConflict || !strings.Contains(body, "An account with this email already exists.") {
		t.Errorf("existing account: %d %s", status, body)
	}
	if invites := listAdminInvites(t, baseURL, admin.Client.Token); len(invites) != 0 {
		t.Errorf("invites after a refusal: %+v", invites)
	}
}

func TestAdminInvitesRevokeThenRedeemFails(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	admin := signUpTestAccount(t, ctx, server, baseURL, "admin@example.com", apitypes.RoleAdmin)
	created := createAdminInvite(t, baseURL, admin.Client.Token, "carol@example.com", apitypes.RoleMember)
	pendingBefore := getPendingInviteCount(t, baseURL, admin.Client.Token)

	revokePath := adminInvitesPath + "/" + created.Invite.ID
	if status, body := sendRawRequest(t, baseURL, admin.Client.Token, "DELETE", revokePath, nil); status != http.StatusNoContent {
		t.Fatalf("revoke: %d %s", status, body)
	}
	if invites := listAdminInvites(t, baseURL, admin.Client.Token); len(invites) != 0 {
		t.Errorf("invites after revoke: %+v", invites)
	}
	if pendingAfter := getPendingInviteCount(t, baseURL, admin.Client.Token); pendingAfter != pendingBefore-1 {
		t.Errorf("Overview's pending invites %d, then %d", pendingBefore, pendingAfter)
	}
	token := getInviteToken(t, created.Link)
	_, revokedPage := fetchInvitePage(t, baseURL, token)
	unknownResponse, unknownPage := fetchInvitePage(t, baseURL, "never-issued-token")
	if unknownResponse.StatusCode != http.StatusNotFound || revokedPage != unknownPage {
		t.Error("the revoked invite's page differs from an unknown code's")
	}
	if _, err := signUpWithInvite(ctx, baseURL, token, "carol@example.com"); err == nil {
		t.Fatal("signed up with a revoked invite")
	}
	for _, path := range []string{revokePath, adminInvitesPath + "/not-hex", adminInvitesPath + "/" + strings.Repeat("ab", 32)} {
		if status, body := sendRawRequest(t, baseURL, admin.Client.Token, "DELETE", path, nil); status != http.StatusNotFound || !strings.Contains(body, "This invite no longer exists.") {
			t.Errorf("revoke %s again: %d %s", path, status, body)
		}
	}
	if count := countAuditEvents(t, databaseURL, "admin_invite_revoked"); count != 1 {
		t.Errorf("%d admin_invite_revoked events", count)
	}
}

func getPendingInviteCount(t *testing.T, baseURL, token string) int64 {
	t.Helper()
	status, body := sendRawRequest(t, baseURL, token, "GET", "/api/v1/admin/health", nil)
	if status != http.StatusOK {
		t.Fatalf("health: %d %s", status, body)
	}
	return decodeJSON[apitypes.AdminHealth](t, body).Counts.PendingInvites
}

func TestAdminInvitesResendInvalidatesTheOldToken(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	admin := signUpTestAccount(t, ctx, server, baseURL, "admin@example.com", apitypes.RoleAdmin)
	operatorInvite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, "carol@example.com", apitypes.RoleAdmin, nil))
	listed := listAdminInvites(t, baseURL, admin.Client.Token)
	if len(listed) != 1 || listed[0].CreatedBy != nil || listed[0].Role != apitypes.RoleAdmin {
		t.Fatalf("operator invite: %+v", listed)
	}

	status, body := sendRawRequest(t, baseURL, admin.Client.Token, "POST", adminInvitesPath+"/"+listed[0].ID+"/resend", nil)
	if status != http.StatusCreated {
		t.Fatalf("resend: %d %s", status, body)
	}
	resent := decodeJSON[apitypes.AdminInviteCreated](t, body)
	if resent.Invite.ID == listed[0].ID || resent.Invite.Email != "carol@example.com" || resent.Invite.Role != apitypes.RoleAdmin ||
		resent.Invite.CreatedBy == nil || resent.Invite.CreatedBy.AccountID != admin.AccountID || !resent.Invite.ExpiresAt.After(listed[0].ExpiresAt) {
		t.Errorf("resent: %+v, was %+v", resent.Invite, listed[0])
	}
	if invites := listAdminInvites(t, baseURL, admin.Client.Token); len(invites) != 1 || invites[0].ID != resent.Invite.ID {
		t.Errorf("invites after resend: %+v", invites)
	}
	if response, _ := fetchInvitePage(t, baseURL, operatorInvite.Token); response.StatusCode != http.StatusNotFound {
		t.Errorf("old link: %d", response.StatusCode)
	}
	if _, err := signUpWithInvite(ctx, baseURL, operatorInvite.Token, "carol@example.com"); err == nil {
		t.Fatal("signed up with the old token")
	}
	if status, _ := sendRawRequest(t, baseURL, admin.Client.Token, "POST", adminInvitesPath+"/"+listed[0].ID+"/resend", nil); status != http.StatusNotFound {
		t.Errorf("resend the old invite again: %d", status)
	}
	carol, err := signUpWithInvite(ctx, baseURL, getInviteToken(t, resent.Link), "carol@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if account := requireValue[*apitypes.Account](t)(carol.Client.GetAccount(ctx)); account.Role != apitypes.RoleAdmin {
		t.Errorf("resent invite made a %s", account.Role)
	}
	if status, _ := sendRawRequest(t, baseURL, admin.Client.Token, "POST", adminInvitesPath+"/"+resent.Invite.ID+"/resend", nil); status != http.StatusNotFound {
		t.Errorf("resend a used invite: %d", status)
	}
	if count := countAuditEvents(t, databaseURL, "admin_invite_resent"); count != 1 {
		t.Errorf("%d admin_invite_resent events", count)
	}
	if count := countAuditEvents(t, databaseURL, "operator_invite"); count != 2 {
		t.Errorf("%d operator_invite events, want the admin's and carol's", count)
	}
}

func TestAdminInvitesListPendingInvitesNewestFirst(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	admin := signUpTestAccount(t, ctx, server, baseURL, "admin@example.com", apitypes.RoleAdmin)
	member := signUpTestAccount(t, ctx, server, baseURL, "member@example.com", apitypes.RoleMember)
	recorder := attachMailRecorder(server)
	if _, err := member.Client.Invite(ctx, &apitypes.InviteRequest{Email: "from-member@example.com"}); err != nil {
		t.Fatal(err)
	}
	findInviteToken(t, server, recorder, "from-member@example.com")
	createAdminInvite(t, baseURL, admin.Client.Token, "from-admin@example.com", apitypes.RoleMember)
	queryValue[int](t, databaseURL, `INSERT INTO invites (token_hash, email, role, expires_at) VALUES (decode('01', 'hex'), '', 'admin', now() + interval '1 hour') RETURNING 1`)
	server.service.InviteTTL = -time.Minute
	requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, "expired@example.com", apitypes.RoleMember, nil))

	invites := listAdminInvites(t, baseURL, admin.Client.Token)
	if len(invites) != 2 || invites[0].Email != "from-admin@example.com" || invites[1].Email != "from-member@example.com" {
		t.Fatalf("invites: %+v", invites)
	}
	if creator := invites[1].CreatedBy; creator == nil || creator.AccountID != member.AccountID || creator.Name != "member@example.com" {
		t.Errorf("member's invite sender: %+v", creator)
	}
}

func TestAdminInvitesEmailOnceWhenSMTPIsSet(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	admin := signUpTestAccount(t, ctx, server, baseURL, "admin@example.com", apitypes.RoleAdmin)
	recorder := attachMailRecorder(server)

	created := createAdminInvite(t, baseURL, admin.Client.Token, "carol@example.com", apitypes.RoleMember)
	messages := recorder.ListMessages()
	if !created.IsEmailed || created.EmailError != "" || len(messages) != 1 || messages[0].To != "carol@example.com" ||
		!strings.Contains(messages[0].Body, created.Link) || !strings.Contains(messages[0].Subject, "admin@example.com invited you to Hisar") {
		t.Fatalf("created %+v, messages %+v", created, messages)
	}

	recorder.SendError = errors.New("535 authentication failed")
	status, body := sendRawRequest(t, baseURL, admin.Client.Token, "POST", adminInvitesPath+"/"+created.Invite.ID+"/resend", nil)
	if status != http.StatusCreated {
		t.Fatalf("resend: %d %s", status, body)
	}
	resent := decodeJSON[apitypes.AdminInviteCreated](t, body)
	if resent.IsEmailed || resent.EmailError != "535 authentication failed" || resent.Link == "" || resent.Link == created.Link {
		t.Errorf("resent while sending fails: %+v", resent)
	}
	if count := len(recorder.ListMessages()); count != 1 {
		t.Errorf("%d emails", count)
	}
}

func TestAdminInvitesNeedAnAdmin(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	signUpTestAccount(t, ctx, server, baseURL, "admin@example.com", apitypes.RoleAdmin)
	member := signUpTestAccount(t, ctx, server, baseURL, "member@example.com", apitypes.RoleMember)
	before := queryValue[int](t, databaseURL, `SELECT count(*) FROM invites`)

	status, body := sendRawRequest(t, baseURL, member.Client.Token, "POST", adminInvitesPath, apitypes.AdminInviteRequest{Email: "carol@example.com", Role: apitypes.RoleAdmin})
	if status != http.StatusForbidden || !strings.Contains(body, "admin only") {
		t.Errorf("member: %d %s", status, body)
	}
	if after := queryValue[int](t, databaseURL, `SELECT count(*) FROM invites`); after != before {
		t.Errorf("invites %d, then %d", before, after)
	}
}
