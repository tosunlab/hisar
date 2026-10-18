package app

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/tryhisar/hisar/core/browserprotocol"
	"github.com/tryhisar/hisar/core/item"
)

// listCardSources returns the credit cards the extension may fill a payment form with, sorted by title. Like addresses,
// cards belong to no website, so the extension asks only when the user picks one in the inline menu or the popup; the
// list carries only the brand, last four digits and expiry.
func (s *Service) listCardSources(ctx context.Context) ([]browserprotocol.CardSummary, map[string]item.Card, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlockedWithoutActivity(); err != nil {
		return nil, nil, err
	}
	if err := s.loadItemsOfAllVault(ctx); err != nil {
		return nil, nil, err
	}
	summaries := []browserprotocol.CardSummary{}
	cardsByKey := map[string]item.Card{}
	for vaultID, items := range s.itemsByVaultID {
		vault := s.vaultsByID[vaultID]
		if vault == nil {
			continue
		}
		for index := range items {
			vaultItem := &items[index]
			if !vaultItem.IsCategory(item.CategoryCreditCard) {
				continue
			}
			card, _ := item.ReadCard(vaultItem)
			if getCardNumberDigits(card.Number) == "" {
				continue
			}
			expiry, _ := item.NormalizeExpiry(card.Expiry)
			summaries = append(summaries, browserprotocol.CardSummary{
				VaultID: vaultID, ItemID: vaultItem.ID, Title: vaultItem.Overview.Title, Brand: item.GetCardBrand(card.Number),
				Last4: item.GetCardLast4(card.Number), Expiry: expiry, VaultName: vault.Name,
			})
			cardsByKey[vaultID+"/"+vaultItem.ID] = card
		}
	}
	slices.SortFunc(summaries, func(first, second browserprotocol.CardSummary) int {
		return cmp.Or(strings.Compare(strings.ToLower(first.Title), strings.ToLower(second.Title)), strings.Compare(first.ItemID, second.ItemID))
	})
	return summaries, cardsByKey, nil
}

// getCardForFill returns a card listCardSources offers; any other item is not found.
func (s *Service) getCardForFill(ctx context.Context, vaultID, itemID string) (browserprotocol.CardFillResult, error) {
	_, cardsByKey, err := s.listCardSources(ctx)
	if err != nil {
		return browserprotocol.CardFillResult{}, err
	}
	card, found := cardsByKey[vaultID+"/"+itemID]
	if !found {
		return browserprotocol.CardFillResult{}, ErrNotFound
	}
	result := browserprotocol.CardFillResult{
		Cardholder: card.Cardholder, Number: getCardNumberDigits(card.Number), SecurityCode: card.SecurityCode, Brand: item.GetCardBrand(card.Number),
	}
	if expiry, isValid := item.NormalizeExpiry(card.Expiry); isValid {
		result.ExpiryMonth, result.ExpiryYear = expiry[:2], "20"+expiry[3:]
	}
	return result, nil
}

// getCardNumberDigits returns the digits of a card number saved with spaces or dashes, and "" for anything else.
func getCardNumberDigits(number string) string {
	digits := strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(number))
	if digits == "" || strings.Trim(digits, "0123456789") != "" {
		return ""
	}
	return digits
}
