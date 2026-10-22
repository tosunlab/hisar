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

	// Another admin reviews alice's deletion through the same paged plan alice sees for her own deletion.
	adminInvite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, "reviewer@example.com", apitypes.RoleAdmin, nil))
	reviewer, _, err := session.SignUp(ctx, apiclient.New(baseURL), adminInvite.Token, "reviewer@example.com", "Reviewer", []byte("fixture password"), testDevice)
	if err != nil {
		t.Fatal(err)
	}
	defer reviewer.Keyset.Wipe()
	own := requireValue[*apitypes.AccountDeletionPreview](t)(alice.Client.GetAccountDeletionPreview(ctx))
	previewPages := &collectionPageCounter{base: http.DefaultTransport, path: "/api/v1/admin/accounts/" + alice.AccountID + "/deletion-preview"}
	reviewer.Client.HTTP = &http.Client{Transport: previewPages}
	reviewed := requireValue[*apitypes.AccountDeletionPreview](t)(reviewer.Client.GetAdminAccountDeletionPreview(ctx, alice.AccountID))
	if reviewed.PlanVersion == "" || reviewed.PlanVersion != own.PlanVersion || len(reviewed.OwnedVaults) != len(own.OwnedVaults) ||
		len(reviewed.OwnedVaults) <= apitypes.CollectionPageRows || len(reviewed.Organizations) != 1 || previewPages.pages < 2 {
		t.Fatalf("admin deletion preview plan=%q own=%q ownedVaults=%d/%d organizations=%d pages=%d", reviewed.PlanVersion, own.PlanVersion,
			len(reviewed.OwnedVaults), len(own.OwnedVaults), len(reviewed.Organizations), previewPages.pages)
	}
}

// An old peer sends no pagination header and never retries a collection-changed conflict, so a write elsewhere in the
// account must not fail its list.
func TestLegacyPeerListsCompletelyDuringWrites(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	invite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, "legacy@example.com", apitypes.RoleAdmin, nil))
	alice, _, err := session.SignUp(ctx, apiclient.New(baseURL), invite.Token, "legacy@example.com", "Legacy", []byte("fixture password"), testDevice)
	if err != nil {
		t.Fatal(err)
	}
	defer alice.Keyset.Wipe()
	for index := range 130 {
		if _, err := alice.CreateVault(ctx, fmt.Sprintf("Legacy vault %03d", index), apitypes.VaultShared); err != nil {
			t.Fatal(err)
		}
	}
	stop := make(chan struct{})
	written := make(chan int)
	go func() {
		writes := 0
		for {
			select {
			case <-stop:
				written <- writes
				return
			default:
			}
			if _, err := alice.CreateVault(ctx, fmt.Sprintf("Concurrent vault %03d", writes), apitypes.VaultShared); err != nil {
				t.Error(err)
				written <- writes
				return
			}
			writes++
		}
	}()
	var failures []string
	counts := []int{}
	for range 40 {
		status, body := sendRawRequest(t, baseURL, alice.Client.Token, "GET", "/api/v1/vaults", nil)
		var vaults []apitypes.Vault
		if status != http.StatusOK || json.Unmarshal([]byte(body), &vaults) != nil {
			failures = append(failures, fmt.Sprintf("%d %.80s", status, body))
			continue
		}
		ids := map[string]bool{}
		for _, vault := range vaults {
			ids[vault.ID] = true
		}
		counts = append(counts, len(ids))
	}
	close(stop)
	writes := <-written
	if len(failures) > 0 || writes == 0 {
		t.Fatalf("legacy lists during %d writes failed %d of 40 times: %v", writes, len(failures), failures)
	}
	for _, count := range counts {
		if count < 131 || count > 131+writes {
			t.Fatalf("legacy list held %d distinct vaults, outside 131..%d", count, 131+writes)
		}
	}
}

type collectionPageCounter struct {
	base  http.RoundTripper
	path  string
	pages int
}

func (counter *collectionPageCounter) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Path == counter.path && request.Header.Get(apitypes.CollectionPageHeader) != "" {
		counter.pages++
	}
	return counter.base.RoundTrip(request)
}
