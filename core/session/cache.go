package session

import (
	"context"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
)

// Cache keeps the server's encrypted records so a session can unlock and read while the server is unreachable.
type Cache interface {
	SaveAccount(account *apitypes.Account) error
	LoadAccount(accountID string) (*apitypes.Account, error)
	ReplaceAllVault(allVault []apitypes.Vault) error
	ListAllVault() ([]apitypes.Vault, error)
	GetSeq(vaultID string) (int64, error)
	ApplyAllChange(vaultID string, allItem []apitypes.Item, nextSeq int64) error
	ListAllItem(vaultID string) ([]apitypes.Item, error)
	ResetVault(vaultID string) error
}

// IsOffline reports whether the last read was served from the cache because the server was unreachable.
func (s *Session) IsOffline() bool {
	return s.isOffline.Load()
}

// AttachCache starts caching a session created by SignUp or LogIn, including the account needed to unlock offline.
func (s *Session) AttachCache(ctx context.Context, accountCache Cache) error {
	s.Cache = accountCache
	account, err := s.Client.GetAccount(ctx)
	if err != nil {
		return err
	}
	return accountCache.SaveAccount(account)
}

// getAccount falls back to the cached account only when the server cannot be reached.
func getAccount(ctx context.Context, apiClient *apiclient.Client, accountCache Cache, accountID string) (account *apitypes.Account, isOffline bool, err error) {
	account, err = apiClient.GetAccount(ctx)
	if err == nil {
		if accountCache != nil && account.AccountID == accountID {
			accountCache.SaveAccount(account)
		}
		return account, false, nil
	}
	if accountCache == nil || !apiclient.IsUnreachable(err) {
		return nil, false, err
	}
	cachedAccount, cacheErr := accountCache.LoadAccount(accountID)
	if cacheErr != nil || cachedAccount == nil {
		return nil, false, err
	}
	return cachedAccount, true, nil
}

func (s *Session) listAllProtoVault(ctx context.Context) ([]apitypes.Vault, error) {
	allProtoVault, err := s.Client.ListAllVault(ctx)
	if s.Cache == nil {
		return allProtoVault, err
	}
	if err == nil {
		s.isOffline.Store(false)
		if allCachedVault, cacheErr := s.Cache.ListAllVault(); cacheErr == nil {
			s.noteAllRemovedVault(allCachedVault, allProtoVault)
		}
		return allProtoVault, s.Cache.ReplaceAllVault(allProtoVault)
	}
	if !apiclient.IsUnreachable(err) {
		return nil, err
	}
	s.isOffline.Store(true)
	return s.Cache.ListAllVault()
}

// syncVault downloads the changes since the cached seq and returns the vault's cached items.
func (s *Session) syncVault(ctx context.Context, vault *Vault) ([]apitypes.Item, error) {
	since, err := s.Cache.GetSeq(vault.ID)
	if err != nil {
		return nil, err
	}
	for {
		changeResponse, err := s.Client.ListAllChange(ctx, vault.ID, since, 0)
		if apiclient.IsUnreachable(err) {
			s.isOffline.Store(true)
			return s.Cache.ListAllItem(vault.ID)
		}
		if err != nil {
			return nil, err
		}
		if changeResponse.NextSeq < since {
			return nil, ErrRollback
		}
		if err := s.Cache.ApplyAllChange(vault.ID, changeResponse.Items, changeResponse.NextSeq); err != nil {
			return nil, err
		}
		since = changeResponse.NextSeq
		if !changeResponse.More {
			break
		}
	}
	if s.SeqStore != nil {
		if since < s.SeqStore.GetSeq(vault.ID) {
			return nil, ErrRollback
		}
		s.SeqStore.SetSeq(vault.ID, since)
	}
	s.isOffline.Store(false)
	return s.Cache.ListAllItem(vault.ID)
}
