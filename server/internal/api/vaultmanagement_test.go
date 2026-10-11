package api

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/tryhisar/hisar/core/client"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/core/session"
)

func findTestVault(t *testing.T, signedIn *session.Session, vaultID string) (*session.Vault, bool) {
	t.Helper()
	allVault := requireValue[[]session.Vault](t)(signedIn.ListAllVault(context.Background()))
	for index := range allVault {
		if allVault[index].ID == vaultID {
			return &allVault[index], true
		}
	}
	return nil, false
}

func findPersonalVault(t *testing.T, signedIn *session.Session) *session.Vault {
	t.Helper()
	allVault := requireValue[[]session.Vault](t)(signedIn.ListAllVault(context.Background()))
	for index := range allVault {
		if allVault[index].Type == proto.VaultPersonal {
			return &allVault[index]
		}
	}
	t.Fatal("no personal vault")
	return nil
}

func listAllAuditType(t *testing.T, databaseURL string) []string {
	t.Helper()
	ctx := context.Background()
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(ctx)
	allRow, err := connection.Query(ctx, `SELECT type FROM audit_events ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	return requireValue[[]string](t)(pgx.CollectRows(allRow, pgx.RowTo[string]))
}

func TestVaultRenameDeleteAndLeavePermissions(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	attachMailRecorder(server)
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", proto.RoleMember)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", proto.RoleMember)
	carol := signUpTestAccount(t, ctx, server, baseURL, "carol@example.com", proto.RoleMember)
	work := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Work", proto.VaultShared))
	shareThroughInvitation(t, alice, bob, work, proto.VaultEditor)
	shareThroughInvitation(t, alice, carol, work, proto.VaultReader)

	bobWork, _ := findTestVault(t, bob, work.ID)
	carolWork, _ := findTestVault(t, carol, work.ID)
	expectStatus(t, "editor renames", bob.Client.RenameVault(ctx, work.ID, &proto.RenameVaultRequest{EncName: []byte{1}}), http.StatusForbidden)
	bobWork.Role = proto.VaultOwner
	if err := bob.RenameVault(ctx, bobWork, "Taken over"); !client.IsStatus(err, http.StatusForbidden) {
		t.Errorf("editor renames through the session: %v", err)
	}
	expectStatus(t, "editor deletes", bob.Client.DeleteVault(ctx, work.ID, &proto.DeleteVaultRequest{IsDeletingItems: true}), http.StatusForbidden)
	expectStatus(t, "reader deletes", carol.Client.DeleteVault(ctx, work.ID, &proto.DeleteVaultRequest{IsDeletingItems: true}), http.StatusForbidden)
	stranger := signUpTestAccount(t, ctx, server, baseURL, "dave@example.com", proto.RoleMember)
	expectStatus(t, "stranger renames", stranger.Client.RenameVault(ctx, work.ID, &proto.RenameVaultRequest{EncName: []byte{1}}), http.StatusNotFound)
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
	expectStatus(t, "owner deletes the personal vault", alice.Client.DeleteVault(ctx, personal.ID, &proto.DeleteVaultRequest{}), http.StatusBadRequest)
	expectStatus(t, "only owner leaves", alice.Client.RemoveAccess(ctx, work.ID, alice.AccountID), http.StatusBadRequest)
	expectStatus(t, "owner leaves the personal vault", alice.Client.RemoveAccess(ctx, personal.ID, alice.AccountID), http.StatusBadRequest)

	if err := carol.LeaveVault(ctx, carolWork); err != nil {
		t.Fatalf("reader leaves: %v", err)
	}
	if _, isFound := findTestVault(t, carol, work.ID); isFound {
		t.Error("carol still lists the vault she left")
	}
	if _, err := carol.Client.ListAllChange(ctx, work.ID, 0, 0); !client.IsStatus(err, http.StatusNotFound) {
		t.Errorf("carol reads the vault she left: %v", err)
	}
	allMember := requireValue[[]proto.VaultMember](t)(alice.Client.ListAllVaultMember(ctx, work.ID))
	if slices.ContainsFunc(allMember, func(member proto.VaultMember) bool { return member.AccountID == carol.AccountID }) {
		t.Errorf("carol is still a member: %+v", allMember)
	}

	bobMember := requireValue[*proto.Member](t)(alice.GetVerifiedMember(ctx, bob.AccountID))
	if err := alice.ShareVault(ctx, work, bobMember, proto.VaultOwner); err != nil {
		t.Fatal(err)
	}
	if err := alice.Client.RemoveAccess(ctx, work.ID, alice.AccountID); err != nil {
		t.Errorf("an owner leaves while another owner remains: %v", err)
	}
	if !slices.Contains(listAllAuditType(t, databaseURL), "vault_renamed") || !slices.Contains(listAllAuditType(t, databaseURL), "vault_left") {
		t.Errorf("audit events: %v", listAllAuditType(t, databaseURL))
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
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", proto.RoleMember)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", proto.RoleMember)
	work := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Work", proto.VaultShared))
	shareThroughInvitation(t, alice, bob, work, proto.VaultEditor)
	bobOnly := requireValue[*session.Vault](t)(bob.CreateVault(ctx, "Bob's", proto.VaultShared))
	shareThroughInvitation(t, bob, alice, bobOnly, proto.VaultReader)

	allCreated := []item.Item{
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
	expectStatus(t, "moving into a read-only vault", alice.Client.DeleteVault(ctx, work.ID, &proto.DeleteVaultRequest{MoveToVaultID: bobOnly.ID, AllMovedItem: []proto.MovedItem{}}), http.StatusForbidden)
	personal := findPersonalVault(t, alice)
	expectStatus(t, "moving without every item", alice.Client.DeleteVault(ctx, work.ID, &proto.DeleteVaultRequest{MoveToVaultID: personal.ID, AllMovedItem: []proto.MovedItem{}}), http.StatusConflict)

	bobEvents := openEventStream(t, bob.Client)
	waitForSubscriber(t, server, bob.AccountID, 1)
	movedCount, err := alice.DeleteVault(ctx, work, session.VaultDeletion{MoveTo: personal})
	if err != nil || movedCount != len(allCreated) {
		t.Fatalf("move and delete: %d moved, %v", movedCount, err)
	}
	bobEvents.waitForEvent(t, proto.EventVaultsChanged)

	allMoved := requireValue[[]item.Item](t)(alice.ListAllItem(ctx, personal))
	if len(allMoved) != len(allCreated) {
		t.Fatalf("personal vault has %d items, want %d", len(allMoved), len(allCreated))
	}
	for _, created := range allCreated {
		index := slices.IndexFunc(allMoved, func(moved item.Item) bool { return moved.ID == created.ID })
		if index < 0 {
			t.Fatalf("%q was not moved", created.Overview.Title)
		}
		if !reflect.DeepEqual(allMoved[index].Overview, created.Overview) || !reflect.DeepEqual(allMoved[index].Details, created.Details) {
			t.Errorf("moved item differs:\n got %+v\nwant %+v", allMoved[index], created)
		}
	}
	if _, isFound := findTestVault(t, alice, work.ID); isFound {
		t.Error("alice still lists the deleted vault")
	}
	if _, isFound := findTestVault(t, bob, work.ID); isFound {
		t.Error("bob still lists the deleted vault")
	}
	if _, err := bob.Client.ListAllChange(ctx, work.ID, 0, 0); !client.IsStatus(err, http.StatusNotFound) {
		t.Errorf("reading the deleted vault: %v", err)
	}
	history := requireValue[[]proto.Item](t)(alice.Client.GetItemHistory(ctx, personal.ID, allCreated[0].ID))
	if len(history) != 1 {
		t.Errorf("moved item history: %d entries", len(history))
	}

	scratch := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Scratch", proto.VaultShared))
	createTestItem(t, alice, scratch, item.Item{Overview: item.Overview{Title: "Throwaway"}})
	if _, err := alice.DeleteVault(ctx, scratch, session.VaultDeletion{IsDeletingItems: true}); err != nil {
		t.Fatalf("delete with items: %v", err)
	}
	if _, isFound := findTestVault(t, alice, scratch.ID); isFound {
		t.Error("the vault deleted with its items is still listed")
	}
	if !slices.Contains(listAllAuditType(t, databaseURL), "vault_deleted") {
		t.Errorf("audit events: %v", listAllAuditType(t, databaseURL))
	}
}
