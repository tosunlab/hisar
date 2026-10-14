package session

import (
	"context"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
)

// forEachChangePage calls handle with every page of a vault's changes after since, and returns the last page's NextSeq.
func (s *Session) forEachChangePage(ctx context.Context, vaultID string, since int64, handle func(page *apitypes.ChangesResponse) error) (int64, error) {
	for {
		page, err := s.Client.ListChanges(ctx, vaultID, since, 0)
		if err != nil {
			return since, err
		}
		if err := handle(page); err != nil {
			return since, err
		}
		since = page.NextSeq
		if !page.More {
			return since, nil
		}
	}
}

// recordSeq refuses a seq older than the highest one seen for the vault, which would mean the server rolled back, then remembers it.
func (s *Session) recordSeq(vaultID string, seq int64) error {
	if s.SeqStore == nil {
		return nil
	}
	if seq < s.SeqStore.GetSeq(vaultID) {
		return ErrRollback
	}
	s.SeqStore.SetSeq(vaultID, seq)
	return nil
}

// listProtoItems downloads every record of a vault, deleted ones included.
func (s *Session) listProtoItems(ctx context.Context, vault *Vault) ([]apitypes.Item, error) {
	var protoItems []apitypes.Item
	_, err := s.forEachChangePage(ctx, vault.ID, 0, func(page *apitypes.ChangesResponse) error {
		protoItems = append(protoItems, page.Items...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return protoItems, nil
}

// downloadItems downloads the live items of a vault, without a cache.
func (s *Session) downloadItems(ctx context.Context, vault *Vault) ([]apitypes.Item, error) {
	protoItemsByID := map[string]apitypes.Item{}
	nextSeq, err := s.forEachChangePage(ctx, vault.ID, 0, func(page *apitypes.ChangesResponse) error {
		for _, protoItem := range page.Items {
			protoItemsByID[protoItem.ID] = protoItem
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := s.recordSeq(vault.ID, nextSeq); err != nil {
		return nil, err
	}
	protoItems := make([]apitypes.Item, 0, len(protoItemsByID))
	for _, protoItem := range protoItemsByID {
		if !protoItem.Deleted {
			protoItems = append(protoItems, protoItem)
		}
	}
	return protoItems, nil
}

// syncVault downloads the changes since the cached seq and returns the vault's cached items.
func (s *Session) syncVault(ctx context.Context, vault *Vault) ([]apitypes.Item, error) {
	since, err := s.Cache.GetSeq(vault.ID)
	if err != nil {
		return nil, err
	}
	since, err = s.forEachChangePage(ctx, vault.ID, since, func(page *apitypes.ChangesResponse) error {
		if page.NextSeq < since {
			return ErrRollback
		}
		since = page.NextSeq
		return s.Cache.ApplyChanges(vault.ID, page.Items, page.NextSeq)
	})
	if apiclient.IsUnreachable(err) {
		s.isOffline.Store(true)
		return s.Cache.ListItems(vault.ID)
	}
	if err != nil {
		return nil, err
	}
	if err := s.recordSeq(vault.ID, since); err != nil {
		return nil, err
	}
	s.isOffline.Store(false)
	return s.Cache.ListItems(vault.ID)
}
