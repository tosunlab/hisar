package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/secretreference"
)

// serviceSecretSource lets references resolve against the unlocked vaults; the caller holds s.mutex.
// isCacheOnly uses what the app already loaded, so describing a request for the approval window stays quick.
type serviceSecretSource struct {
	service     *Service
	ctx         context.Context
	isCacheOnly bool
}

func (source serviceSecretSource) ListVaults() ([]secretreference.Vault, error) {
	s := source.service
	if !source.isCacheOnly || len(s.vaultsByID) == 0 {
		if err := s.loadVaults(source.ctx); err != nil {
			return nil, err
		}
	}
	vaults := make([]secretreference.Vault, 0, len(s.vaultsByID))
	for _, vault := range s.vaultsByID {
		vaults = append(vaults, secretreference.Vault{ID: vault.ID, Name: vault.Name})
	}
	return vaults, nil
}

func (source serviceSecretSource) ListItems(vaultID string) ([]item.Item, error) {
	s := source.service
	if items, isCached := s.itemsByVaultID[vaultID]; isCached && source.isCacheOnly {
		return items, nil
	}
	vault, isFound := s.vaultsByID[vaultID]
	if !isFound {
		return nil, fmt.Errorf("vault %w", ErrNotFound)
	}
	items, err := s.unlockedSession.ListItems(source.ctx, vault)
	if err != nil {
		return nil, convertError(err)
	}
	s.itemsByVaultID[vaultID] = items
	return items, nil
}

func (s *Service) resolveSecrets(references []string) (cliprotocol.ResolveSecretResult, error) {
	if len(references) == 0 || len(references) > cliprotocol.MaxReferenceCount {
		return cliprotocol.ResolveSecretResult{}, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, fmt.Sprintf("send between 1 and %d secret references", cliprotocol.MaxReferenceCount))
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return cliprotocol.ResolveSecretResult{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	resolvedReferences, err := secretreference.ResolveAll(serviceSecretSource{service: s, ctx: ctx}, references, time.Now())
	var resolveError *secretreference.Error
	if errors.As(err, &resolveError) {
		return cliprotocol.ResolveSecretResult{}, cliprotocol.NewError(cliprotocol.ErrorNotFound, err.Error())
	}
	if err != nil {
		return cliprotocol.ResolveSecretResult{}, err
	}
	result := cliprotocol.ResolveSecretResult{Values: make([]string, 0, len(resolvedReferences))}
	for _, resolved := range resolvedReferences {
		result.Values = append(result.Values, resolved.Value)
	}
	return result, nil
}

// DescribeSecrets names the secrets for the approval window: with the vault, item and field names while
// Hisar is unlocked, and as the references are written while it is locked.
func (bridge *CommandLineBridge) DescribeSecrets(references []string) []string {
	s := bridge.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	names := make([]string, 0, len(references))
	for _, text := range references {
		if s.unlockedSession != nil {
			if resolved, err := secretreference.Locate(serviceSecretSource{service: s, ctx: ctx, isCacheOnly: true}, text); err == nil {
				names = append(names, resolved.Describe())
				continue
			}
		}
		if reference, err := secretreference.Parse(text); err == nil {
			names = append(names, reference.Describe())
		} else {
			names = append(names, text)
		}
	}
	return names
}

func isSharedName(names []string, name string) bool {
	count := 0
	for _, candidate := range names {
		if strings.EqualFold(candidate, name) {
			count++
		}
	}
	return count > 1
}

// CopySecretReference copies hisar://<vault>/<item>/<field> for one of the item's fields and returns it.
// A reference is not a secret, so the clipboard is not cleared later.
func (s *Service) CopySecretReference(vaultID, itemID, fieldName string) (string, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return "", err
	}
	vault, isFound := s.vaultsByID[vaultID]
	if !isFound {
		return "", fmt.Errorf("vault %w", ErrNotFound)
	}
	vaultItem, err := s.findItem(vaultID, itemID)
	if err != nil {
		return "", err
	}
	builtInField, isBuiltIn := secretreference.GetBuiltInField(fieldName)
	label := builtInField
	if !isBuiltIn {
		if err := secretreference.CheckCustomFieldLabel(vaultItem, fieldName); err != nil {
			return "", err
		}
		label = fieldName
	}
	if _, _, err := secretreference.ReadField(vaultItem, label, time.Now()); err != nil {
		return "", err
	}
	vaultNames := make([]string, 0, len(s.vaultsByID))
	for _, candidate := range s.vaultsByID {
		vaultNames = append(vaultNames, candidate.Name)
	}
	itemTitles := make([]string, 0, len(s.itemsByVaultID[vaultID]))
	for _, candidate := range s.itemsByVaultID[vaultID] {
		itemTitles = append(itemTitles, candidate.Overview.Title)
	}
	reference := secretreference.Reference{Vault: vault.Name, Item: vaultItem.Overview.Title, Field: label}
	if strings.TrimSpace(vault.Name) == "" || isSharedName(vaultNames, vault.Name) {
		reference.Vault = vault.ID
	}
	if strings.TrimSpace(vaultItem.Overview.Title) == "" || isSharedName(itemTitles, vaultItem.Overview.Title) {
		reference.Item = vaultItem.ID
	}
	text := reference.String()
	if s.config.Clipboard == nil || !s.config.Clipboard.SetText(text) {
		return "", errors.New("could not copy to the clipboard")
	}
	return text, nil
}
