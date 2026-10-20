package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
)

type collectionMutationTransport struct {
	base      http.RoundTripper
	once      sync.Once
	mutate    func()
	pages     int
	conflicts int
}

func (transport *collectionMutationTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := transport.base.RoundTrip(request)
	if err == nil && request.Method == "GET" && request.URL.Path == "/api/v1/vaults" {
		transport.pages++
		if response.StatusCode == http.StatusConflict {
			transport.conflicts++
		}
		if request.URL.Query().Get("cursor") == "" {
			transport.once.Do(transport.mutate)
		}
	}
	return response, err
}

func TestPagedHistoryAndObjectTraversal(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	invite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, "pages@example.com", apitypes.RoleAdmin, nil))
	alice, _, err := session.SignUp(ctx, apiclient.New(baseURL), invite.Token, "pages@example.com", "Pages", []byte("fixture password"), testDevice)
	if err != nil {
		t.Fatal(err)
	}
	defer alice.Keyset.Wipe()
	vaults := requireValue[[]session.Vault](t)(alice.ListVaults(ctx))
	personal := &vaults[0]
	note := &item.Item{Overview: item.Overview{Title: "Public pagination fixture", Category: item.CategorySecureNote}, Details: item.Details{Notes: strings.Repeat("public fixture ", 35000)}}
	for index := range 20 {
		note.Overview.Title = fmt.Sprintf("Public revision %02d", index)
		if _, err := alice.SaveItem(ctx, personal, note); err != nil {
			t.Fatal(err)
		}
	}
	history := requireValue[[]apitypes.Item](t)(alice.Client.GetItemHistory(ctx, personal.ID, note.ID))
	if len(history) != 20 {
		t.Fatalf("history truncated to %d", len(history))
	}
	for _, revision := range history {
		plaintext := requireValue[[]byte](t)(crypt.OpenItemPart(personal.Key, personal.ID, note.ID, revision.Revision, crypt.Details, revision.Details))
		var details item.Details
		if err := json.Unmarshal(plaintext, &details); err != nil || details.Notes != note.Details.Notes {
			t.Fatalf("history unreadable: %v", err)
		}
		clear(plaintext)
	}
	path := "/api/v1/vaults/" + personal.ID + "/items/" + note.ID + "/history"
	status, body := sendRawRequest(t, baseURL, alice.Client.Token, "GET", path, nil)
	if status != http.StatusPreconditionRequired || !strings.Contains(body, "pagination") {
		t.Fatalf("old peer got %d %s", status, body)
	}
	request := requireValue[*http.Request](t)(http.NewRequestWithContext(ctx, "GET", baseURL+path, nil))
	request.Header.Set("Authorization", "Bearer "+alice.Client.Token)
	request.Header.Set(apitypes.CollectionPageHeader, apitypes.CollectionPageVersion)
	response := requireValue[*http.Response](t)(http.DefaultClient.Do(request))
	data := requireValue[[]byte](t)(io.ReadAll(response.Body))
	response.Body.Close()
	var firstPage apitypes.CollectionPage
	if err := json.Unmarshal(data, &firstPage); err != nil || len(data) > apitypes.CollectionPageBytes || firstPage.NextCursor == "" {
		t.Fatalf("byte page %d bytes: %v", len(data), err)
	}
	t.Logf("real encrypted history smoke revisions=%d firstPageBytes=%d firstPageRows=%d", len(history), len(data), len(firstPage.Records))
	for index := range 120 {
		if _, err := alice.CreateVault(ctx, fmt.Sprintf("Paged vault %03d", index), apitypes.VaultShared); err != nil {
			t.Fatal(err)
		}
	}
	transport := &collectionMutationTransport{base: http.DefaultTransport, mutate: func() {
		if _, err := alice.CreateVault(ctx, "Concurrent vault", apitypes.VaultShared); err != nil {
			t.Error(err)
		}
	}}
	alice.Client.HTTP = &http.Client{Transport: transport}
	inventory := requireValue[[]session.Vault](t)(alice.ListVaults(ctx))
	if len(inventory) != 122 || transport.conflicts != 1 || transport.pages < 4 {
		t.Fatalf("concurrent inventory count=%d pages=%d conflicts=%d", len(inventory), transport.pages, transport.conflicts)
	}
	seen := map[string]bool{}
	for _, vault := range inventory {
		if seen[vault.ID] {
			t.Fatal("duplicate vault")
		}
		seen[vault.ID] = true
	}
	alice.Client.HTTP = http.DefaultClient
	organization := requireValue[*apitypes.OrganizationSummary](t)(alice.Client.CreateOrganization(ctx, &apitypes.CreateOrganizationRequest{Name: "Paged organization"}))
	var organizationVault *session.Vault
	for index := range 120 {
		team := requireValue[*apitypes.Team](t)(alice.Client.CreateTeam(ctx, organization.ID, &apitypes.CreateTeamRequest{Name: fmt.Sprintf("Team %03d", index)}))
		if err := alice.Client.AddTeamMember(ctx, organization.ID, team.ID, alice.AccountID); err != nil {
			t.Fatal(err)
		}
		if organizationVault == nil {
			organizationVault = createOrganizationVault(t, alice, organization.ID, team.ID, "Paged organization vault")
		} else if err := alice.Client.GiveVaultToTeam(ctx, organization.ID, team.ID, organizationVault.ID, &apitypes.GrantTeamVaultRequest{Role: apitypes.VaultOwner}); err != nil {
			t.Fatal(err)
		}
	}
	organizationDetails := requireValue[*apitypes.Organization](t)(alice.Client.GetOrganization(ctx, organization.ID))
	if len(organizationDetails.Teams) != 120 {
		t.Fatalf("teams truncated: %d", len(organizationDetails.Teams))
	}
	for _, team := range organizationDetails.Teams {
		if len(team.MemberIDs) != 1 || len(team.Vaults) != 1 {
			t.Fatalf("team edges incomplete: %+v", team)
		}
	}
	t.Logf("real inventory smoke vaults=%d concurrentRestarts=%d teams=%d with all member/grant edges", len(inventory), transport.conflicts, len(organizationDetails.Teams))
}
