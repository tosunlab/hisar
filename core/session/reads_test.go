package session

import (
	"context"
	"errors"
	"testing"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
)

func TestUnreadableItemDoesNotHideHealthyRecords(t *testing.T) {
	account := createOfflineTestAccount(t)
	ctx := context.Background()
	c := openTestCache(t)
	unlocked, err := Unlock(ctx, account.apiClient, c, account.password, account.secretKey)
	if err != nil {
		t.Fatal(err)
	}
	defer unlocked.Lock()
	vaults, err := unlocked.ListVaults(ctx)
	if err != nil {
		t.Fatal(err)
	}
	healthy := &item.Item{Overview: item.Overview{Title: "Healthy"}}
	if _, err := unlocked.SaveItem(ctx, &vaults[0], healthy); err != nil {
		t.Fatal(err)
	}
	account.server.mutex.Lock()
	account.server.seq++
	account.server.itemsByID["bad"] = apitypes.Item{ID: "bad", Revision: "bad-r1", Seq: account.server.seq, Overview: []byte("bad"), Details: []byte("bad")}
	account.server.mutex.Unlock()
	result, err := unlocked.ReadItems(ctx, &vaults[0])
	if err != nil || len(result.Items) != 1 || result.Items[0].ID != healthy.ID || len(result.Issues) != 1 || result.Issues[0].ItemID != "bad" || result.Issues[0].Revision != "bad-r1" {
		t.Fatalf("partial readable result: %+v %v", result, err)
	}
	if _, err := unlocked.ListItems(ctx, &vaults[0]); !errors.Is(err, ErrIncompleteRead) {
		t.Fatalf("strict consumer inferred completeness: %v", err)
	}
	account.server.setUnreachable(true)
	result, err = unlocked.ReadItems(ctx, &vaults[0])
	if err != nil || len(result.Items) != 1 || len(result.Issues) != 1 || result.Provenance.Source != "cached" {
		t.Fatalf("offline evidence missing: %+v %v", result, err)
	}
}

func TestUnreadableVaultDoesNotDisableHealthyInventory(t *testing.T) {
	account := createOfflineTestAccount(t)
	ctx := context.Background()
	unlocked, err := Unlock(ctx, account.apiClient, openTestCache(t), account.password, account.secretKey)
	if err != nil {
		t.Fatal(err)
	}
	defer unlocked.Lock()
	account.server.mutex.Lock()
	account.server.vaults = append(account.server.vaults, apitypes.Vault{ID: "bad-vault", WrappedKey: []byte("bad")})
	account.server.vaults[0].EncName = []byte("bad-name")
	account.server.mutex.Unlock()
	result, err := unlocked.ReadVaults(ctx)
	if err != nil || len(result.Vaults) != 1 || !result.Vaults[0].NameUnreadable || result.Vaults[0].Name != "" || len(result.Issues) != 2 {
		t.Fatalf("fault isolation: %+v %v", result, err)
	}
	if _, err := unlocked.ListVaults(ctx); !errors.Is(err, ErrIncompleteRead) {
		t.Fatalf("strict inventory lost issues: %v", err)
	}
}

func TestRotationRaceReloadsKeyBeforePublishing(t *testing.T) {
	account := createOfflineTestAccount(t)
	ctx := context.Background()
	c := openTestCache(t)
	unlocked, err := Unlock(ctx, account.apiClient, c, account.password, account.secretKey)
	if err != nil {
		t.Fatal(err)
	}
	defer unlocked.Lock()
	vaults, err := unlocked.ListVaults(ctx)
	if err != nil {
		t.Fatal(err)
	}
	vault := &vaults[0]
	next := *vault
	next.Key = crypt.GenerateKey()
	next.KeyGeneration = 2
	record, err := unlocked.sealItem(&next, &item.Item{ID: "rotated", Overview: item.Overview{Title: "Still here"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	wrapped, err := crypt.WrapVaultKey(unlocked.Keyset.GetKEMPublicKey(), vault.ID, unlocked.AccountID, next.Key)
	if err != nil {
		t.Fatal(err)
	}
	name, err := crypt.SealVaultName(next.Key, vault.ID, []byte("Family"))
	if err != nil {
		t.Fatal(err)
	}
	account.server.mutex.Lock()
	account.server.vaults[0].WrappedKey = wrapped
	account.server.vaults[0].EncName = name
	account.server.vaults[0].KeyGeneration = 2
	account.server.seq = 1
	account.server.itemsByID["rotated"] = apitypes.Item{ID: "rotated", Revision: record.Revision, Seq: 1, Overview: record.Overview, Details: record.Details}
	account.server.mutex.Unlock()
	result, err := unlocked.ReadItems(ctx, vault)
	if err != nil || len(result.Issues) != 0 || len(result.Items) != 1 || result.Items[0].Overview.Title != "Still here" || vault.KeyGeneration != 2 {
		t.Fatalf("rotation race lost records: %+v %v", result, err)
	}
}
