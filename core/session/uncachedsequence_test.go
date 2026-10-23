package session

import (
	"context"
	"testing"

	"github.com/tryhisar/hisar/core/item"
)

func TestR08UncachedReadReportsItsActualCompletedSequence(t *testing.T) {
	account := createOfflineTestAccount(t)
	ctx := context.Background()
	unlocked, err := Unlock(ctx, account.apiClient, nil, account.password, account.secretKey)
	if err != nil {
		t.Fatal(err)
	}
	defer unlocked.Lock()
	inventory, err := unlocked.ReadVaults(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unlocked.SaveItem(ctx, &inventory.Vaults[0], &item.Item{Overview: item.Overview{Title: "Online item"}}); err != nil {
		t.Fatal(err)
	}
	result, err := unlocked.ReadItems(ctx, &inventory.Vaults[0])
	if err != nil || len(result.Items) != 1 || result.Provenance.CompletedSequence != 1 {
		t.Fatalf("standalone acquisition exported a fabricated sequence-zero checkpoint: %+v %v", result, err)
	}
}
