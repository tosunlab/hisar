package store

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
)

func TestGetInstanceCounts(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "ALICE1", "alice@example.com")
	createTestAccount(t, database, "BOB123", "bob@example.com")
	vaultID := createTestVault(t, database, "ALICE1")
	createTestVault(t, database, "BOB123")
	for range 3 {
		if _, err := database.PutItem(ctx, vaultID, "", 0, &Item{ID: uuid.NewString(), Revision: uuid.NewString(), Overview: []byte{1}, Details: []byte{1}, UpdatedBy: "ALICE1"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.pool.Exec(ctx, `UPDATE items SET deleted = true WHERE id = (SELECT id FROM items LIMIT 1)`); err != nil {
		t.Fatal(err)
	}
	createTestShareLink(t, database, "live", "ALICE1", 0, time.Hour)
	createTestShareLink(t, database, "expired", "ALICE1", 0, time.Hour)
	if _, err := database.pool.Exec(ctx, `UPDATE share_links SET expires_at = now() - interval '1 second' WHERE id = 'expired'`); err != nil {
		t.Fatal(err)
	}
	if err := database.CreateInvite(ctx, []byte("pending"), "carol@example.com", "member", nil, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := database.CreateInvite(ctx, []byte("expired"), "dave@example.com", "member", nil, -time.Minute); err != nil {
		t.Fatal(err)
	}

	counts, err := database.GetInstanceCounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if counts.StorageBytes <= 0 {
		t.Errorf("storage bytes %d", counts.StorageBytes)
	}
	counts.StorageBytes = 0
	if expected := (InstanceCounts{Accounts: 2, Vaults: 2, Items: 2, ShareLinks: 1, PendingInvites: 1}); counts != expected {
		t.Errorf("counts %+v, expected %+v", counts, expected)
	}

	if _, err := database.pool.Exec(ctx, `UPDATE items SET deleted = true WHERE id = (SELECT id FROM items WHERE NOT deleted LIMIT 1)`); err != nil {
		t.Fatal(err)
	}
	if counts, err := database.GetInstanceCounts(ctx); err != nil || counts.Items != 2 {
		t.Errorf("items %d within a minute of the last count, expected 2 (%v)", counts.Items, err)
	}
	database.liveItemCount.startedAt = database.liveItemCount.startedAt.Add(-itemCountMaxAge)
	if counts, err := database.GetInstanceCounts(ctx); err != nil || counts.Items != 1 {
		t.Errorf("items %d a minute after the last count, expected 1 (%v)", counts.Items, err)
	}
}

func TestItemCountCacheCountsOnceAtATime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := newItemCountCache()
		release := make(chan struct{})
		var countCalls atomic.Int64
		count := func(context.Context) (int64, error) {
			countCalls.Add(1)
			<-release
			return 7, nil
		}
		var running sync.WaitGroup
		for range 5 {
			running.Go(func() {
				if items, err := cache.get(t.Context(), count); err != nil || items != 7 {
					t.Errorf("items %d, %v", items, err)
				}
			})
		}
		synctest.Wait()
		close(release)
		running.Wait()
		if calls := countCalls.Load(); calls != 1 {
			t.Errorf("%d counts for 5 callers at once, expected 1", calls)
		}
	})
}

func TestItemCountCacheReusesACountForAMinute(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := newItemCountCache()
		var countCalls int64
		count := func(context.Context) (int64, error) {
			countCalls++
			return countCalls, nil
		}
		for _, step := range []struct {
			wait     time.Duration
			expected int64
		}{{0, 1}, {59 * time.Second, 1}, {time.Second, 2}, {30 * time.Second, 2}} {
			time.Sleep(step.wait)
			if items, err := cache.get(t.Context(), count); err != nil || items != step.expected {
				t.Errorf("after %s more: items %d, expected %d (%v)", step.wait, items, step.expected, err)
			}
		}
	})
}

func TestItemCountCacheCountsAgainAfterAnError(t *testing.T) {
	cache := newItemCountCache()
	if _, err := cache.get(t.Context(), func(context.Context) (int64, error) { return 0, errors.New("database down") }); err == nil {
		t.Fatal("the error was not returned")
	}
	if items, err := cache.get(t.Context(), func(context.Context) (int64, error) { return 3, nil }); err != nil || items != 3 {
		t.Errorf("items %d, %v", items, err)
	}
}
