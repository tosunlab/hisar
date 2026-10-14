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

func (s *Service) loadVaults(ctx context.Context) error {
	vaults, err := s.unlockedSession.ListVaults(ctx)
	if err != nil {
		return convertError(err)
	}
	for _, vault := range s.vaultsByID {
		clear(vault.Key)
	}
	s.vaultsByID = map[string]*session.Vault{}
	for index := range vaults {
		s.vaultsByID[vaults[index].ID] = &vaults[index]
	}
	return nil
}

func (s *Service) ListVaults() ([]VaultSummary, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.loadVaults(ctx); err != nil {
		return nil, err
	}
	summaries := make([]VaultSummary, 0, len(s.vaultsByID))
	for _, vault := range s.vaultsByID {
		items, err := s.unlockedSession.ListItems(ctx, vault)
		if err != nil {
			return nil, convertError(err)
		}
		s.itemsByVaultID[vault.ID] = items
		summaries = append(summaries, convertToVaultSummary(vault, len(items)))
	}
	sortVaultSummaries(summaries)
	return summaries, nil
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

// sortVaultSummaries puts personal vaults first, then sorts by name.
func sortVaultSummaries(summaries []VaultSummary) {
	slices.SortFunc(summaries, func(first, second VaultSummary) int {
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
	if s.vaultsByID == nil {
		s.vaultsByID = map[string]*session.Vault{}
	}
	s.vaultsByID[vault.ID] = vault
	return convertToVaultSummary(vault, 0), nil
}

func (s *Service) findVault(ctx context.Context, vaultID string) (*session.Vault, error) {
	if vault, found := s.vaultsByID[vaultID]; found {
		return vault, nil
	}
	if err := s.loadVaults(ctx); err != nil {
		return nil, err
	}
	if vault, found := s.vaultsByID[vaultID]; found {
		return vault, nil
	}
	return nil, fmt.Errorf("vault %w", ErrNotFound)
}

func (s *Service) findPersonalVaultID(ctx context.Context) (string, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlockedWithoutActivity(); err != nil {
		return "", err
	}
	if err := s.loadItemsOfAllVault(ctx); err != nil {
		return "", err
	}
	for vaultID, vault := range s.vaultsByID {
		if vault.Type == apitypes.VaultPersonal && vault.Role != apitypes.VaultReader {
			return vaultID, nil
		}
	}
	return "", fmt.Errorf("personal vault %w", ErrNotFound)
}
