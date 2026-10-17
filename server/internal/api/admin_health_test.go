package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
)

func TestAdminHealth(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	signUp := func(email, role string) *session.Session {
		invite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, email, role, nil))
		signedUp, _, err := session.SignUp(ctx, apiclient.New(baseURL), invite.Token, email, email, []byte("fixture master password"), testDevice)
		if err != nil {
			t.Fatal(err)
		}
		return signedUp
	}
	admin := signUp("admin@example.com", apitypes.RoleAdmin)
	member := signUp("member@example.com", apitypes.RoleMember)
	getHealth := func(token, forwardedFor string) (int, apitypes.AdminHealth) {
		t.Helper()
		request := requireValue[*http.Request](t)(http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/admin/health", nil))
		request.Header.Set("Authorization", "Bearer "+token)
		if forwardedFor != "" {
			request.Header.Set("X-Forwarded-For", forwardedFor)
		}
		response := requireValue[*http.Response](t)(http.DefaultClient.Do(request))
		defer response.Body.Close()
		var health apitypes.AdminHealth
		if response.StatusCode == http.StatusOK {
			if err := json.NewDecoder(response.Body).Decode(&health); err != nil {
				t.Fatal(err)
			}
		}
		return response.StatusCode, health
	}

	status, before := getHealth(admin.Client.Token, "")
	if status != http.StatusOK {
		t.Fatalf("admin: %d", status)
	}
	work := requireValue[*session.Vault](t)(member.CreateVault(ctx, "Work", apitypes.VaultShared))
	for _, title := range []string{"GitHub", "AWS"} {
		requireValue[*item.Item](t)(member.SaveItem(ctx, work, &item.Item{Overview: item.Overview{Title: title}}))
	}
	requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, "carol@example.com", apitypes.RoleMember, nil))
	requireValue[*apitypes.ShareLink](t)(member.Client.CreateShareLink(ctx, &apitypes.CreateShareLinkRequest{Ciphertext: []byte{1}, EncryptedLabel: []byte{1}, ExpiresInSeconds: 3600, MaxViews: 1}))

	_, after := getHealth(admin.Client.Token, "")
	if before.Counts.Accounts != 2 || after.Counts.Accounts != 2 {
		t.Errorf("accounts %d then %d", before.Counts.Accounts, after.Counts.Accounts)
	}
	expected := apitypes.AdminHealthCounts{
		Accounts: 2, Vaults: before.Counts.Vaults + 1, Items: before.Counts.Items, ShareLinks: before.Counts.ShareLinks + 1, PendingInvites: before.Counts.PendingInvites + 1,
	}
	if after.Counts != expected {
		t.Errorf("counts %+v, expected %+v with the item count of a minute ago", after.Counts, expected)
	}
	if after.StorageBytes <= 0 || !after.Database.OK || len(after.Database.Migrations) == 0 {
		t.Errorf("database %+v, %d bytes", after.Database, after.StorageBytes)
	}
	for _, migration := range after.Database.Migrations {
		if migration.AppliedAt == nil {
			t.Errorf("migration %s pending", migration.Name)
		}
	}
	statusByCheck := map[string]string{}
	for _, check := range after.Checks {
		statusByCheck[check.ID] = check.Status
	}
	for checkID, expectedStatus := range map[string]string{
		"https": apitypes.CheckStatusOK, "proxy": apitypes.CheckStatusOK, "migrations": apitypes.CheckStatusOK,
		"secrets-backup": apitypes.CheckStatusWarning, "email": apitypes.CheckStatusWarning, "public-url": apitypes.CheckStatusWarning,
	} {
		if statusByCheck[checkID] != expectedStatus {
			t.Errorf("check %s: %q, expected %q", checkID, statusByCheck[checkID], expectedStatus)
		}
	}
	if _, proxied := getHealth(admin.Client.Token, "203.0.113.7"); proxied.Checks[2].ID != "proxy" || proxied.Checks[2].Status != apitypes.CheckStatusWarning {
		t.Errorf("proxied request without HISAR_TRUST_PROXY: %+v", proxied.Checks[2])
	}

	if status, _ := getHealth(member.Client.Token, ""); status != http.StatusForbidden {
		t.Errorf("member: %d", status)
	}
	if status, _ := getHealth("not a token", ""); status != http.StatusUnauthorized {
		t.Errorf("no session: %d", status)
	}
}
