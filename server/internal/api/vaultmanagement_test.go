package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
)

func findTestVault(t *testing.T, signedIn *session.Session, vaultID string) (*session.Vault, bool) {
	t.Helper()
	vaults := requireValue[[]session.Vault](t)(signedIn.ListVaults(context.Background()))
	for index := range vaults {
		if vaults[index].ID == vaultID {
			return &vaults[index], true
		}
	}
	return nil, false
}

func findPersonalVault(t *testing.T, signedIn *session.Session) *session.Vault {
	t.Helper()
	vaults := requireValue[[]session.Vault](t)(signedIn.ListVaults(context.Background()))
	for index := range vaults {
		if vaults[index].Type == apitypes.VaultPersonal {
			return &vaults[index]
		}
	}
	t.Fatal("no personal vault")
	return nil
}

func listAuditTypes(t *testing.T, databaseURL string) []string {
	t.Helper()
	ctx := context.Background()
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(ctx)
	rows, err := connection.Query(ctx, `SELECT type FROM audit_events ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	return requireValue[[]string](t)(pgx.CollectRows(rows, pgx.RowTo[string]))
}

func TestVaultRenameDeleteAndLeavePermissions(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	attachMailRecorder(server)
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleMember)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", apitypes.RoleMember)
	carol := signUpTestAccount(t, ctx, server, baseURL, "carol@example.com", apitypes.RoleMember)
	work := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Work", apitypes.VaultShared))
	shareThroughInvitation(t, alice, bob, work, apitypes.VaultEditor)
	shareThroughInvitation(t, alice, carol, work, apitypes.VaultReader)

	bobWork, _ := findTestVault(t, bob, work.ID)
	carolWork, _ := findTestVault(t, carol, work.ID)
	expectStatus(t, "editor renames", bob.Client.RenameVault(ctx, work.ID, &apitypes.RenameVaultRequest{EncName: []byte{1}}), http.StatusForbidden)
	bobWork.Role = apitypes.VaultOwner
	if err := bob.RenameVault(ctx, bobWork, "Taken over"); !apiclient.IsStatus(err, http.StatusForbidden) {
		t.Errorf("editor renames through the session: %v", err)
	}
	expectStatus(t, "editor deletes", bob.Client.DeleteVault(ctx, work.ID, &apitypes.DeleteVaultRequest{IsDeletingItems: true}), http.StatusForbidden)
	expectStatus(t, "reader deletes", carol.Client.DeleteVault(ctx, work.ID, &apitypes.DeleteVaultRequest{IsDeletingItems: true}), http.StatusForbidden)
	stranger := signUpTestAccount(t, ctx, server, baseURL, "dave@example.com", apitypes.RoleMember)
	expectStatus(t, "stranger renames", stranger.Client.RenameVault(ctx, work.ID, &apitypes.RenameVaultRequest{EncName: []byte{1}}), http.StatusNotFound)
	expectStatus(t, "stranger leaves", stranger.Client.RemoveAccess(ctx, work.ID, stranger.AccountID), http.StatusNotFound)

	if err := alice.RenameVault(ctx, work, "Work (2026)"); err != nil {
		t.Fatal(err)
	}
	if renamed, _ := findTestVault(t, bob, work.ID); renamed == nil || renamed.Name != "Work (2026)" {
		t.Errorf("bob sees %+v after the rename", renamed)
	}

	personal := findPersonalVault(t, alice)
	if _, err := alice.DeleteVault(ctx, personal, session.VaultDeletion{}); !errors.Is(err, session.ErrPersonalVault) {
		t.Errorf("deleting the personal vault through the session: %v", err)
	}
	expectStatus(t, "owner deletes the personal vault", alice.Client.DeleteVault(ctx, personal.ID, &apitypes.DeleteVaultRequest{}), http.StatusBadRequest)
	expectStatus(t, "only owner leaves", alice.Client.RemoveAccess(ctx, work.ID, alice.AccountID), http.StatusBadRequest)
	expectStatus(t, "owner leaves the personal vault", alice.Client.RemoveAccess(ctx, personal.ID, alice.AccountID), http.StatusBadRequest)

	if err := carol.LeaveVault(ctx, carolWork); err != nil {
		t.Fatalf("reader leaves: %v", err)
	}
	if _, isFound := findTestVault(t, carol, work.ID); isFound {
		t.Error("carol still lists the vault she left")
	}
	if _, err := carol.Client.ListChanges(ctx, work.ID, 0, 0); !apiclient.IsStatus(err, http.StatusNotFound) {
		t.Errorf("carol reads the vault she left: %v", err)
	}
	members := requireValue[[]apitypes.VaultMember](t)(alice.Client.ListVaultMembers(ctx, work.ID))
	if slices.ContainsFunc(members, func(member apitypes.VaultMember) bool { return member.AccountID == carol.AccountID }) {
		t.Errorf("carol is still a member: %+v", members)
	}

	bobMember := requireValue[*apitypes.Member](t)(alice.GetVerifiedMember(ctx, bob.AccountID))
	if err := alice.ShareVault(ctx, work, bobMember, apitypes.VaultOwner); err != nil {
		t.Fatal(err)
	}
	if err := alice.Client.RemoveAccess(ctx, work.ID, alice.AccountID); err != nil {
		t.Errorf("an owner leaves while another owner remains: %v", err)
	}
	if !slices.Contains(listAuditTypes(t, databaseURL), "vault_renamed") || !slices.Contains(listAuditTypes(t, databaseURL), "vault_left") {
		t.Errorf("audit events: %v", listAuditTypes(t, databaseURL))
	}
}

func getStatusAndError(t *testing.T, signedIn *session.Session, path string) (int, string) {
	t.Helper()
	request := requireValue[*http.Request](t)(http.NewRequest(http.MethodGet, signedIn.Client.BaseURL+path, nil))
	request.Header.Set("Authorization", "Bearer "+signedIn.Client.Token)
	response := requireValue[*http.Response](t)(http.DefaultClient.Do(request))
	defer response.Body.Close()
	var body apitypes.Error
	json.NewDecoder(response.Body).Decode(&body)
	return response.StatusCode, body.Error
}

func TestVaultAccessIsCheckedBeforeTheRequest(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	attachMailRecorder(server)
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleMember)
	carol := signUpTestAccount(t, ctx, server, baseURL, "carol@example.com", apitypes.RoleMember)
	stranger := signUpTestAccount(t, ctx, server, baseURL, "dave@example.com", apitypes.RoleMember)
	work := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Work", apitypes.VaultShared))
	shareThroughInvitation(t, alice, carol, work, apitypes.VaultReader)

	for _, test := range []struct {
		description string
		signedIn    *session.Session
		query       string
		status      int
		message     string
	}{
		{"stranger with a malformed since", stranger, "since=x", http.StatusNotFound, "not found"},
		{"stranger with an out-of-range limit", stranger, "limit=0", http.StatusNotFound, "not found"},
		{"member with a malformed since", carol, "since=x", http.StatusBadRequest, "invalid since"},
		{"member with a negative since", carol, "since=-1", http.StatusBadRequest, "invalid since"},
		{"member with a malformed limit", carol, "limit=x", http.StatusBadRequest, "invalid limit"},
		{"member with a limit above the maximum", carol, "limit=1001", http.StatusBadRequest, "invalid limit"},
		{"member with valid numbers", carol, "since=0&limit=1000", http.StatusOK, ""},
	} {
		status, message := getStatusAndError(t, test.signedIn, "/api/v1/vaults/"+work.ID+"/changes?"+test.query)
		if status != test.status || message != test.message {
			t.Errorf("%s: got %d %q, want %d %q", test.description, status, message, test.status, test.message)
		}
	}

	_, _, err := carol.Client.PutItem(ctx, work.ID, uuid.NewString(), "", &apitypes.PutItemRequest{Revision: uuid.NewString(), Overview: []byte{1}, Details: []byte{1}})
	var apiError *apiclient.APIError
	if !errors.As(err, &apiError) || apiError.Status != http.StatusForbidden || apiError.Message != "read only" {
		t.Errorf("reader saves an item: %v", err)
	}
	_, _, err = stranger.Client.PutItem(ctx, work.ID, uuid.NewString(), "", &apitypes.PutItemRequest{Revision: uuid.NewString(), Overview: []byte{1}, Details: []byte{1}})
	expectStatus(t, "stranger saves an item", err, http.StatusNotFound)
}

// TestTombstoneNeedsAnExistingItem: a deletion replaces an item, so a tombstone for an item that never existed, which no
// app reads or shows, is refused.
func TestTombstoneNeedsAnExistingItem(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleMember)
	work := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Work", apitypes.VaultShared))

	tombstone := &apitypes.PutItemRequest{Revision: uuid.NewString(), Deleted: true, Overview: []byte{1}, Details: []byte{1}, KeyGeneration: work.KeyGeneration}
	_, _, err := alice.Client.PutItem(ctx, work.ID, uuid.NewString(), "", tombstone)
	var apiError *apiclient.APIError
	if !errors.As(err, &apiError) || apiError.Status != http.StatusBadRequest || apiError.Message != "invalid item" {
		t.Fatalf("a tombstone for a new item: %v", err)
	}
	if changes := requireValue[*apitypes.ChangesResponse](t)(alice.Client.ListChanges(ctx, work.ID, 0, 0)); len(changes.Items) != 0 {
		t.Errorf("the refused tombstone was stored: %+v", changes.Items)
	}
	saved := createTestItem(t, alice, work, item.Item{Overview: item.Overview{Title: "Wiki"}})
	if _, err := alice.DeleteItem(ctx, work, &saved); err != nil {
		t.Errorf("deleting an existing item: %v", err)
	}
}

// TestChangesOfLargestItemsFitAResponse: a vault of items at the largest allowed size still syncs, in pages smaller than
// what an app reads of one response.
func TestChangesOfLargestItemsFitAResponse(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleMember)
	work := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Work", apitypes.VaultShared))
	const itemCount = 50
	details := bytes.Repeat([]byte{7}, 1<<20)
	for range itemCount {
		request := &apitypes.PutItemRequest{Revision: uuid.NewString(), Overview: []byte{1}, Details: details, KeyGeneration: work.KeyGeneration}
		if _, _, err := alice.Client.PutItem(ctx, work.ID, uuid.NewString(), "", request); err != nil {
			t.Fatal(err)
		}
	}

	var since int64
	var itemTotal, pageCount int
	for {
		page, err := alice.Client.ListChanges(ctx, work.ID, since, 0)
		if err != nil {
			t.Fatalf("page %d: %v", pageCount+1, err)
		}
		itemTotal += len(page.Items)
		pageCount++
		since = page.NextSeq
		if !page.More {
			break
		}
	}
	if itemTotal != itemCount || pageCount < 2 {
		t.Errorf("synced %d items in %d pages", itemTotal, pageCount)
	}
}

func createTestItem(t *testing.T, owner *session.Session, vault *session.Vault, vaultItem item.Item) item.Item {
	t.Helper()
	if _, err := owner.SaveItem(context.Background(), vault, &vaultItem); err != nil {
		t.Fatal(err)
	}
	return vaultItem
}

func TestDeleteVaultMovesItemsDecryptableIntoTarget(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	attachMailRecorder(server)
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleMember)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", apitypes.RoleMember)
	work := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Work", apitypes.VaultShared))
	shareThroughInvitation(t, alice, bob, work, apitypes.VaultEditor)
	bobOnly := requireValue[*session.Vault](t)(bob.CreateVault(ctx, "Bob's", apitypes.VaultShared))
	shareThroughInvitation(t, bob, alice, bobOnly, apitypes.VaultReader)

	createdItems := []item.Item{
		createTestItem(t, alice, work, item.Item{Overview: item.Overview{Title: "GitHub", Username: "alice", URLs: []string{"https://github.com"}}, Details: item.Details{
			Password: "hunter2", OneTimePassword: "otpauth://totp/GitHub?secret=JBSWY3DPEHPK3PXP", Fields: []item.Field{{Name: "API_TOKEN", Value: "ghp_secret", Concealed: true}},
		}}),
		createTestItem(t, alice, work, item.Item{Overview: item.Overview{Title: "Deploy key", Category: item.CategorySSHKey, PublicKey: "ssh-ed25519 AAAA test"}, Details: item.Details{PrivateKey: "-----BEGIN OPENSSH PRIVATE KEY-----\ntest\n-----END OPENSSH PRIVATE KEY-----\n"}}),
		createTestItem(t, bob, work, item.Item{Overview: item.Overview{Title: "Note"}, Details: item.Details{Notes: "line one\nline two"}}),
	}
	removed := createTestItem(t, alice, work, item.Item{Overview: item.Overview{Title: "Removed"}})
	if _, err := alice.DeleteItem(ctx, work, &removed); err != nil {
		t.Fatal(err)
	}

	if _, err := alice.DeleteVault(ctx, work, session.VaultDeletion{}); !errors.Is(err, session.ErrVaultNotEmpty) {
		t.Errorf("deleting a vault with items without a choice: %v", err)
	}
	aliceBobOnly, _ := findTestVault(t, alice, bobOnly.ID)
	if _, err := alice.DeleteVault(ctx, work, session.VaultDeletion{MoveTo: aliceBobOnly}); !errors.Is(err, session.ErrTargetNotWritable) {
		t.Errorf("moving into a read-only vault through the session: %v", err)
	}
	expectStatus(t, "moving into a read-only vault", alice.Client.DeleteVault(ctx, work.ID, &apitypes.DeleteVaultRequest{MoveToVaultID: bobOnly.ID, MovedItems: []apitypes.MovedItem{}}), http.StatusForbidden)
	personal := findPersonalVault(t, alice)
	expectStatus(t, "moving without every item", alice.Client.DeleteVault(ctx, work.ID, &apitypes.DeleteVaultRequest{MoveToVaultID: personal.ID, MovedItems: []apitypes.MovedItem{}}), http.StatusConflict)

	bobEvents := openEventStream(t, server, bob.Client, bob.AccountID)
	waitForSubscriber(t, server, bob.AccountID, 1)
	movedCount, err := alice.DeleteVault(ctx, work, session.VaultDeletion{MoveTo: personal})
	if err != nil || movedCount != len(createdItems) {
		t.Fatalf("move and delete: %d moved, %v", movedCount, err)
	}
	bobEvents.waitForEvent(t, apitypes.EventVaultsChanged)

	movedItems := requireValue[[]item.Item](t)(alice.ListItems(ctx, personal))
	if len(movedItems) != len(createdItems) {
		t.Fatalf("personal vault has %d items, want %d", len(movedItems), len(createdItems))
	}
	for _, created := range createdItems {
		index := slices.IndexFunc(movedItems, func(moved item.Item) bool { return moved.ID == created.ID })
		if index < 0 {
			t.Fatalf("%q was not moved", created.Overview.Title)
		}
		if !reflect.DeepEqual(movedItems[index].Overview, created.Overview) || !reflect.DeepEqual(movedItems[index].Details, created.Details) {
			t.Errorf("moved item differs:\n got %+v\nwant %+v", movedItems[index], created)
		}
	}
	if _, isFound := findTestVault(t, alice, work.ID); isFound {
		t.Error("alice still lists the deleted vault")
	}
	if _, isFound := findTestVault(t, bob, work.ID); isFound {
		t.Error("bob still lists the deleted vault")
	}
	if _, err := bob.Client.ListChanges(ctx, work.ID, 0, 0); !apiclient.IsStatus(err, http.StatusNotFound) {
		t.Errorf("reading the deleted vault: %v", err)
	}
	history := requireValue[[]apitypes.Item](t)(alice.Client.GetItemHistory(ctx, personal.ID, createdItems[0].ID))
	if len(history) != 1 {
		t.Errorf("moved item history: %d entries", len(history))
	}

	scratch := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Scratch", apitypes.VaultShared))
	createTestItem(t, alice, scratch, item.Item{Overview: item.Overview{Title: "Throwaway"}})
	if _, err := alice.DeleteVault(ctx, scratch, session.VaultDeletion{IsDeletingItems: true}); err != nil {
		t.Fatalf("delete with items: %v", err)
	}
	if _, isFound := findTestVault(t, alice, scratch.ID); isFound {
		t.Error("the vault deleted with its items is still listed")
	}
	if !slices.Contains(listAuditTypes(t, databaseURL), "vault_deleted") {
		t.Errorf("audit events: %v", listAuditTypes(t, databaseURL))
	}
}
