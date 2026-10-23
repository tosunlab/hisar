package session

import (
	"context"
	"net/http"
	"testing"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/item"
)

func TestR08AccessRefusalNeverBecomesAnEmptyCompletedSnapshot(t *testing.T) {
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
	if _, err := unlocked.SaveItem(ctx, &vaults[0], &item.Item{Overview: item.Overview{Title: "Published before refusal"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := unlocked.ListItems(ctx, &vaults[0]); err != nil {
		t.Fatal(err)
	}
	before, err := c.GetCheckpoint(vaults[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	account.apiClient.HTTP.Transport = r08Transport(func(_ *http.Request) (*http.Response, error) {
		return r08Response(404, apitypes.Error{Error: "access removed"}), nil
	})
	_, err = unlocked.ReadItems(ctx, &vaults[0])
	if !apiclient.IsStatus(err, 404) {
		t.Fatalf("access refusal silently fell back to a rescue read: %v", err)
	}
	records, err := c.ListItems(vaults[0].ID)
	if err != nil || len(records) != 1 {
		t.Fatalf("failed access acquisition destroyed rescue ciphertext: %+v %v", records, err)
	}
	after, err := c.GetCheckpoint(vaults[0].ID)
	if err != nil || after != before {
		t.Fatalf("access refusal published absence: %+v %v", after, err)
	}
}
