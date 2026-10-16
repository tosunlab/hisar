package app

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/tryhisar/hisar/core/browserprotocol"
	"github.com/tryhisar/hisar/core/item"
)

// listAddressSources returns the addresses the extension may fill a form with: address items with any value, then
// identities whose address is enough to fill one, each sorted by title. A card's billing address is not offered. Unlike
// logins, addresses belong to no website, so the extension asks only when the user picks one in the inline menu or the
// popup; the list carries no street or postal code.
func (s *Service) listAddressSources(ctx context.Context) ([]browserprotocol.AddressSummary, map[string]item.Address, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlockedWithoutActivity(); err != nil {
		return nil, nil, err
	}
	if err := s.loadItemsOfAllVault(ctx); err != nil {
		return nil, nil, err
	}
	summaries := []browserprotocol.AddressSummary{}
	addressesByKey := map[string]item.Address{}
	for vaultID, items := range s.itemsByVaultID {
		vault := s.vaultsByID[vaultID]
		if vault == nil {
			continue
		}
		for index := range items {
			vaultItem := &items[index]
			var address item.Address
			source := ""
			switch {
			case vaultItem.IsCategory(item.CategoryAddress):
				address, _ = item.ReadAddress(vaultItem)
				if !address.IsEmpty() {
					source = browserprotocol.AddressSourceAddress
				}
			case vaultItem.IsCategory(item.CategoryIdentity):
				identity, _ := item.ReadIdentity(vaultItem)
				if address = identity.Address; address.IsFillable() {
					source = browserprotocol.AddressSourceIdentity
				}
			}
			if source == "" {
				continue
			}
			address = address.Normalize()
			summary := strings.Trim(address.City+", "+address.Country, ", ")
			summaries = append(summaries, browserprotocol.AddressSummary{
				VaultID: vaultID, ItemID: vaultItem.ID, Title: vaultItem.Overview.Title, Summary: summary, Source: source, VaultName: vault.Name,
			})
			addressesByKey[vaultID+"/"+vaultItem.ID] = address
		}
	}
	slices.SortFunc(summaries, func(first, second browserprotocol.AddressSummary) int {
		// "address" sorts before "identity", so address items come first.
		return cmp.Or(cmp.Compare(first.Source, second.Source), strings.Compare(strings.ToLower(first.Title), strings.ToLower(second.Title)), strings.Compare(first.ItemID, second.ItemID))
	})
	return summaries, addressesByKey, nil
}

// getAddressForFill returns an address listAddressSources offers; any other item, such as a card or an identity without
// an address, is not found.
func (s *Service) getAddressForFill(ctx context.Context, vaultID, itemID string) (browserprotocol.AddressFillResult, error) {
	_, addressesByKey, err := s.listAddressSources(ctx)
	if err != nil {
		return browserprotocol.AddressFillResult{}, err
	}
	address, found := addressesByKey[vaultID+"/"+itemID]
	if !found {
		return browserprotocol.AddressFillResult{}, ErrNotFound
	}
	return browserprotocol.AddressFillResult{Street: address.Street, City: address.City, Region: address.Region, PostalCode: address.PostalCode, Country: address.Country}, nil
}
