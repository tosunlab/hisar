package session

import (
	"bytes"
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
	SaveAccountPreferences(accountID string, preferences *apitypes.AccountPreferences) error
	LoadAccountPreferences(accountID string) (*apitypes.AccountPreferences, error)
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
	verified := s.verifiedAccount
	if verified == nil || account.AccountID != s.AccountID ||
		!bytes.Equal(account.EncKeyset, verified.EncKeyset) ||
		!bytes.Equal(account.Salt, verified.Salt) || account.Argon2 != verified.Argon2 {
		return ErrKeyMismatch
	}
	publicKeys := s.Keyset.GetPublicKeys(s.AccountID)
	if !bytes.Equal(account.PublicKeys.KEM, publicKeys.KEM) || !bytes.Equal(account.PublicKeys.Sign, publicKeys.Sign) {
		return ErrKeyMismatch
	}
	return accountCache.SaveAccount(account)
}

// getAccount falls back to the cached account only when the server cannot be reached. The caller caches a fetched
// account once its keyset has opened, so a bad answer never replaces the copy that unlocks offline.
func getAccount(ctx context.Context, apiClient *apiclient.Client, accountCache Cache, accountID string) (account *apitypes.Account, isOffline bool, err error) {
	account, err = apiClient.GetAccount(ctx)
	if err == nil {
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

// listProtoVaults returns the server's vault list, or the cached one while the server is unreachable; isFetched says
// which, so the caller replaces the cache only once every fetched vault has opened.
func (s *Session) listProtoVaults(ctx context.Context) (protoVaults []apitypes.Vault, isFetched bool, err error) {
	protoVaults, err = s.Client.ListVaults(ctx)
	if err == nil {
		if s.Cache != nil {
			s.isOffline.Store(false)
		}
		return protoVaults, true, nil
	}
	if s.Cache == nil || !apiclient.IsUnreachable(err) {
		return nil, false, err
	}
	s.isOffline.Store(true)
	protoVaults, err = s.Cache.ListVaults()
	return protoVaults, false, err
}

func (s *Session) replaceCachedVaults(protoVaults []apitypes.Vault) error {
	if cachedVaults, err := s.Cache.ListVaults(); err == nil {
		s.noteRemovedVaults(cachedVaults, protoVaults)
	}
	return s.Cache.ReplaceVaults(protoVaults)
}
