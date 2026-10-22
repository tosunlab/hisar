package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
)

type transitionTransport struct {
	base        http.RoundTripper
	maxUpload   int64
	itemBatches int
	lostCommit  bool
}

func (tr *transitionTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if strings.Contains(request.URL.Path, "/transitions/") && strings.HasSuffix(request.URL.Path, "/items") && request.Method == "POST" {
		tr.itemBatches++
		if request.ContentLength > tr.maxUpload {
			tr.maxUpload = request.ContentLength
		}
	}
	response, err := tr.base.RoundTrip(request)
	if err == nil && strings.HasSuffix(request.URL.Path, "/commit") && !tr.lostCommit && response.StatusCode == http.StatusOK {
		tr.lostCommit = true
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		return nil, io.ErrUnexpectedEOF
	}
	return response, err
}

func TestLargeVaultMemberRemovalUsesAtomicBoundedStaging(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	attachMailRecorder(server)
	ctx := context.Background()
	owner := signUpTestAccount(t, ctx, server, baseURL, "owner@example.com", apitypes.RoleMember)
	removed := signUpTestAccount(t, ctx, server, baseURL, "removed@example.com", apitypes.RoleMember)
	remaining := signUpTestAccount(t, ctx, server, baseURL, "remaining@example.com", apitypes.RoleMember)
	vault := requireValue[*session.Vault](t)(owner.CreateVault(ctx, "Large accepted vault", apitypes.VaultShared))
	shareThroughInvitation(t, owner, removed, vault, apitypes.VaultEditor)
	shareThroughInvitation(t, owner, remaining, vault, apitypes.VaultReader)
	payload := strings.Repeat("x", 900<<10)
	for range 40 {
		createTestItem(t, owner, vault, item.Item{Overview: item.Overview{Title: "Large note"}, Details: item.Details{Notes: payload}})
	}
	deleted := createTestItem(t, owner, vault, item.Item{Overview: item.Overview{Title: "Deleted"}})
	requireValue[*item.Item](t)(owner.DeleteItem(ctx, vault, &deleted))
	oldKey := bytes.Clone(vault.Key)
	transport := &transitionTransport{base: http.DefaultTransport}
	owner.Client.HTTP = &http.Client{Transport: transport}
	if err := owner.RemoveVaultMember(ctx, vault, removed.AccountID); err != nil {
		t.Fatal(err)
	}
	if !transport.lostCommit || transport.itemBatches < 2 || transport.maxUpload > apitypes.VaultTransitionPageBytes {
		t.Fatalf("bounded uploads and lost-response recovery: batches=%d max=%d reconciled=%v", transport.itemBatches, transport.maxUpload, transport.lostCommit)
	}
	if vault.KeyGeneration != 2 || vault.NeedsRotation || bytes.Equal(vault.Key, oldKey) {
		t.Fatal("removal did not complete a new generation")
	}
	if _, err := removed.FindVault(ctx, vault.ID); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("excluded member received a new grant: %v", err)
	}
	readerVault := requireValue[*session.Vault](t)(remaining.FindVault(ctx, vault.ID))
	items := requireValue[[]item.Item](t)(remaining.ListItems(ctx, readerVault))
	if len(items) != 40 {
		t.Fatalf("remaining member read %d live items", len(items))
	}
	for _, item := range items {
		if item.Details.Notes != payload {
			t.Fatal("remaining member lost accepted data")
		}
	}
	total := 0
	var since int64
	for {
		page := requireValue[*apitypes.ChangesResponse](t)(remaining.Client.ListChanges(ctx, vault.ID, since, 100))
		for _, record := range page.Items {
			total++
			if _, err := crypt.OpenItemPart(readerVault.Key, vault.ID, record.ID, record.Revision, crypt.Details, record.Details); err != nil {
				t.Fatalf("remaining member cannot read a live item or tombstone: %v", err)
			}
			if _, err := crypt.OpenItemPart(oldKey, vault.ID, record.ID, record.Revision, crypt.Details, record.Details); err == nil {
				t.Fatal("excluded member's key opens the new generation")
			}
		}
		if !page.More {
			break
		}
		since = page.NextSeq
	}
	if total != 41 {
		t.Fatalf("rotation lost tombstones or items: %d", total)
	}
	createTestItem(t, owner, vault, item.Item{Overview: item.Overview{Title: "After removal"}, Details: item.Details{Password: "new-generation-only"}})
	if got := requireValue[[]item.Item](t)(remaining.ListItems(ctx, readerVault)); len(got) != 41 {
		t.Fatal("remaining member cannot read subsequent writes")
	}
	legacy := &apitypes.PutItemRequest{Revision: deleted.Revision, Overview: []byte{1}, Details: []byte{1}}
	if _, _, err := owner.Client.PutItem(ctx, vault.ID, deleted.ID, "", legacy); !apiclient.IsStatus(err, 400) || !strings.Contains(err.Error(), "update Hisar") {
		t.Fatalf("legacy write after rotation: %v", err)
	}

	target := findPersonalVault(t, owner)
	moved, err := owner.DeleteVault(ctx, vault, session.VaultDeletion{MoveTo: target})
	if err != nil || moved != 41 {
		t.Fatalf("large delete-and-move: %d %v", moved, err)
	}
	if got := requireValue[[]item.Item](t)(owner.ListItems(ctx, target)); len(got) != 41 {
		t.Fatalf("large deletion move lost records: %d", len(got))
	}
}

func TestRawRemovalAndVoluntaryLeaveHaveDistinctRotationPolicy(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	attachMailRecorder(server)
	ctx := context.Background()
	owner := signUpTestAccount(t, ctx, server, baseURL, "owner@example.com", apitypes.RoleMember)
	member := signUpTestAccount(t, ctx, server, baseURL, "member@example.com", apitypes.RoleMember)
	vault := requireValue[*session.Vault](t)(owner.CreateVault(ctx, "Raw removal", apitypes.VaultShared))
	shareThroughInvitation(t, owner, member, vault, apitypes.VaultEditor)
	if err := owner.Client.RemoveAccess(ctx, vault.ID, member.AccountID); err != nil {
		t.Fatal(err)
	}
	listed, _ := findListedVault(t, owner, vault.ID)
	if !listed.NeedsRotation {
		t.Fatal("raw owner removal omitted pending rotation")
	}
	if err := owner.RenameVault(ctx, vault, "Exposed generation"); !apiclient.IsStatus(err, 400) {
		t.Fatalf("pending rotation accepted new ciphertext: %v", err)
	}
	if _, err := owner.SaveItem(ctx, vault, &item.Item{Overview: item.Overview{Title: "Exposed key write"}}); !apiclient.IsStatus(err, 400) || !strings.Contains(err.Error(), "pending vault key rotation") {
		t.Fatalf("pending write was stored or misreported as an item conflict: %v", err)
	}
	rotated := requireValue[[]session.RotatedVault](t)(owner.RotatePendingVaults(ctx))
	if len(rotated) != 1 {
		t.Fatalf("owner sync did not finish raw removal: %+v", rotated)
	}
	current := requireValue[*session.Vault](t)(owner.FindVault(ctx, vault.ID))
	shareThroughInvitation(t, owner, member, current, apitypes.VaultEditor)
	memberVault := requireValue[*session.Vault](t)(member.FindVault(ctx, vault.ID))
	if err := member.LeaveVault(ctx, memberVault); err != nil {
		t.Fatal(err)
	}
	listed, _ = findListedVault(t, owner, vault.ID)
	if listed.NeedsRotation || listed.KeyGeneration != 2 {
		t.Fatal("voluntary leave unexpectedly promised rotation")
	}
	t.Run("deletion reloads a rotated source generation", func(t *testing.T) {
		empty := requireValue[*session.Vault](t)(owner.CreateVault(ctx, "Rotated before deletion", apitypes.VaultShared))
		stale := *empty
		stale.Key = bytes.Clone(empty.Key)
		if err := owner.RotateVaultKey(ctx, empty, []session.AccessGrant{{AccountID: owner.AccountID, Role: apitypes.VaultOwner}}); err != nil {
			t.Fatal(err)
		}
		if _, err := owner.DeleteVault(ctx, &stale, session.VaultDeletion{}); err != nil {
			t.Fatalf("a first-party deletion kept sending the retired source generation: %v", err)
		}
		if _, err := owner.FindVault(ctx, empty.ID); !errors.Is(err, session.ErrNotFound) {
			t.Fatalf("rotated empty vault was not deleted: %v", err)
		}
	})
}

func TestStagedSnapshotCannotOverwriteConcurrentAPIEdit(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	owner := signUpTestAccount(t, ctx, server, baseURL, "owner@example.com", apitypes.RoleMember)
	vault := requireValue[*session.Vault](t)(owner.CreateVault(ctx, "Concurrent edits", apitypes.VaultShared))
	original := createTestItem(t, owner, vault, item.Item{Overview: item.Overview{Title: "Original"}, Details: item.Details{Password: "original"}})
	op := requireValue[*apitypes.VaultTransition](t)(owner.Client.BeginVaultTransition(ctx, vault.ID, &apitypes.BeginVaultTransitionRequest{
		Protocol: 1, ID: uuid.NewString(), Kind: "rotate", KeyGeneration: 1,
	}))
	page := requireValue[apitypes.VaultTransitionPage](t)(owner.Client.ListVaultTransitionItems(ctx, vault.ID, op.ID, ""))
	if len(page.Items) != 1 {
		t.Fatal("snapshot omitted the source item")
	}
	newKey := crypt.GenerateKey()
	defer clear(newKey)
	record := page.Items[0]
	revision := uuid.NewString()
	overview := requireValue[[]byte](t)(crypt.OpenItemPart(vault.Key, vault.ID, record.ID, record.Revision, crypt.Overview, record.Overview))
	details := requireValue[[]byte](t)(crypt.OpenItemPart(vault.Key, vault.ID, record.ID, record.Revision, crypt.Details, record.Details))
	staged := apitypes.RotatedItem{ID: record.ID, BaseRevision: record.Revision, Revision: revision,
		Overview: requireValue[[]byte](t)(crypt.SealItemPart(newKey, vault.ID, record.ID, revision, crypt.Overview, overview)),
		Details:  requireValue[[]byte](t)(crypt.SealItemPart(newKey, vault.ID, record.ID, revision, crypt.Details, details))}
	if err := owner.Client.StageVaultItems(ctx, vault.ID, op.ID, &apitypes.StageVaultItemsRequest{Items: []apitypes.RotatedItem{staged}}); err != nil {
		t.Fatal(err)
	}
	if got := requireValue[[]item.Item](t)(owner.ListItems(ctx, vault)); len(got) != 1 || got[0].Details.Password != "original" {
		t.Fatal("interrupted staging changed the active generation")
	}
	original.Details.Password = "concurrent edit"
	requireValue[*item.Item](t)(owner.SaveItem(ctx, vault, &original))
	name := requireValue[[]byte](t)(crypt.SealVaultName(newKey, vault.ID, []byte(vault.Name)))
	if _, err := owner.Client.CommitVaultTransition(ctx, vault.ID, op.ID, &apitypes.CommitVaultTransitionRequest{EncName: name}); !apiclient.IsStatus(err, 409) {
		t.Fatalf("concurrent edit overwritten: %v", err)
	}
	if _, err := owner.Client.ListVaultTransitionItems(ctx, vault.ID, op.ID, ""); !apiclient.IsStatus(err, 409) {
		t.Fatalf("changed snapshot continued: %v", err)
	}
	if err := owner.Client.AbortVaultTransition(ctx, vault.ID, op.ID); err != nil {
		t.Fatal(err)
	}
	if got := requireValue[[]item.Item](t)(owner.ListItems(ctx, vault)); len(got) != 1 || got[0].Details.Password != "concurrent edit" {
		t.Fatal("failed staging lost the concurrent edit")
	}
}
