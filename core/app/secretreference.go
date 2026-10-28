package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/secretreference"
	"github.com/tryhisar/hisar/core/session"
)

// serviceSecretSource lets references resolve against the unlocked vaults; the caller holds s.mutex.
// isCacheOnly uses what the app already loaded, so describing a request for the approval window stays quick.
type serviceSecretSource struct {
	service      *Service
	ctx          context.Context
	isCacheOnly  bool
	isMemoryOnly bool
}

func (source serviceSecretSource) ListVaults() (secretreference.Inventory, error) {
	if err := source.loadVaults(); err != nil {
		return secretreference.Inventory{}, err
	}
	return source.service.listSecretVaults(isAnyVault), nil
}

func (source serviceSecretSource) loadVaults() error {
	if source.isMemoryOnly {
		return nil
	}
	if !source.isCacheOnly || len(source.service.vaultsByID) == 0 {
		return source.service.loadVaults(source.ctx)
	}
	return nil
}

// listSecretVaults lists the loaded vaults isListed keeps. It is complete when none of those failed to open; a vault
// isListed drops cannot be the one a name means, since the caller never reaches it. It needs s.mutex.
func (s *Service) listSecretVaults(isListed func(vaultID string) bool) secretreference.Inventory {
	inventory := secretreference.Inventory{Vaults: []secretreference.Vault{}, IsComplete: true}
	for _, vault := range s.vaultsByID {
		if isListed(vault.ID) {
			inventory.Vaults = append(inventory.Vaults, secretreference.Vault{ID: vault.ID, Name: vault.Name})
		}
	}
	for _, issue := range s.vaultReadIssues {
		if isListed(issue.VaultID) {
			inventory.IsComplete = false
		}
	}
	return inventory
}

func isAnyVault(string) bool { return true }

func (source serviceSecretSource) ListItems(vaultID string) (secretreference.ItemList, error) {
	s := source.service
	if source.isMemoryOnly {
		if _, isLoaded := s.itemsByVaultID[vaultID]; !isLoaded {
			return secretreference.ItemList{}, fmt.Errorf("vault items %w", ErrNotFound)
		}
		return s.listSecretItems(vaultID), nil
	}
	if _, isCached := s.itemsByVaultID[vaultID]; !isCached || !source.isCacheOnly {
		vault, isFound := s.vaultsByID[vaultID]
		if !isFound {
			return secretreference.ItemList{}, fmt.Errorf("vault %w", ErrNotFound)
		}
		if _, err := s.readVaultItems(source.ctx, vault); err != nil {
			return secretreference.ItemList{}, err
		}
	}
	return s.listSecretItems(vaultID), nil
}

// listSecretItems is a vault's loaded items as references see them: complete only when every record opened and the
// vault synced to the end. It needs s.mutex.
func (s *Service) listSecretItems(vaultID string) secretreference.ItemList {
	return secretreference.ItemList{Items: s.itemsByVaultID[vaultID], IsComplete: s.isItemListComplete(vaultID)}
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
		if s.isUnlocked() {
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

// nameCounts counts vault names or item titles as the resolver matches them (strings.EqualFold), so a reference can
// tell a name that finds one vault or item from a name that finds several. IDs count too: the resolver tries them
// first, so a name that is another vault's or item's ID finds that one. Count exactly the vaults the reference's resolver
// searches: a vault it never sees must not change the reference.
type nameCounts map[string]int

// countVaultNames counts nothing in an incomplete inventory, whose names the resolver refuses, so references use IDs.
func countVaultNames(inventory secretreference.Inventory) nameCounts {
	counts := nameCounts{}
	if !inventory.IsComplete {
		return counts
	}
	for _, vault := range inventory.Vaults {
		counts[foldName(vault.Name)]++
		counts[foldName(vault.ID)]++
	}
	return counts
}

// countItemTitles counts nothing in an incomplete item list, whose titles the resolver refuses, so references use IDs.
func countItemTitles(items secretreference.ItemList) nameCounts {
	counts := nameCounts{}
	if !items.IsComplete {
		return counts
	}
	for index := range items.Items {
		counts[foldName(items.Items[index].Overview.Title)]++
		counts[foldName(items.Items[index].ID)]++
	}
	return counts
}

// isUnique: a reference can use name, which is not blank and finds nothing else.
func (counts nameCounts) isUnique(name string) bool {
	return strings.TrimSpace(name) != "" && counts[foldName(name)] == 1
}

// foldName maps every rune to the smallest rune of its case-folding orbit, so two names have the same foldName exactly
// when strings.EqualFold matches them.
func foldName(name string) string {
	return strings.Map(func(character rune) rune {
		smallest := character
		for folded := unicode.SimpleFold(character); folded != character; folded = unicode.SimpleFold(folded) {
			smallest = min(smallest, folded)
		}
		return smallest
	}, name)
}

// buildItemReference names the item's vault and the item by name, or by ID where the name is blank or also finds
// another vault, or another item of the vault, so the reference finds this item only.
func buildItemReference(vault *session.Vault, vaultItem *item.Item, vaultNames, itemTitles nameCounts) secretreference.Reference {
	reference := secretreference.Reference{Vault: vault.ID, Item: vaultItem.ID}
	if vaultNames.isUnique(vault.Name) {
		reference.Vault = vault.Name
	}
	if itemTitles.isUnique(vaultItem.Overview.Title) {
		reference.Item = vaultItem.Overview.Title
	}
	return reference
}

// fieldReference is a field an item has a value in: its label and its hisar:// reference.
type fieldReference struct {
	label     string
	reference string
}

// listFieldReferences returns the reference of every field of the item that has a value, as agents and launchers list
// them: never the notes, which may hold secrets an agent should not go looking for, or the private key.
func listFieldReferences(reference secretreference.Reference, vaultItem *item.Item) []fieldReference {
	references := []fieldReference{}
	addField := func(label, field, value string) {
		if value == "" {
			return
		}
		reference.Field = field
		references = append(references, fieldReference{label: label, reference: reference.String()})
	}
	addField(secretreference.FieldUsername, secretreference.FieldUsername, vaultItem.Overview.Username)
	addField(secretreference.FieldPassword, secretreference.FieldPassword, vaultItem.Details.Password)
	addField("one-time code", secretreference.FieldOneTimeCode, vaultItem.Details.OneTimePassword)
	addField(secretreference.FieldPublicKey, secretreference.FieldPublicKey, vaultItem.Overview.PublicKey)
	addField(secretreference.FieldFingerprint, secretreference.FieldFingerprint, vaultItem.Overview.Fingerprint)
	// A database's connection string is built, not stored, so callers get its reference instead of assembling one.
	addField(secretreference.FieldConnectionString, secretreference.FieldConnectionString, vaultItem.ConnectionString())
	for _, field := range vaultItem.Details.Fields {
		if secretreference.CheckCustomFieldLabel(vaultItem, field.Name) == nil {
			addField(field.Name, field.Name, field.Value)
		}
	}
	return references
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
	if !s.hasReadVaultItems(vaultID) {
		// Whether the title finds only this item depends on every item of the vault.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := s.readVaultItems(ctx, vault); err != nil {
			return "", err
		}
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
	reference := buildItemReference(vault, vaultItem, countVaultNames(s.listSecretVaults(isAnyVault)), countItemTitles(s.listSecretItems(vaultID)))
	reference.Field = label
	text := reference.String()
	if s.config.Clipboard == nil || !s.config.Clipboard.SetText(text) {
		return "", errors.New("could not copy to the clipboard")
	}
	return text, nil
}
