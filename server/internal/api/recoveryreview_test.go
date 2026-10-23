package api

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/app"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
)

type missingSecretKeyStore struct{}

func (missingSecretKeyStore) LoadSecretKey(string) (crypt.SecretKey, error) {
	return crypt.SecretKey{}, errors.New("no Secret Key in this test")
}
func (missingSecretKeyStore) StoreSecretKey(crypt.SecretKey) error { return nil }
func (missingSecretKeyStore) DeleteSecretKey(string) error         { return nil }

func findVaultIssue(t *testing.T, service *app.Service, vaultID string) session.ReadIssue {
	t.Helper()
	summaries, err := service.ListVaults()
	if err != nil {
		t.Fatal(err)
	}
	for _, summary := range summaries {
		for _, issue := range summary.ReadIssues {
			if issue.VaultID == vaultID && issue.ItemID == "" {
				return issue
			}
		}
	}
	t.Fatalf("vault %s shows no issue", vaultID)
	return session.ReadIssue{}
}

func TestRecoveryActionsActOnlyOnTheReviewedRecord(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	signedIn := signUpTestAccount(t, ctx, server, baseURL, "recovery@example.com", apitypes.RoleMember)
	personal := findPersonalVault(t, signedIn)
	shared := requireValue[*session.Vault](t)(signedIn.CreateVault(ctx, "Recovery", apitypes.VaultShared))
	healthy := &item.Item{Overview: item.Overview{Title: "Healthy item"}, Details: item.Details{Password: "made-up-test-secret"}}
	requireValue[*item.Item](t)(signedIn.SaveItem(ctx, shared, healthy))
	service, err := app.NewService(app.Config{StatePath: filepath.Join(t.TempDir(), "state.json"), SecretKeyStore: missingSecretKeyStore{}, IsEventStreamDisabled: true, AutoLockAfter: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	app.OpenUnlockedSession(service, baseURL, signedIn.Client.Token, signedIn)
	defer service.Lock()
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(ctx)
	setEncryptedName := func(encryptedName []byte) {
		t.Helper()
		if _, err := connection.Exec(ctx, `UPDATE vaults SET enc_name = $2 WHERE id = $1`, shared.ID, encryptedName); err != nil {
			t.Fatal(err)
		}
	}
	countVaultRows := func() int {
		return countDatabaseRows(t, databaseURL, `SELECT count(*) FROM vaults WHERE id = $1`, shared.ID)
	}

	badID, firstRevision, secondRevision := uuid.NewString(), uuid.NewString(), uuid.NewString()
	putUnreadable := func(baseRevision, revision string) {
		t.Helper()
		if _, _, err := signedIn.Client.PutItem(ctx, shared.ID, badID, baseRevision, &apitypes.PutItemRequest{Revision: revision, Overview: []byte("bad"), Details: []byte("bad"), KeyGeneration: shared.KeyGeneration}); err != nil {
			t.Fatal(err)
		}
	}
	putUnreadable("", firstRevision)
	requireValue[[]app.VaultSummary](t)(service.ListVaults())
	requireValue[[]app.ItemDetail](t)(service.ListItems(shared.ID))
	if err := service.DeleteUnreadableItem(personal.ID, badID, firstRevision); err == nil {
		t.Fatal("an unreadable item was deleted through another vault")
	}
	putUnreadable(firstRevision, secondRevision)
	if err := service.DeleteUnreadableItem(shared.ID, badID, firstRevision); err == nil {
		t.Fatal("an unreadable item was deleted at a revision nobody reviewed")
	}
	requireValue[[]app.ItemDetail](t)(service.ListItems(shared.ID))
	if _, err := connection.Exec(ctx, `UPDATE vault_access SET role = 'reader' WHERE vault_id = $1 AND account_id = $2`, shared.ID, signedIn.AccountID); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteUnreadableItem(shared.ID, badID, secondRevision); err == nil {
		t.Fatal("a reviewed issue outlived the reader's lost write permission")
	}
	if _, err := connection.Exec(ctx, `UPDATE vault_access SET role = 'owner' WHERE vault_id = $1 AND account_id = $2`, shared.ID, signedIn.AccountID); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteUnreadableItem(shared.ID, badID, secondRevision); err != nil {
		t.Fatal(err)
	}
	read, err := signedIn.ReadItems(ctx, shared)
	if err != nil || len(read.Issues) != 0 || len(read.Items) != 1 || read.Items[0].ID != healthy.ID {
		t.Fatalf("deleting the reviewed unreadable item lost the healthy one: %+v %v", read, err)
	}

	var original []byte
	if err := connection.QueryRow(ctx, `SELECT enc_name FROM vaults WHERE id = $1`, shared.ID).Scan(&original); err != nil {
		t.Fatal(err)
	}
	setEncryptedName([]byte("bad-name"))
	staleIssue := findVaultIssue(t, service, shared.ID)
	setEncryptedName(original)
	if _, err := service.RepairVaultName(shared.ID, staleIssue.Revision, "Stale repair"); err == nil {
		t.Fatal("a stale review overwrote the name another device repaired")
	}
	if _, err := service.RenameVault(shared.ID, "Stale rename"); err == nil {
		t.Fatal("renaming the vault last seen unreadable overwrote the name another device repaired")
	}
	if err := service.DeleteUnreadableVault(shared.ID, staleIssue.Revision); err == nil {
		t.Fatal("a stale review deleted the vault another device repaired")
	}
	if countVaultRows() != 1 || countDatabaseRows(t, databaseURL, `SELECT count(*) FROM vaults WHERE id = $1 AND enc_name = $2`, shared.ID, original) != 1 {
		t.Fatal("a refused recovery action changed the repaired vault")
	}

	setEncryptedName([]byte("bad-name"))
	reviewed := findVaultIssue(t, service, shared.ID)
	if _, err := service.RepairVaultName(shared.ID, reviewed.Revision, "Repaired"); err != nil {
		t.Fatalf("repairing the reviewed name: %v", err)
	}
	if renamed, _ := findTestVault(t, signedIn, shared.ID); renamed == nil || renamed.Name != "Repaired" {
		t.Fatalf("the repaired name does not open: %+v", renamed)
	}

	setEncryptedName([]byte("bad-name"))
	reviewed = findVaultIssue(t, service, shared.ID)
	if err := service.DeleteUnreadableVault(shared.ID, reviewed.Revision); err != nil {
		t.Fatalf("deleting the reviewed unreadable vault: %v", err)
	}
	if countVaultRows() != 0 {
		t.Fatal("the reviewed unreadable vault stayed")
	}
}

func TestRecoveryRoutesRefuseAnotherRecord(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	signedIn := signUpTestAccount(t, ctx, server, baseURL, "review-routes@example.com", apitypes.RoleMember)
	defer signedIn.Lock()
	vault := requireValue[*session.Vault](t)(signedIn.CreateVault(ctx, "Reviewed", apitypes.VaultShared))
	records := requireValue[[]apitypes.Vault](t)(signedIn.Client.ListVaults(ctx))
	var record apitypes.Vault
	for _, candidate := range records {
		if candidate.ID == vault.ID {
			record = candidate
		}
	}
	other := apitypes.VaultReview{EncName: []byte("another name"), WrappedKey: record.WrappedKey, KeyGeneration: record.KeyGeneration}
	expectStatus(t, "a repair of another record", signedIn.Client.RepairVaultName(ctx, vault.ID, &apitypes.RepairVaultNameRequest{Review: other, EncName: record.EncName}), 409)
	expectStatus(t, "a deletion of another record", signedIn.Client.DeleteUnreadableVault(ctx, vault.ID, &apitypes.DeleteUnreadableVaultRequest{Review: other}), 409)
	other = apitypes.VaultReview{EncName: record.EncName, WrappedKey: record.WrappedKey, KeyGeneration: record.KeyGeneration + 1}
	expectStatus(t, "a deletion of another key generation", signedIn.Client.DeleteUnreadableVault(ctx, vault.ID, &apitypes.DeleteUnreadableVaultRequest{Review: other}), 409)
	stranger := signUpTestAccount(t, ctx, server, baseURL, "review-stranger@example.com", apitypes.RoleMember)
	defer stranger.Lock()
	reviewed := apitypes.VaultReview{EncName: record.EncName, WrappedKey: record.WrappedKey, KeyGeneration: record.KeyGeneration}
	expectStatus(t, "a stranger's deletion", stranger.Client.DeleteUnreadableVault(ctx, vault.ID, &apitypes.DeleteUnreadableVaultRequest{Review: reviewed}), 404)
	if err := signedIn.Client.DeleteUnreadableVault(ctx, vault.ID, &apitypes.DeleteUnreadableVaultRequest{Review: reviewed}); err != nil {
		t.Fatalf("deleting the reviewed record: %v", err)
	}
}

func TestNameLookupNeedsACompleteInventory(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	signedIn := signUpTestAccount(t, ctx, server, baseURL, "same-name@example.com", apitypes.RoleMember)
	defer signedIn.Lock()
	readable := requireValue[*session.Vault](t)(signedIn.CreateVault(ctx, "Same", apitypes.VaultShared))
	damaged := requireValue[*session.Vault](t)(signedIn.CreateVault(ctx, "Same", apitypes.VaultShared))
	if err := signedIn.Client.RenameVault(ctx, damaged.ID, &apitypes.RenameVaultRequest{EncName: []byte("bad-name"), KeyGeneration: damaged.KeyGeneration}); err != nil {
		t.Fatal(err)
	}
	if found, err := signedIn.FindVault(ctx, readable.ID); err != nil || found.ID != readable.ID {
		t.Fatalf("an exact ID stopped resolving: %+v %v", found, err)
	}
	if found, err := signedIn.FindVault(ctx, "Same"); !errors.Is(err, session.ErrIncompleteRead) {
		t.Fatalf("a name matching one readable vault was taken as unique beside an unreadable name: %+v %v", found, err)
	}
	if _, err := signedIn.FindVault(ctx, "Missing"); !errors.Is(err, session.ErrIncompleteRead) {
		t.Fatalf("a partial inventory proved absence: %v", err)
	}
}
