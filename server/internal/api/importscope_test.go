package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
)

func TestImportRequiresReviewedCurrentMembership(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	attachMailRecorder(server)
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleMember)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", apitypes.RoleMember)
	work := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Work", apitypes.VaultShared))
	scope := func() string {
		members := requireValue[[]apitypes.VaultMember](t)(alice.Client.ListVaultMembers(ctx, work.ID))
		reviewed := []crypt.ImportMember{}
		for _, member := range members {
			reviewed = append(reviewed, crypt.ImportMember{AccountID: member.AccountID, Role: member.Role})
		}
		return crypt.ImportScope(work.ID, reviewed)
	}
	reviewed := scope()
	first := &item.Item{Overview: item.Overview{Title: "Reviewed import"}, Details: item.Details{Password: "synthetic-first"}}
	if _, err := alice.SaveImportItem(ctx, work, first, reviewed); err != nil {
		t.Fatal(err)
	}
	shareThroughInvitation(t, alice, bob, work, apitypes.VaultReader)
	second := &item.Item{Overview: item.Overview{Title: "Unreviewed import"}, Details: item.Details{Password: "synthetic-second"}}
	if _, err := alice.SaveImportItem(ctx, work, second, reviewed); !errors.Is(err, session.ErrImportScopeChanged) {
		t.Fatalf("changed members did not stop import as a membership change: %v", err)
	}
	if _, exists := findTestItem(t, alice, work, second.ID); exists {
		t.Fatal("unreviewed item persisted")
	}
	if _, err := alice.SaveImportItem(ctx, work, second, scope()); err != nil {
		t.Fatal(err)
	}
	bobWork, _ := findTestVault(t, bob, work.ID)
	if saved, exists := findTestItem(t, bob, bobWork, second.ID); !exists || saved.Details.Password != "synthetic-second" {
		t.Fatalf("explicitly reviewed shared import not readable: %+v", saved)
	}
	request, err := http.NewRequest(http.MethodPut, baseURL+"/api/v1/vaults/"+work.ID+"/items/"+uuid.NewString()+"/import", bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+alice.Client.Token)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("scope-less import returned %d", response.StatusCode)
	}
}

func TestStandaloneVaultSelectionRejectsDuplicateNames(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleMember)
	first := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Work", apitypes.VaultShared))
	second := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "WORK", apitypes.VaultShared))
	if vault, err := alice.FindVault(ctx, "work"); err == nil || vault != nil || !strings.Contains(err.Error(), first.ID) || !strings.Contains(err.Error(), second.ID) || !strings.Contains(err.Error(), "shared") {
		t.Fatalf("ambiguous standalone selection: %+v %v", vault, err)
	}
	shadow := requireValue[*session.Vault](t)(alice.CreateVault(ctx, first.ID, apitypes.VaultShared))
	if vault, err := alice.FindVault(ctx, first.ID); err != nil || vault.ID != first.ID {
		t.Fatalf("name shadowed exact ID: %+v %v", vault, err)
	}
	if vault, err := alice.FindVault(ctx, shadow.ID); err != nil || vault.ID != shadow.ID {
		t.Fatalf("shadow ID not selectable: %+v %v", vault, err)
	}
}
