package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/server/internal/service"
)

const adminAccountsPath = "/api/v1/admin/accounts"

func signUpAccountWithRole(t *testing.T, server *Server, baseURL, email, role string) *lifecycleAccount {
	t.Helper()
	invite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(context.Background(), email, role, nil))
	password := []byte(email + " password")
	signedUp, secretKey, err := session.SignUp(context.Background(), apiclient.New(baseURL), invite.Token, email, strings.Split(email, "@")[0], password, testDevice)
	if err != nil {
		t.Fatal(err)
	}
	return &lifecycleAccount{session: signedUp, password: password, secretKey: secretKey}
}

// proveAdmin returns a re-authentication proof from the admin's session.
func proveAdmin(t *testing.T, admin *lifecycleAccount) apitypes.ReauthenticationProof {
	t.Helper()
	proof, err := session.Reauthenticate(context.Background(), admin.session.Client, admin.session.Email, admin.password, admin.secretKey)
	if err != nil {
		t.Fatal(err)
	}
	return proof
}

func decodeAnswer[T any](t *testing.T, status int, body string) T {
	t.Helper()
	if status != http.StatusOK {
		t.Fatalf("answer: %d %s", status, body)
	}
	var value T
	if err := json.Unmarshal([]byte(body), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func readAdminAccountDetail(t *testing.T, baseURL, token, accountID string) apitypes.AdminAccountDetail {
	t.Helper()
	status, body := sendRawRequest(t, baseURL, token, "GET", adminAccountsPath+"/"+accountID, nil)
	return decodeAnswer[apitypes.AdminAccountDetail](t, status, body)
}

func countAdminAudit(t *testing.T, databaseURL, eventType, actorID, targetID string) int {
	t.Helper()
	return queryValue[int](t, databaseURL, `SELECT count(*)::int FROM audit_events WHERE type = $1 AND account_id = $2 AND detail->>'account' = $3`, eventType, actorID, targetID)
}

func waitForStreamEnd(t *testing.T, stream *eventStream, name string) {
	t.Helper()
	select {
	case <-stream.finished:
	case <-time.After(5 * time.Second):
		t.Errorf("%s's event stream stayed open", name)
	}
}

func TestAdminAccountsListAndDetail(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	admin := signUpAccountWithRole(t, server, baseURL, "admin@example.com", apitypes.RoleAdmin)
	member := signUpAccountWithRole(t, server, baseURL, "member@example.com", apitypes.RoleMember)
	signUpAccountWithRole(t, server, baseURL, "second@example.com", apitypes.RoleMember)
	token := admin.session.Client.Token

	if status, body := sendRawRequest(t, baseURL, member.session.Client.Token, "GET", adminAccountsPath, nil); status != http.StatusForbidden || !strings.Contains(body, "admin only") {
		t.Errorf("member: %d %s", status, body)
	}
	status, body := sendRawRequest(t, baseURL, token, "GET", adminAccountsPath, nil)
	page := decodeAnswer[apitypes.AdminAccountPage](t, status, body)
	emails := []string{}
	for _, account := range page.Accounts {
		emails = append(emails, account.Email)
	}
	if !slices.Equal(emails, []string{"admin@example.com", "member@example.com", "second@example.com"}) || page.NextCursor != "" {
		t.Errorf("accounts: %v %q", emails, page.NextCursor)
	}
	listed := page.Accounts[1]
	if listed.ID != member.session.AccountID || listed.Role != apitypes.RoleMember || listed.State != apitypes.AccountStateActive ||
		listed.DeviceCount != 1 || listed.VaultCount != 1 || listed.LastSeenAt == nil || listed.Name != "member" {
		t.Errorf("member as listed: %+v", listed)
	}
	if strings.Contains(body, "opaque") || strings.Contains(body, "keyset") || strings.Contains(body, "salt") {
		t.Errorf("the list carries secrets: %s", body)
	}

	status, body = sendRawRequest(t, baseURL, token, "GET", adminAccountsPath+"?query=SEC", nil)
	if found := decodeAnswer[apitypes.AdminAccountPage](t, status, body); len(found.Accounts) != 1 || found.Accounts[0].Email != "second@example.com" {
		t.Errorf("search: %s", body)
	}
	status, body = sendRawRequest(t, baseURL, token, "GET", adminAccountsPath+"?cursor=member%40example.com", nil)
	if after := decodeAnswer[apitypes.AdminAccountPage](t, status, body); len(after.Accounts) != 1 || after.Accounts[0].Email != "second@example.com" {
		t.Errorf("after the cursor: %s", body)
	}

	detail := readAdminAccountDetail(t, baseURL, token, member.session.AccountID)
	if detail.Account.Email != "member@example.com" || len(detail.Devices) != 1 || detail.Devices[0].Name != testDevice.Name || detail.Devices[0].RevokedAt != nil {
		t.Errorf("detail: %+v", detail)
	}
	for _, path := range []string{"/NOBODY", "/not-an-id"} {
		if status, _ := sendRawRequest(t, baseURL, token, "GET", adminAccountsPath+path, nil); status != http.StatusNotFound {
			t.Errorf("%s: %d", path, status)
		}
	}
}

func TestAdminAccountsSuspendAndReactivate(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	admin := signUpAccountWithRole(t, server, baseURL, "admin@example.com", apitypes.RoleAdmin)
	member := signUpAccountWithRole(t, server, baseURL, "member@example.com", apitypes.RoleMember)
	token, memberID := admin.session.Client.Token, member.session.AccountID

	memberEvents := openEventStream(t, member.session.Client)
	waitForSubscriber(t, server, memberID, 1)
	if status, body := sendRawRequest(t, baseURL, token, "POST", adminAccountsPath+"/"+memberID+"/suspend", nil); status != http.StatusNoContent {
		t.Fatalf("suspend: %d %s", status, body)
	}
	waitForStreamEnd(t, memberEvents, "the suspended member")
	if _, err := member.session.Client.ListVaults(ctx); !errors.Is(err, apiclient.ErrAccountSuspended) {
		t.Errorf("the suspended member's session: %v", err)
	}
	if status, body := sendRawRequest(t, baseURL, member.session.Client.Token, "GET", "/api/v1/vaults", nil); status != http.StatusForbidden ||
		body != `{"error":"This account is suspended. Contact the admin of the Hisar server."}` {
		t.Errorf("the suspended member's request: %d %s", status, body)
	}
	if err := member.session.Client.StreamEvents(ctx, func(apiclient.Event) {}); !errors.Is(err, apiclient.ErrAccountSuspended) {
		t.Errorf("the suspended member's event stream: %v", err)
	}
	if queryValue[int](t, databaseURL, `SELECT count(*)::int FROM devices WHERE account_id = $1 AND revoked_at IS NULL`, memberID) != 1 {
		t.Error("the suspension signed out the member's device")
	}

	if _, err := session.LogIn(ctx, apiclient.New(baseURL), "member@example.com", []byte("wrong password"), member.secretKey, testDevice); !errors.Is(err, session.ErrBadCredentials) {
		t.Errorf("a wrong password on a suspended account: %v", err)
	}
	_, err := session.LogIn(ctx, apiclient.New(baseURL), "member@example.com", member.password, member.secretKey, testDevice)
	if !errors.Is(err, session.ErrAccountSuspended) || err.Error() != "This account is suspended. Contact the admin of the Hisar server." {
		t.Errorf("the right password on a suspended account: %v", err)
	}
	if queryValue[int](t, databaseURL, `SELECT count(*)::int FROM devices WHERE account_id = $1`, memberID) != 1 {
		t.Error("the refused sign-in added a device")
	}

	if detail := readAdminAccountDetail(t, baseURL, token, memberID); detail.Account.State != apitypes.AccountStateSuspended {
		t.Errorf("state: %q", detail.Account.State)
	}
	status, body := sendRawRequest(t, baseURL, token, "GET", "/api/v1/admin/health", nil)
	if health := decodeAnswer[apitypes.AdminHealth](t, status, body); health.Counts.SuspendedAccounts != 1 || health.Counts.Accounts != 2 {
		t.Errorf("health counts: %+v", health.Counts)
	}

	if status, body := sendRawRequest(t, baseURL, token, "POST", adminAccountsPath+"/"+memberID+"/reactivate", nil); status != http.StatusNoContent {
		t.Fatalf("reactivate: %d %s", status, body)
	}
	if _, err := member.session.Client.ListVaults(ctx); err != nil {
		t.Errorf("the session from before the suspension after reactivation: %v", err)
	}
	if _, err := session.LogIn(ctx, apiclient.New(baseURL), "member@example.com", member.password, member.secretKey, testDevice); err != nil {
		t.Errorf("sign-in after reactivation: %v", err)
	}
	if detail := readAdminAccountDetail(t, baseURL, token, memberID); detail.Account.State != apitypes.AccountStateActive {
		t.Errorf("state after reactivation: %q", detail.Account.State)
	}
	for _, eventType := range []string{"admin_account_suspended", "admin_account_reactivated"} {
		if countAdminAudit(t, databaseURL, eventType, admin.session.AccountID, memberID) != 1 {
			t.Errorf("no %s audit event", eventType)
		}
	}
}

func TestAdminAccountsRefuseTheAdminsOwnAccountAndAMissingProof(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	admin := signUpAccountWithRole(t, server, baseURL, "admin@example.com", apitypes.RoleAdmin)
	member := signUpAccountWithRole(t, server, baseURL, "member@example.com", apitypes.RoleMember)
	token, adminPath := admin.session.Client.Token, adminAccountsPath+"/"+admin.session.AccountID

	for name, request := range map[string]struct {
		method, path string
		body         any
	}{
		"suspend": {"POST", adminPath + "/suspend", nil},
		"role":    {"PUT", adminPath + "/role", apitypes.AdminRoleChangeRequest{Role: apitypes.RoleMember, Proof: proveAdmin(t, admin)}},
		"delete":  {"POST", adminPath + "/delete", apitypes.AdminAccountDeleteRequest{Email: "admin@example.com", Proof: proveAdmin(t, admin)}},
	} {
		status, body := sendRawRequest(t, baseURL, token, request.method, request.path, request.body)
		if status != http.StatusConflict || body != `{"error":"Use Settings → Account in the Hisar app for your own account."}` {
			t.Errorf("%s of the admin's own account: %d %s", name, status, body)
		}
	}

	memberPath := adminAccountsPath + "/" + member.session.AccountID
	for name, body := range map[string]any{
		"no proof":         apitypes.AdminAccountDeleteRequest{Email: "member@example.com"},
		"an unknown proof": apitypes.AdminAccountDeleteRequest{Email: "member@example.com", Proof: apitypes.ReauthenticationProof{ReauthenticationID: "unknown", KE3: []byte{1}}},
	} {
		if status, answer := sendRawRequest(t, baseURL, token, "POST", memberPath+"/delete", body); status != http.StatusForbidden {
			t.Errorf("delete with %s: %d %s", name, status, answer)
		}
	}
	if status, answer := sendRawRequest(t, baseURL, token, "PUT", memberPath+"/role", apitypes.AdminRoleChangeRequest{Role: apitypes.RoleAdmin}); status != http.StatusForbidden {
		t.Errorf("role change without a proof: %d %s", status, answer)
	}
	reauthentication, notAProof := startReauthentication(t, admin.session.Client, []byte("wrong password"), admin.secretKey)
	wrongProof := apitypes.ReauthenticationProof{ReauthenticationID: reauthentication.ReauthenticationID, KE3: notAProof}
	if status, answer := sendRawRequest(t, baseURL, token, "POST", memberPath+"/delete", apitypes.AdminAccountDeleteRequest{Email: "member@example.com", Proof: wrongProof}); status != http.StatusForbidden {
		t.Errorf("delete with a wrong proof: %d %s", status, answer)
	}
	if status, answer := sendRawRequest(t, baseURL, token, "POST", memberPath+"/delete", apitypes.AdminAccountDeleteRequest{Email: "other@example.com", Proof: proveAdmin(t, admin)}); status != http.StatusBadRequest {
		t.Errorf("delete with another email: %d %s", status, answer)
	}
	if status, answer := sendRawRequest(t, baseURL, member.session.Client.Token, "POST", memberPath+"/delete", apitypes.AdminAccountDeleteRequest{Email: "member@example.com", Proof: proveAdmin(t, admin)}); status != http.StatusForbidden {
		t.Errorf("a member's delete: %d %s", status, answer)
	}
	if queryValue[int](t, databaseURL, `SELECT count(*)::int FROM accounts`) != 2 || queryValue[string](t, databaseURL, `SELECT role FROM accounts WHERE id = $1`, admin.session.AccountID) != apitypes.RoleAdmin {
		t.Error("a refused change went through")
	}
}

func TestAdminAccountsKeepAnAdmin(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	first := signUpAccountWithRole(t, server, baseURL, "first@example.com", apitypes.RoleAdmin)
	second := signUpAccountWithRole(t, server, baseURL, "second@example.com", apitypes.RoleAdmin)

	// The first admin starts a change, as the route would after requireAdmin, and the second demotes them meanwhile:
	// the change is checked against the accounts as they are by then, and would leave no admin.
	proof := proveAdmin(t, first)
	if status, body := sendRawRequest(t, baseURL, second.session.Client.Token, "PUT", adminAccountsPath+"/"+first.session.AccountID+"/role",
		apitypes.AdminRoleChangeRequest{Role: apitypes.RoleMember, Proof: proveAdmin(t, second)}); status != http.StatusNoContent {
		t.Fatalf("demote the first admin: %d %s", status, body)
	}
	tokenHash := sha256.Sum256([]byte(first.session.Client.Token))
	actor := service.Actor{AccountID: first.session.AccountID, DeviceID: first.session.DeviceID, TokenHash: tokenHash[:]}
	err := server.service.ChangeAccountRole(ctx, actor, second.session.AccountID, &apitypes.AdminRoleChangeRequest{Role: apitypes.RoleMember, Proof: proof})
	var serviceError *service.Error
	if !errors.As(err, &serviceError) || serviceError.Kind != service.KindConflict || serviceError.Message != "The Hisar server needs at least one admin." {
		t.Errorf("demoting the last admin: %v", err)
	}
	if queryValue[string](t, databaseURL, `SELECT role FROM accounts WHERE id = $1`, second.session.AccountID) != apitypes.RoleAdmin {
		t.Error("the last admin was demoted")
	}
	if status, _ := sendRawRequest(t, baseURL, first.session.Client.Token, "GET", "/api/v1/admin/server", nil); status != http.StatusForbidden {
		t.Errorf("the demoted admin's next request: %d", status)
	}
	if countAdminAudit(t, databaseURL, "admin_role_changed", second.session.AccountID, first.session.AccountID) != 1 {
		t.Error("no admin_role_changed audit event")
	}
}

func TestAdminAccountsPromoteAMember(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	admin := signUpAccountWithRole(t, server, baseURL, "admin@example.com", apitypes.RoleAdmin)
	member := signUpAccountWithRole(t, server, baseURL, "member@example.com", apitypes.RoleMember)
	if status, _ := sendRawRequest(t, baseURL, member.session.Client.Token, "GET", "/api/v1/admin/server", nil); status != http.StatusForbidden {
		t.Fatalf("member before the promotion: %d", status)
	}
	rolePath := adminAccountsPath + "/" + member.session.AccountID + "/role"
	if status, body := sendRawRequest(t, baseURL, admin.session.Client.Token, "PUT", rolePath, apitypes.AdminRoleChangeRequest{Role: "owner", Proof: proveAdmin(t, admin)}); status != http.StatusBadRequest {
		t.Errorf("an unknown role: %d %s", status, body)
	}
	proof := proveAdmin(t, admin)
	if status, body := sendRawRequest(t, baseURL, admin.session.Client.Token, "PUT", rolePath, apitypes.AdminRoleChangeRequest{Role: apitypes.RoleAdmin, Proof: proof}); status != http.StatusNoContent {
		t.Fatalf("promote: %d %s", status, body)
	}
	if status, body := sendRawRequest(t, baseURL, admin.session.Client.Token, "PUT", rolePath, apitypes.AdminRoleChangeRequest{Role: apitypes.RoleAdmin, Proof: proof}); status != http.StatusForbidden {
		t.Errorf("a proof used twice: %d %s", status, body)
	}
	if status, _ := sendRawRequest(t, baseURL, member.session.Client.Token, "GET", "/api/v1/admin/server", nil); status != http.StatusOK {
		t.Errorf("the promoted member: %d", status)
	}
}

func TestAdminAccountsDelete(t *testing.T) {
	baseURL, server, databaseURL, recorder := setupLifecycleServer(t)
	ctx := context.Background()
	admin := signUpAccountWithRole(t, server, baseURL, "admin@example.com", apitypes.RoleAdmin)
	member := signUpAccountWithRole(t, server, baseURL, "member@example.com", apitypes.RoleMember)
	memberID := member.session.AccountID

	shared := requireValue[*session.Vault](t)(member.session.CreateVault(ctx, "Shared", apitypes.VaultShared))
	requireValue[*item.Item](t)(member.session.SaveItem(ctx, shared, &item.Item{Overview: item.Overview{Title: "Router"}, Details: item.Details{Password: "router-pass"}}))
	shareThroughInvitation(t, member.session, admin.session, shared, apitypes.VaultOwner)
	memberPersonal := requireValue[*session.Vault](t)(member.session.FindVault(ctx, "Personal"))

	memberEvents := openEventStream(t, member.session.Client)
	adminEvents := openEventStream(t, admin.session.Client)
	waitForSubscriber(t, server, memberID, 1)
	waitForSubscriber(t, server, admin.session.AccountID, 1)
	request := apitypes.AdminAccountDeleteRequest{Email: " Member@Example.com ", Proof: proveAdmin(t, admin)}
	if status, body := sendRawRequest(t, baseURL, admin.session.Client.Token, "POST", adminAccountsPath+"/"+memberID+"/delete", request); status != http.StatusNoContent {
		t.Fatalf("delete: %d %s", status, body)
	}
	waitForStreamEnd(t, memberEvents, "the deleted member")
	adminEvents.waitForEvent(t, apitypes.EventVaultsChanged)

	for name, query := range map[string]string{
		"account":        `SELECT count(*)::int FROM accounts WHERE id = $1`,
		"devices":        `SELECT count(*)::int FROM devices WHERE account_id = $1`,
		"vault access":   `SELECT count(*)::int FROM vault_access WHERE account_id = $1`,
		"personal vault": `SELECT count(*)::int FROM vaults WHERE id = '` + memberPersonal.ID + `' AND $1 <> ''`,
	} {
		if count := queryValue[int](t, databaseURL, query, memberID); count != 0 {
			t.Errorf("%s left: %d", name, count)
		}
	}
	if titles := readItemTitles(t, admin.session, "Shared"); !slices.Equal(titles, []string{"Router=router-pass"}) {
		t.Errorf("the vault the admin co-owns: %v", titles)
	}
	status, body := sendRawRequest(t, baseURL, admin.session.Client.Token, "GET", adminAccountsPath, nil)
	if page := decodeAnswer[apitypes.AdminAccountPage](t, status, body); len(page.Accounts) != 1 {
		t.Errorf("accounts after the deletion: %+v", page.Accounts)
	}
	if countAdminAudit(t, databaseURL, "admin_account_deleted", admin.session.AccountID, memberID) != 1 {
		t.Error("no admin_account_deleted audit event")
	}
	if email := findLatestEmail(t, server, recorder, "member@example.com", "Your Hisar account was deleted"); !strings.Contains(email, "deleted by an admin of the Hisar server") {
		t.Errorf("email: %s", email)
	}
	if _, err := session.LogIn(ctx, apiclient.New(baseURL), "member@example.com", member.password, member.secretKey, testDevice); !errors.Is(err, session.ErrBadCredentials) {
		t.Errorf("the deleted member signs in: %v", err)
	}
}

func TestAdminAccountsSignOutAndRevokeADevice(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	admin := signUpAccountWithRole(t, server, baseURL, "admin@example.com", apitypes.RoleAdmin)
	member := signUpAccountWithRole(t, server, baseURL, "member@example.com", apitypes.RoleMember)
	memberID, token := member.session.AccountID, admin.session.Client.Token
	laptop := requireValue[*session.Session](t)(session.LogIn(ctx, apiclient.New(baseURL), "member@example.com", member.password, member.secretKey, testDevice))

	laptopEvents := openEventStream(t, laptop.Client)
	desktopEvents := openEventStream(t, member.session.Client)
	waitForSubscriber(t, server, memberID, 2)
	if status, body := sendRawRequest(t, baseURL, token, "DELETE", adminAccountsPath+"/"+memberID+"/devices/"+laptop.DeviceID, nil); status != http.StatusNoContent {
		t.Fatalf("revoke: %d %s", status, body)
	}
	waitForStreamEnd(t, laptopEvents, "the revoked laptop")
	if _, err := laptop.Client.ListVaults(ctx); !apiclient.IsStatus(err, http.StatusUnauthorized) {
		t.Errorf("the revoked laptop: %v", err)
	}
	if _, err := member.session.Client.ListVaults(ctx); err != nil {
		t.Errorf("the other device after one revoke: %v", err)
	}
	if status, _ := sendRawRequest(t, baseURL, token, "DELETE", adminAccountsPath+"/"+memberID+"/devices/"+laptop.DeviceID, nil); status != http.StatusNotFound {
		t.Errorf("revoking again: %d", status)
	}
	if status, _ := sendRawRequest(t, baseURL, token, "DELETE", adminAccountsPath+"/"+admin.session.AccountID+"/devices/"+member.session.DeviceID, nil); status != http.StatusNotFound {
		t.Errorf("another account's device: %d", status)
	}

	if status, body := sendRawRequest(t, baseURL, token, "POST", adminAccountsPath+"/"+memberID+"/sign-out", nil); status != http.StatusNoContent {
		t.Fatalf("sign out: %d %s", status, body)
	}
	waitForStreamEnd(t, desktopEvents, "the member's desktop")
	if count := queryValue[int](t, databaseURL, `SELECT count(*)::int FROM devices WHERE account_id = $1 AND revoked_at IS NULL`, memberID); count != 0 {
		t.Errorf("devices still signed in: %d", count)
	}
	detail := readAdminAccountDetail(t, baseURL, token, memberID)
	if len(detail.Devices) != 2 || detail.Devices[0].RevokedAt == nil || detail.Devices[1].RevokedAt == nil || detail.Account.DeviceCount != 0 {
		t.Errorf("detail after sign out: %+v", detail)
	}
	if countAdminAudit(t, databaseURL, "admin_signed_out", admin.session.AccountID, memberID) != 1 ||
		countAdminAudit(t, databaseURL, "admin_device_revoked", admin.session.AccountID, memberID) != 1 {
		t.Error("missing admin_signed_out or admin_device_revoked audit event")
	}
}

func TestRevokingOwnDeviceEndsItsStream(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	member := signUpAccountWithRole(t, server, baseURL, "member@example.com", apitypes.RoleMember)
	laptop := requireValue[*session.Session](t)(session.LogIn(ctx, apiclient.New(baseURL), "member@example.com", member.password, member.secretKey, testDevice))
	laptopEvents := openEventStream(t, laptop.Client)
	waitForSubscriber(t, server, member.session.AccountID, 1)
	if err := member.session.Client.RevokeDevice(ctx, laptop.DeviceID); err != nil {
		t.Fatal(err)
	}
	waitForStreamEnd(t, laptopEvents, "the revoked laptop")
}

func TestAdminAccountsSearchRefusesInvalidText(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	admin := signUpAccountWithRole(t, server, baseURL, "admin@example.com", apitypes.RoleAdmin)
	for _, parameters := range []string{"query=a%00b", "query=%FF", "cursor=a%00b", "cursor=%FF"} {
		if status, body := sendRawRequest(t, baseURL, admin.session.Client.Token, "GET", adminAccountsPath+"?"+parameters, nil); status != http.StatusBadRequest {
			t.Errorf("%s: %d %s", parameters, status, body)
		}
	}
}

func TestAdminAccountsRepeatedSuspendOrReactivateChangesNothing(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	admin := signUpAccountWithRole(t, server, baseURL, "admin@example.com", apitypes.RoleAdmin)
	member := signUpAccountWithRole(t, server, baseURL, "member@example.com", apitypes.RoleMember)
	token, memberID := admin.session.Client.Token, member.session.AccountID

	for _, action := range []string{"reactivate", "suspend", "suspend", "reactivate", "reactivate"} {
		if status, body := sendRawRequest(t, baseURL, token, "POST", adminAccountsPath+"/"+memberID+"/"+action, nil); status != http.StatusNoContent {
			t.Fatalf("%s: %d %s", action, status, body)
		}
	}
	for _, eventType := range []string{"admin_account_suspended", "admin_account_reactivated"} {
		if count := countAdminAudit(t, databaseURL, eventType, admin.session.AccountID, memberID); count != 1 {
			t.Errorf("%s audit events: %d", eventType, count)
		}
	}
}

func TestAdminAccountsRefuseAnAdminDemotedMeanwhile(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	first := signUpAccountWithRole(t, server, baseURL, "first@example.com", apitypes.RoleAdmin)
	second := signUpAccountWithRole(t, server, baseURL, "second@example.com", apitypes.RoleAdmin)
	member := signUpAccountWithRole(t, server, baseURL, "member@example.com", apitypes.RoleMember)

	// The first admin starts each change, as the route would after requireAdmin, and the second demotes them meanwhile.
	roleProof, deleteProof := proveAdmin(t, first), proveAdmin(t, first)
	if status, body := sendRawRequest(t, baseURL, second.session.Client.Token, "PUT", adminAccountsPath+"/"+first.session.AccountID+"/role",
		apitypes.AdminRoleChangeRequest{Role: apitypes.RoleMember, Proof: proveAdmin(t, second)}); status != http.StatusNoContent {
		t.Fatalf("demote the first admin: %d %s", status, body)
	}
	tokenHash := sha256.Sum256([]byte(first.session.Client.Token))
	actor := service.Actor{AccountID: first.session.AccountID, DeviceID: first.session.DeviceID, TokenHash: tokenHash[:]}
	memberID := member.session.AccountID
	for name, change := range map[string]func() error{
		"role change": func() error {
			return server.service.ChangeAccountRole(ctx, actor, memberID, &apitypes.AdminRoleChangeRequest{Role: apitypes.RoleAdmin, Proof: roleProof})
		},
		"suspend":    func() error { return server.service.SuspendAccount(ctx, actor, memberID) },
		"reactivate": func() error { return server.service.ReactivateAccount(ctx, actor, memberID) },
		"sign-out":   func() error { return server.service.SignOutAccount(ctx, actor, second.session.AccountID) },
		"device sign-out": func() error {
			return server.service.RevokeAccountDevice(ctx, actor, second.session.AccountID, second.session.DeviceID)
		},
		"delete": func() error {
			return server.service.DeleteAccountAsAdmin(ctx, actor, memberID, &apitypes.AdminAccountDeleteRequest{Email: "member@example.com", Proof: deleteProof})
		},
	} {
		if err := change(); !errors.Is(err, service.ErrAdminOnly) {
			t.Errorf("%s by a demoted admin: %v", name, err)
		}
	}
	if queryValue[int](t, databaseURL, `SELECT count(*)::int FROM accounts WHERE id = $1 AND role = 'member' AND suspended_at IS NULL`, memberID) != 1 {
		t.Error("a demoted admin changed the member")
	}
	if queryValue[int](t, databaseURL, `SELECT count(*)::int FROM devices WHERE account_id = $1 AND revoked_at IS NULL`, second.session.AccountID) != 1 {
		t.Error("a demoted admin signed out the other admin")
	}
}

func TestAdminAccountsRefuseAProofWhoseSessionEnded(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	admin := signUpAccountWithRole(t, server, baseURL, "admin@example.com", apitypes.RoleAdmin)
	member := signUpAccountWithRole(t, server, baseURL, "member@example.com", apitypes.RoleMember)

	// The admin proves their master password, then their session ends before the change is saved.
	roleProof, deleteProof := proveAdmin(t, admin), proveAdmin(t, admin)
	tokenHash := sha256.Sum256([]byte(admin.session.Client.Token))
	actor := service.Actor{AccountID: admin.session.AccountID, DeviceID: admin.session.DeviceID, TokenHash: tokenHash[:]}
	if err := admin.session.Client.Logout(ctx); err != nil {
		t.Fatal(err)
	}
	memberID := member.session.AccountID
	if err := server.service.ChangeAccountRole(ctx, actor, memberID, &apitypes.AdminRoleChangeRequest{Role: apitypes.RoleAdmin, Proof: roleProof}); !errors.Is(err, service.ErrUnauthorized) {
		t.Errorf("role change after the session ended: %v", err)
	}
	if err := server.service.DeleteAccountAsAdmin(ctx, actor, memberID, &apitypes.AdminAccountDeleteRequest{Email: "member@example.com", Proof: deleteProof}); !errors.Is(err, service.ErrUnauthorized) {
		t.Errorf("deletion after the session ended: %v", err)
	}
	if queryValue[int](t, databaseURL, `SELECT count(*)::int FROM accounts WHERE id = $1 AND role = 'member'`, memberID) != 1 {
		t.Error("a change with a proof from an ended session went through")
	}
}
