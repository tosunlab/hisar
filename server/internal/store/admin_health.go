package store

import (
	"context"
	"time"
)

// itemCountMaxAge is how long GetInstanceCounts reuses a count of live items, which reads the whole items table.
const itemCountMaxAge = time.Minute

// InstanceCounts is what the server holds, for the admin dashboard's Overview.
type InstanceCounts struct {
	Accounts       int64
	Vaults         int64
	Items          int64
	ShareLinks     int64
	PendingInvites int64
	// StorageBytes is the size of the whole database on disk.
	StorageBytes int64
}

// itemCountCache keeps the last count of live items. Its turn lets one count run at a time: callers that arrive during
// a count wait for it and reuse it.
type itemCountCache struct {
	turn chan struct{}
	// startedAt is when the count in items began; zero before the first count.
	startedAt time.Time
	items     int64
}

func newItemCountCache() *itemCountCache {
	return &itemCountCache{turn: make(chan struct{}, 1)}
}

// get returns the last count when it began less than itemCountMaxAge ago, and calls count otherwise.
func (cache *itemCountCache) get(ctx context.Context, count func(ctx context.Context) (int64, error)) (int64, error) {
	select {
	case cache.turn <- struct{}{}:
	case <-ctx.Done():
		return 0, ctx.Err()
	}
	defer func() { <-cache.turn }()
	startedAt := time.Now()
	if !cache.startedAt.IsZero() && startedAt.Sub(cache.startedAt) < itemCountMaxAge {
		return cache.items, nil
	}
	items, err := count(ctx)
	if err != nil {
		return 0, err
	}
	cache.startedAt, cache.items = startedAt, items
	return items, nil
}

// GetInstanceCounts counts live items, share links that have not expired and sign-up invites that are neither used nor
// expired. The item count may be up to itemCountMaxAge old.
func (s *Store) GetInstanceCounts(ctx context.Context) (InstanceCounts, error) {
	var counts InstanceCounts
	err := s.pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM accounts),
		       (SELECT count(*) FROM vaults),
		       (SELECT count(*) FROM share_links WHERE expires_at > now()),
		       (SELECT count(*) FROM invites WHERE used_at IS NULL AND expires_at > now()),
		       pg_database_size(current_database())`).
		Scan(&counts.Accounts, &counts.Vaults, &counts.ShareLinks, &counts.PendingInvites, &counts.StorageBytes)
	if err != nil {
		return InstanceCounts{}, err
	}
	counts.Items, err = s.liveItemCount.get(ctx, s.countLiveItems)
	if err != nil {
		return InstanceCounts{}, err
	}
	return counts, nil
}

func (s *Store) countLiveItems(ctx context.Context) (int64, error) {
	var items int64
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM items WHERE NOT deleted`).Scan(&items)
	return items, err
}
