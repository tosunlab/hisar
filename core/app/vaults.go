package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/session"
)

func (s *Service) loadAllVault(ctx context.Context) error {
	allVault, err := s.unlockedSession.ListAllVault(ctx)
	if err != nil {
		return convertError(err)
	}
	for _, vault := range s.allVaultByID {
		clear(vault.Key)
	}
	s.allVaultByID = map[string]*session.Vault{}
	for index := range allVault {
		s.allVaultByID[allVault[index].ID] = &allVault[index]
	}
	return nil
}

func (s *Service) ListAllVault() ([]VaultSummary, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.loadAllVault(ctx); err != nil {
		return nil, err
	}
	allSummary := make([]VaultSummary, 0, len(s.allVaultByID))
	for _, vault := range s.allVaultByID {
		allItem, err := s.unlockedSession.ListAllItem(ctx, vault)
		if err != nil {
			return nil, convertError(err)
		}
		s.allItemByVaultID[vault.ID] = allItem
		allSummary = append(allSummary, convertToVaultSummary(vault, len(allItem)))
	}
	sortAllVaultSummary(allSummary)
	return allSummary, nil
}

func convertToVaultSummary(vault *session.Vault, itemCount int) VaultSummary {
	return VaultSummary{
		ID:        vault.ID,
		Name:      vault.Name,
		Type:      vault.Type,
		Role:      vault.Role,
		ItemCount: itemCount,
	}
}

// sortAllVaultSummary puts personal vaults first, then sorts by name.
func sortAllVaultSummary(allSummary []VaultSummary) {
	slices.SortFunc(allSummary, func(first, second VaultSummary) int {
		if (first.Type == apitypes.VaultPersonal) != (second.Type == apitypes.VaultPersonal) {
			if first.Type == apitypes.VaultPersonal {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(first.Name), strings.ToLower(second.Name))
	})
}

func (s *Service) CreateVault(name string) (VaultSummary, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return VaultSummary{}, err
	}
	if strings.TrimSpace(name) == "" {
		return VaultSummary{}, errors.New("enter a vault name")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vault, createErr := s.unlockedSession.CreateVault(ctx, strings.TrimSpace(name), apitypes.VaultShared)
	if err := s.state.Save(); createErr == nil && err != nil {
		return VaultSummary{}, err
	}
	if createErr != nil {
		return VaultSummary{}, convertError(createErr)
	}
	if s.allVaultByID == nil {
		s.allVaultByID = map[string]*session.Vault{}
	}
	s.allVaultByID[vault.ID] = vault
	return convertToVaultSummary(vault, 0), nil
}

func (s *Service) findVault(ctx context.Context, vaultID string) (*session.Vault, error) {
	if vault, found := s.allVaultByID[vaultID]; found {
		return vault, nil
	}
	if err := s.loadAllVault(ctx); err != nil {
		return nil, err
	}
	if vault, found := s.allVaultByID[vaultID]; found {
		return vault, nil
	}
	return nil, fmt.Errorf("vault %w", ErrNotFound)
}
