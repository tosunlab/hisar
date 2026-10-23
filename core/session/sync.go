package session

import (
	"context"
	"errors"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/snapshot"
)

var ErrSyncProtocol = errors.New("update the Hisar server to provide generation-bound sync pages")

// forEachChangePage calls handle with every page of a vault's changes after since, and returns the last page's NextSeq.
// A page that announces more changes must carry some and move NextSeq past since, or the loop would never end.
func (s *Session) forEachChangePage(ctx context.Context, vaultID string, since int64, handle func(page *apitypes.ChangesResponse) error, generation ...int64) (int64, error) {
	for {
		page, err := s.Client.ListChanges(ctx, vaultID, since, 0)
		if err != nil {
			return since, err
		}
		if len(generation) != 0 {
			if page.KeyGeneration == 0 {
				return since, ErrSyncProtocol
			}
			if page.KeyGeneration != max(int64(1), generation[0]) {
				return since, ErrKeyChanged
			}
		}
		switch {
		case page.NextSeq < since:
			return since, ErrRollback
		case page.More && (page.NextSeq == since || len(page.Items) == 0):
			return since, ErrChangesStalled
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
	}, vault.KeyGeneration)
	if err != nil {
		return nil, err
	}
	return protoItems, nil
}

// downloadItems downloads the live items of a vault, without a cache. The acquisition is complete up to the sequence
// its last page reached.
func (s *Session) downloadItems(ctx context.Context, vault *Vault) (acquisition, error) {
	protoItemsByID := map[string]apitypes.Item{}
	nextSeq, err := s.forEachChangePage(ctx, vault.ID, 0, func(page *apitypes.ChangesResponse) error {
		for _, protoItem := range page.Items {
			protoItemsByID[protoItem.ID] = protoItem
		}
		return nil
	}, vault.KeyGeneration)
	if err != nil {
		return acquisition{}, err
	}
	if err := s.recordSeq(vault.ID, nextSeq); err != nil {
		return acquisition{}, err
	}
	acquired := acquisition{records: make([]apitypes.Item, 0, len(protoItemsByID)), provenance: ReadProvenance{Source: "online", Checkpoint: snapshot.Checkpoint{
		HasCompletedSync: true, CompletedSequence: nextSeq, CompletedAt: time.Now().UTC().Format(time.RFC3339Nano), KeyGeneration: vault.KeyGeneration,
	}}}
	for _, protoItem := range protoItemsByID {
		if !protoItem.Deleted {
			acquired.records = append(acquired.records, protoItem)
		}
	}
	return acquired, nil
}

// syncVaultSnapshot downloads the changes since the published snapshot, or every change when reset, and publishes them
// with the vault's record only once the last page arrived. While the server is unreachable it returns the published
// snapshot as it is.
func (s *Session) syncVaultSnapshot(ctx context.Context, vault *Vault, reset bool) (acquisition, error) {
	checkpoint, err := s.Cache.GetCheckpoint(vault.ID)
	if err != nil {
		return acquisition{}, err
	}
	previousSeq, err := s.Cache.GetSeq(vault.ID)
	if err != nil {
		return acquisition{}, err
	}
	since := checkpoint.CompletedSequence
	reset = reset || !checkpoint.HasCompletedSync || checkpoint.KeyGeneration != vault.KeyGeneration
	if reset {
		since = 0
	}
	staged, err := s.Cache.BeginSync(vault.ID, reset)
	if err != nil {
		return acquisition{}, err
	}
	defer staged.Rollback()
	since, err = s.forEachChangePage(ctx, vault.ID, since, func(page *apitypes.ChangesResponse) error {
		return staged.Apply(page.Items)
	}, vault.KeyGeneration)
	if apiclient.IsUnreachable(err) {
		staged.Rollback()
		s.isOffline.Store(true)
		return s.readPublishedSnapshot(vault, "cached")
	}
	if err != nil {
		return acquisition{}, err
	}
	if since < previousSeq {
		return acquisition{}, ErrRollback
	}
	if err := s.recordSeq(vault.ID, since); err != nil {
		return acquisition{}, err
	}
	if err := staged.Commit(since, &vault.record); err != nil {
		return acquisition{}, err
	}
	s.isOffline.Store(false)
	return s.readPublishedSnapshot(vault, "online")
}

// readPublishedSnapshot returns the published items of vault. A cached snapshot published under another key generation
// than vault's opens with the cached record published with it, since that is the key its items are sealed with.
func (s *Session) readPublishedSnapshot(vault *Vault, source string) (acquisition, error) {
	records, err := s.Cache.ListItems(vault.ID)
	if err != nil {
		return acquisition{}, err
	}
	checkpoint, err := s.Cache.GetCheckpoint(vault.ID)
	if err != nil {
		return acquisition{}, err
	}
	acquired := acquisition{records: records, provenance: ReadProvenance{Source: source, Checkpoint: checkpoint}}
	if source == "cached" && checkpoint.HasCompletedSync && checkpoint.KeyGeneration != vault.KeyGeneration {
		cachedVaults, err := s.Cache.ListVaults()
		if err != nil {
			return acquisition{}, err
		}
		for _, record := range cachedVaults {
			if record.ID == vault.ID && record.KeyGeneration == checkpoint.KeyGeneration {
				acquired.sealedWith, _ = s.openVault(record)
			}
		}
	}
	return acquired, nil
}
