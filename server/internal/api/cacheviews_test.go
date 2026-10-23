package api

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cache"
	"github.com/tryhisar/hisar/core/export"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
)

func TestRecoveryCopyThroughAPI(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	signedIn := signUpTestAccount(t, ctx, server, baseURL, "copy@example.com", apitypes.RoleMember)
	defer signedIn.Lock()
	personal := findPersonalVault(t, signedIn)
	retry := requireValue[*session.Vault](t)(signedIn.CreateVault(ctx, "Personal", apitypes.VaultPersonal))
	if retry.ID != personal.ID {
		t.Fatal("onboarding retry created another Personal")
	}
	healthy := &item.Item{Overview: item.Overview{Title: "Rescue"}, Details: item.Details{Password: "made-up-test-secret"}}
	requireValue[*item.Item](t)(signedIn.SaveItem(ctx, personal, healthy))
	c := requireValue[*cache.Cache](t)(cache.Open(filepath.Join(t.TempDir(), "copy.db")))
	defer c.Close()
	if err := signedIn.AttachCache(ctx, c); err != nil {
		t.Fatal(err)
	}
	if _, err := signedIn.ReadVaults(ctx); err != nil {
		t.Fatal(err)
	}
	originalURL := signedIn.Client.BaseURL
	signedIn.Client.BaseURL = "http://127.0.0.1:1"
	unseen, err := export.Collect(ctx, signedIn)
	if err != nil || len(unseen.Vaults) != 0 || !unseen.Provenance.IsCachedCopy() || len(unseen.Provenance.Omissions) != 1 {
		t.Fatalf("unseen server item became a completed empty vault: %+v %v", unseen, err)
	}
	signedIn.Client.BaseURL = originalURL
	badID := uuid.NewString()
	badRevision := uuid.NewString()
	if _, _, err := signedIn.Client.PutItem(ctx, personal.ID, badID, "", &apitypes.PutItemRequest{Revision: badRevision, Overview: []byte("bounded invalid ciphertext"), Details: []byte("bounded invalid ciphertext"), KeyGeneration: personal.KeyGeneration}); err != nil {
		t.Fatal(err)
	}
	read, err := signedIn.ReadItems(ctx, retry)
	if err != nil || len(read.Items) != 1 || read.Items[0].Details.Password != "made-up-test-secret" || len(read.Issues) != 1 {
		t.Fatalf("healthy item unavailable: %+v %v", read, err)
	}
	partial, err := export.Collect(ctx, signedIn)
	if err != nil || !partial.Provenance.IsPartial() {
		t.Fatalf("export hid unreadable record: %+v %v", partial, err)
	}
	if err := signedIn.DeleteUnreadableItem(ctx, retry, badID, badRevision); err != nil {
		t.Fatal(err)
	}
	read, err = signedIn.ReadItems(ctx, retry)
	if err != nil || len(read.Issues) != 0 || len(read.Items) != 1 {
		t.Fatalf("safe repair damaged healthy record: %+v %v", read, err)
	}
	signedIn.Client.BaseURL = "http://127.0.0.1:1"
	rescue, err := export.Collect(ctx, signedIn)
	if err != nil || !rescue.Provenance.IsCachedCopy() || rescue.Provenance.IsPartial() || len(rescue.Vaults) != 1 || len(rescue.Vaults[0].Items) != 1 {
		t.Fatalf("completed rescue copy unavailable with server gone: %+v %v", rescue, err)
	}
	document := export.Build(export.Account{Email: signedIn.Email, Server: originalURL}, rescue.Vaults, time.Now())
	document.Provenance = &rescue.Provenance
	encoded, err := export.EncodePlain(document)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := export.Decode(encoded, nil)
	if err != nil || restored.CountItem() != 1 || !restored.Provenance.IsCachedCopy() {
		t.Fatalf("portable copy lost provenance: %+v %v", restored, err)
	}
}
