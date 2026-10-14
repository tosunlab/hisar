package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/session"
)

func TestRecoveryEndpointsAreGone(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	adminInvite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, "admin@example.com", apitypes.RoleAdmin, nil))
	admin, _, err := session.SignUp(ctx, apiclient.Create(baseURL), adminInvite.Token, "admin@example.com", "Admin", []byte("admin password"), testDevice)
	if err != nil {
		t.Fatal(err)
	}
	vault := requireValue[*session.Vault](t)(admin.CreateVault(ctx, "Work", apitypes.VaultShared))

	allRemovedEndpoint := []struct{ method, path string }{
		{"GET", "/api/v1/recovery/keyset"},
		{"POST", "/api/v1/recovery/keyset"},
		{"PUT", "/api/v1/vaults/" + vault.ID + "/recovery"},
		{"DELETE", "/api/v1/vaults/" + vault.ID + "/recovery"},
		{"POST", "/api/v1/auth/recovery/account"},
		{"POST", "/api/v1/auth/recovery/start"},
		{"POST", "/api/v1/auth/recovery/finish"},
		{"GET", "/api/v1/admin/recovery"},
		{"POST", "/api/v1/admin/recovery/" + admin.AccountID},
		{"POST", "/api/v1/admin/recovery/" + admin.AccountID + "/complete"},
	}
	for _, endpoint := range allRemovedEndpoint {
		for _, token := range []string{admin.Client.Token, ""} {
			request, err := http.NewRequestWithContext(ctx, endpoint.method, baseURL+endpoint.path, strings.NewReader("{}"))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			if token != "" {
				request.Header.Set("Authorization", "Bearer "+token)
			}
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != http.StatusNotFound {
				t.Errorf("%s %s (signed in: %v) answered %d, want 404", endpoint.method, endpoint.path, token != "", response.StatusCode)
			}
		}
	}

	allVault := requireValue[[]apitypes.Vault](t)(admin.Client.ListAllVault(ctx))
	if len(allVault) != 2 {
		t.Fatalf("vaults: %+v", allVault)
	}
	if err := admin.Client.CreateVault(ctx, &apitypes.CreateVaultRequest{ID: uuid.NewString(), Type: apitypes.VaultShared, EncName: []byte{1}, WrappedKey: []byte{1}}); err != nil {
		t.Errorf("a shared vault without a recovery wrapping was refused: %v", err)
	}
}
