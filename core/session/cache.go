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
	ReplaceVaults(vaults []apitypes.Vault) error
	ListVaults() ([]apitypes.Vault, error)
	GetSeq(vaultID string) (int64, error)
	ApplyChanges(vaultID string, items []apitypes.Item, nextSeq int64) error
	ListItems(vaultID string) ([]apitypes.Item, error)
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

func (s *Session) listProtoVaults(ctx context.Context) ([]apitypes.Vault, error) {
	protoVaults, err := s.Client.ListVaults(ctx)
	if s.Cache == nil {
		return protoVaults, err
	}
	if err == nil {
		s.isOffline.Store(false)
		if cachedVaults, cacheErr := s.Cache.ListVaults(); cacheErr == nil {
			s.noteRemovedVaults(cachedVaults, protoVaults)
		}
		return protoVaults, s.Cache.ReplaceVaults(protoVaults)
	}
	if !apiclient.IsUnreachable(err) {
		return nil, err
	}
	s.isOffline.Store(true)
	return s.Cache.ListVaults()
}
