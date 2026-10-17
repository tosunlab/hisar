package api

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
)

func findTestItem(t *testing.T, signedIn *session.Session, vault *session.Vault, itemID string) (*item.Item, bool) {
	t.Helper()
	items := requireValue[[]item.Item](t)(signedIn.ListItems(context.Background(), vault))
	for index := range items {
		if items[index].ID == itemID {
			return &items[index], true
		}
	}
	return nil, false
}

func TestMoveItemBetweenVaults(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	attachMailRecorder(server)
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleMember)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", apitypes.RoleMember)
	carol := signUpTestAccount(t, ctx, server, baseURL, "carol@example.com", apitypes.RoleMember)
	work := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Work", apitypes.VaultShared))
	shareThroughInvitation(t, alice, bob, work, apitypes.VaultEditor)
	shareThroughInvitation(t, alice, carol, work, apitypes.VaultReader)
	personal := findPersonalVault(t, alice)

	github := &item.Item{Overview: item.Overview{Title: "GitHub", Username: "alice"}, Details: item.Details{Password: "github-password"}}
	if _, err := alice.SaveItem(ctx, personal, github); err != nil {
		t.Fatal(err)
	}
	baseRevision := github.Revision
	if _, err := alice.MoveItem(ctx, personal, work, github); err != nil {
		t.Fatal(err)
	}
	if github.Revision == baseRevision {
		t.Error("the moved item kept its revision")
	}
	bobWork, _ := findTestVault(t, bob, work.ID)
	if moved, isFound := findTestItem(t, bob, bobWork, github.ID); !isFound || moved.Details.Password != "github-password" || moved.Revision != github.Revision {
		t.Errorf("bob sees %+v in Work", moved)
	}
	if _, isFound := findTestItem(t, alice, personal, github.ID); isFound {
		t.Error("the item is still in alice's Personal vault")
	}

	// A move from a stale revision is refused with the current item.
	staleItem := *github
	staleItem.Revision = baseRevision
	if current, err := alice.MoveItem(ctx, work, personal, &staleItem); !errors.Is(err, session.ErrConflict) || current == nil || current.Revision != github.Revision {
		t.Errorf("stale move: %+v, %v", current, err)
	}

	// Readers can't move items out of a vault, and nobody can move items into a vault they only read.
	carolWork, _ := findTestVault(t, carol, work.ID)
	carolPersonal := findPersonalVault(t, carol)
	carolItem := *github
	expectStatus(t, "reader moves out", ignoreItem(carol.MoveItem(ctx, carolWork, carolPersonal, &carolItem)), http.StatusForbidden)
	ops := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Ops", apitypes.VaultShared))
	shareThroughInvitation(t, alice, bob, ops, apitypes.VaultReader)
	bobOps, _ := findTestVault(t, bob, ops.ID)
	bobOps.Role = apitypes.VaultEditor
	bobItem := *github
	expectStatus(t, "editor moves into a vault they read", ignoreItem(bob.MoveItem(ctx, bobWork, bobOps, &bobItem)), http.StatusForbidden)

	// Moving back fills the tombstone's place; the item never lives in both vaults.
	if _, err := alice.MoveItem(ctx, work, personal, github); err != nil {
		t.Fatalf("move back: %v", err)
	}
	if _, isFound := findTestItem(t, alice, personal, github.ID); !isFound {
		t.Error("the item did not come back to Personal")
	}
	if _, isFound := findTestItem(t, bob, bobWork, github.ID); isFound {
		t.Error("the item is still in Work")
	}

	invalid := &apitypes.MoveItemRequest{
		TargetVaultID: work.ID, BaseRevision: github.Revision,
		Tombstone: apitypes.PutItemRequest{Revision: uuid.NewString(), Overview: []byte{1}, Details: []byte{1}},
		Moved:     apitypes.PutItemRequest{Revision: uuid.NewString(), Overview: []byte{1}, Details: []byte{1}},
	}
	_, _, err := alice.Client.MoveItem(ctx, personal.ID, github.ID, invalid)
	expectStatus(t, "tombstone that is not deleted", err, http.StatusBadRequest)
	if !slices.Contains(listAuditTypes(t, databaseURL), "item_moved") {
		t.Errorf("audit events: %v", listAuditTypes(t, databaseURL))
	}
}

func ignoreItem(_ *item.Item, err error) error { return err }
