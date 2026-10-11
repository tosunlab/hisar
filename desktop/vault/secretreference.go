package vault

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

func (source serviceSecretSource) ListAllVault() ([]secretreference.Vault, error) {
	s := source.service
	if !source.isCacheOnly || len(s.allVaultByID) == 0 {
		if err := s.loadAllVault(source.ctx); err != nil {
			return nil, err
		}
	}
	allVault := make([]secretreference.Vault, 0, len(s.allVaultByID))
	for _, vault := range s.allVaultByID {
		allVault = append(allVault, secretreference.Vault{ID: vault.ID, Name: vault.Name})
	}
	return allVault, nil
}

func (source serviceSecretSource) ListAllItem(vaultID string) ([]item.Item, error) {
	s := source.service
	if allItem, isCached := s.allItemByVaultID[vaultID]; isCached && source.isCacheOnly {
		return allItem, nil
	}
	vault, isFound := s.allVaultByID[vaultID]
	if !isFound {
		return nil, fmt.Errorf("vault %w", ErrNotFound)
	}
	allItem, err := s.unlockedSession.ListAllItem(source.ctx, vault)
	if err != nil {
		return nil, convertError(err)
	}
	s.allItemByVaultID[vaultID] = allItem
	return allItem, nil
}

func (s *Service) resolveAllSecret(allReference []string) (cliprotocol.ResolveSecretResult, error) {
	if len(allReference) == 0 || len(allReference) > cliprotocol.MaxReferenceCount {
		return cliprotocol.ResolveSecretResult{}, cliprotocol.CreateError(cliprotocol.ErrorInvalidRequest, fmt.Sprintf("send between 1 and %d secret references", cliprotocol.MaxReferenceCount))
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return cliprotocol.ResolveSecretResult{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	allResolved, err := secretreference.ResolveAll(serviceSecretSource{service: s, ctx: ctx}, allReference, time.Now())
	var resolveError *secretreference.Error
	if errors.As(err, &resolveError) {
		return cliprotocol.ResolveSecretResult{}, cliprotocol.CreateError(cliprotocol.ErrorNotFound, err.Error())
	}
	if err != nil {
		return cliprotocol.ResolveSecretResult{}, err
	}
	result := cliprotocol.ResolveSecretResult{Values: make([]string, 0, len(allResolved))}
	for _, resolved := range allResolved {
		result.Values = append(result.Values, resolved.Value)
	}
	return result, nil
}

// DescribeAllSecret names the secrets for the approval window: with the vault, item and field names while
// Hisar is unlocked, and as the references are written while it is locked.
func (bridge *CommandLineBridge) DescribeAllSecret(allReference []string) []string {
	s := bridge.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	allName := make([]string, 0, len(allReference))
	for _, text := range allReference {
		if s.unlockedSession != nil {
			if resolved, err := secretreference.Locate(serviceSecretSource{service: s, ctx: ctx, isCacheOnly: true}, text); err == nil {
				allName = append(allName, resolved.Describe())
				continue
			}
		}
		if reference, err := secretreference.Parse(text); err == nil {
			allName = append(allName, reference.Describe())
		} else {
			allName = append(allName, text)
		}
	}
	return allName
}

func isSharedName(allName []string, name string) bool {
	count := 0
	for _, candidate := range allName {
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
	vault, isFound := s.allVaultByID[vaultID]
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
	allVaultName := make([]string, 0, len(s.allVaultByID))
	for _, candidate := range s.allVaultByID {
		allVaultName = append(allVaultName, candidate.Name)
	}
	allItemTitle := make([]string, 0, len(s.allItemByVaultID[vaultID]))
	for _, candidate := range s.allItemByVaultID[vaultID] {
		allItemTitle = append(allItemTitle, candidate.Overview.Title)
	}
	reference := secretreference.Reference{Vault: vault.Name, Item: vaultItem.Overview.Title, Field: label}
	if strings.TrimSpace(vault.Name) == "" || isSharedName(allVaultName, vault.Name) {
		reference.Vault = vault.ID
	}
	if strings.TrimSpace(vaultItem.Overview.Title) == "" || isSharedName(allItemTitle, vaultItem.Overview.Title) {
		reference.Item = vaultItem.ID
	}
	text := reference.String()
	if s.config.Clipboard == nil || !s.config.Clipboard.SetText(text) {
		return "", errors.New("could not copy to the clipboard")
	}
	return text, nil
}
