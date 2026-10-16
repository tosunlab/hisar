package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/secretreference"
	"github.com/tryhisar/hisar/core/session"
)

const (
	maximumAgentSearchResultCount = 50
	noAgentVaultMessage           = "no vault is available to coding agents; in Hisar, open Settings → Developer tools → Coding agents and turn on “Available to agents” for the vaults an agent may use"
)

// ListAgentVaultIDs returns the vaults that coding agents may see through hisar mcp on this device.
func (s *Service) ListAgentVaultIDs() []string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return append([]string{}, s.state.AgentVaultIDs...)
}

// SetVaultAvailableToAgents saves the vault's "Available to agents" switch; every vault starts off.
func (s *Service) SetVaultAvailableToAgents(vaultID string, isAvailable bool) ([]string, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	if isAvailable {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := s.findVault(ctx, vaultID); err != nil {
			return nil, err
		}
	}
	vaultIDs := slices.DeleteFunc(append([]string{}, s.state.AgentVaultIDs...), func(candidate string) bool { return candidate == vaultID })
	if isAvailable {
		vaultIDs = append(vaultIDs, vaultID)
	}
	s.state.AgentVaultIDs = vaultIDs
	return append([]string{}, vaultIDs...), s.state.Save()
}

// agentSecretSource is serviceSecretSource limited to the agent-visible vaults: the others do not exist for it.
type agentSecretSource struct {
	serviceSecretSource
}

func (source agentSecretSource) ListVaults() ([]secretreference.Vault, error) {
	vaults, err := source.serviceSecretSource.ListVaults()
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(vaults, func(vault secretreference.Vault) bool { return !source.service.isAgentVault(vault.ID) }), nil
}

func (source agentSecretSource) ListItems(vaultID string) ([]item.Item, error) {
	if !source.service.isAgentVault(vaultID) {
		return nil, fmt.Errorf("vault %w", ErrNotFound)
	}
	return source.serviceSecretSource.ListItems(vaultID)
}

// isAgentVault needs s.mutex.
func (s *Service) isAgentVault(vaultID string) bool {
	return slices.Contains(s.state.AgentVaultIDs, vaultID)
}

// listAgentVaults loads the vaults and returns the agent-visible ones, sorted like the app's list; it needs s.mutex.
func (s *Service) listAgentVaults(ctx context.Context) ([]*session.Vault, error) {
	if err := s.loadVaults(ctx); err != nil {
		return nil, err
	}
	var vaults []*session.Vault
	for _, vault := range s.vaultsByID {
		if s.isAgentVault(vault.ID) {
			vaults = append(vaults, vault)
		}
	}
	slices.SortFunc(vaults, func(first, second *session.Vault) int {
		return strings.Compare(strings.ToLower(first.Name), strings.ToLower(second.Name))
	})
	return vaults, nil
}

func (s *Service) listAgentVaultSummaries() (cliprotocol.AgentVaultListResult, error) {
	result := cliprotocol.AgentVaultListResult{Vaults: []cliprotocol.AgentVault{}}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vaults, err := s.listAgentVaults(ctx)
	if err != nil {
		return result, err
	}
	for _, vault := range vaults {
		items, err := s.unlockedSession.ListItems(ctx, vault)
		if err != nil {
			return result, convertError(err)
		}
		s.itemsByVaultID[vault.ID] = items
		result.Vaults = append(result.Vaults, cliprotocol.AgentVault{ID: vault.ID, Name: vault.Name, ItemCount: len(items)})
	}
	return result, nil
}

func isAgentSearchMatch(vaultName string, vaultItem *item.Item, query string) bool {
	if query == "" {
		return true
	}
	texts := append([]string{vaultName, vaultItem.Overview.Title, vaultItem.Overview.Username}, vaultItem.Overview.URLs...)
	texts = append(texts, vaultItem.Overview.Tags...)
	if vaultItem.IsCategory(item.CategoryAPICredential) {
		// An API credential keeps these in named fields, not the username and websites; never the secret.
		credential, _ := item.ReadAPICredential(vaultItem)
		texts = append(texts, credential.Service, credential.KeyID, credential.Host)
	}
	// An address item's and an identity's city and country, as lists show them; not the street or postal code.
	if vaultItem.IsCategory(item.CategoryAddress) {
		address, _ := item.ReadAddress(vaultItem)
		texts = append(texts, address.City, address.Country)
	}
	if vaultItem.IsCategory(item.CategoryIdentity) {
		identity, _ := item.ReadIdentity(vaultItem)
		texts = append(texts, identity.Address.City, identity.Address.Country)
	}
	if vaultItem.IsCategory(item.CategoryServer) {
		// A server's host is a named field; its username and admin console are the item's own.
		server, _ := item.ReadServer(vaultItem)
		texts = append(texts, server.Host)
	}
	if vaultItem.IsCategory(item.CategoryDatabase) {
		// A database's host and name are named fields; its username is the item's own.
		database, _ := item.ReadDatabase(vaultItem)
		texts = append(texts, database.Host, database.Name)
	}
	if vaultItem.IsCategory(item.CategorySoftwareLicense) {
		// What the apps' search reads; never the license key.
		license, _ := item.ReadSoftwareLicense(vaultItem)
		texts = append(texts, license.Product, license.Version, license.LicensedTo, license.Email)
	}
	return slices.ContainsFunc(texts, func(text string) bool { return strings.Contains(strings.ToLower(text), query) })
}

// searchAgentItems returns metadata only: never a value and never the notes, which may hold secrets.
func (s *Service) searchAgentItems(query, vaultQuery string) (cliprotocol.AgentItemSearchResult, error) {
	result := cliprotocol.AgentItemSearchResult{Items: []cliprotocol.AgentItem{}}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vaults, err := s.listAgentVaults(ctx)
	if err != nil {
		return result, err
	}
	if vaultQuery != "" {
		vaults = slices.DeleteFunc(vaults, func(vault *session.Vault) bool {
			return vault.ID != vaultQuery && !strings.EqualFold(vault.Name, vaultQuery)
		})
		if len(vaults) == 0 {
			return result, cliprotocol.NewError(cliprotocol.ErrorNotFound, fmt.Sprintf("no vault available to agents is named %q", vaultQuery))
		}
	}
	vaultNames := make([]string, 0, len(s.vaultsByID))
	for _, vault := range s.vaultsByID {
		vaultNames = append(vaultNames, vault.Name)
	}
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	for _, vault := range vaults {
		items, err := s.unlockedSession.ListItems(ctx, vault)
		if err != nil {
			return result, convertError(err)
		}
		s.itemsByVaultID[vault.ID] = items
		itemTitles := make([]string, 0, len(items))
		for _, vaultItem := range items {
			itemTitles = append(itemTitles, vaultItem.Overview.Title)
		}
		for index := range items {
			vaultItem := &items[index]
			if !isAgentSearchMatch(vault.Name, vaultItem, normalizedQuery) {
				continue
			}
			result.TotalCount++
			if len(result.Items) < maximumAgentSearchResultCount {
				result.Items = append(result.Items, convertToAgentItem(vault, vaultItem, isSharedName(vaultNames, vault.Name), isSharedName(itemTitles, vaultItem.Overview.Title)))
			}
		}
	}
	return result, nil
}

func convertToAgentItem(vault *session.Vault, vaultItem *item.Item, isVaultNameShared, isItemTitleShared bool) cliprotocol.AgentItem {
	agentItem := cliprotocol.AgentItem{
		ID: vaultItem.ID, VaultID: vault.ID, VaultName: vault.Name, Title: vaultItem.Overview.Title, Type: vaultItem.Category(),
		Username: vaultItem.Overview.Username, URLs: append([]string{}, vaultItem.Overview.URLs...), Tags: append([]string{}, vaultItem.Overview.Tags...),
		HasNotes: strings.TrimSpace(vaultItem.Details.Notes) != "", HasOneTimePassword: vaultItem.Details.OneTimePassword != "",
		Fields: []cliprotocol.AgentField{},
	}
	reference := secretreference.Reference{Vault: vault.Name, Item: vaultItem.Overview.Title}
	if strings.TrimSpace(vault.Name) == "" || isVaultNameShared {
		reference.Vault = vault.ID
	}
	if strings.TrimSpace(vaultItem.Overview.Title) == "" || isItemTitleShared {
		reference.Item = vaultItem.ID
	}
	addField := func(label, field, value string) {
		if value == "" {
			return
		}
		reference.Field = field
		agentItem.Fields = append(agentItem.Fields, cliprotocol.AgentField{Label: label, Reference: reference.String()})
	}
	addField(secretreference.FieldUsername, secretreference.FieldUsername, vaultItem.Overview.Username)
	addField(secretreference.FieldPassword, secretreference.FieldPassword, vaultItem.Details.Password)
	addField("one-time code", secretreference.FieldOneTimeCode, vaultItem.Details.OneTimePassword)
	addField(secretreference.FieldPublicKey, secretreference.FieldPublicKey, vaultItem.Overview.PublicKey)
	addField(secretreference.FieldFingerprint, secretreference.FieldFingerprint, vaultItem.Overview.Fingerprint)
	// A database's connection string is built, not stored, so agents get its reference instead of assembling one.
	addField(secretreference.FieldConnectionString, secretreference.FieldConnectionString, vaultItem.ConnectionString())
	for _, field := range vaultItem.Details.Fields {
		if secretreference.CheckCustomFieldLabel(vaultItem, field.Name) == nil {
			addField(field.Name, field.Name, field.Value)
		}
	}
	return agentItem
}

// buildAgentReferences turns a code request into an otp reference; it needs s.mutex and the vaults loaded.
func (s *Service) buildAgentReferences(source agentSecretSource, request *cliprotocol.Request) ([]string, error) {
	if request.Type != cliprotocol.TypeAgentGetCode {
		return request.References, nil
	}
	if request.ItemID == "" {
		reference, err := secretreference.Parse(request.References[0])
		if err != nil {
			return nil, cliprotocol.NewError(cliprotocol.ErrorNotFound, request.References[0]+": "+err.Error())
		}
		reference.Field = secretreference.FieldOneTimeCode
		return []string{reference.String()}, nil
	}
	vaults, err := source.ListVaults()
	if err != nil {
		return nil, err
	}
	for _, vault := range vaults {
		items, err := source.ListItems(vault.ID)
		if err != nil {
			return nil, err
		}
		if slices.ContainsFunc(items, func(vaultItem item.Item) bool { return vaultItem.ID == request.ItemID }) {
			return []string{secretreference.Reference{Vault: vault.ID, Item: request.ItemID, Field: secretreference.FieldOneTimeCode}.String()}, nil
		}
	}
	return nil, cliprotocol.NewError(cliprotocol.ErrorNotFound, fmt.Sprintf("no item available to agents has the ID %q", request.ItemID))
}

// resolveAgentSecrets resolves among the agent-visible vaults only; a reference elsewhere fails as not found.
func (s *Service) resolveAgentSecrets(request *cliprotocol.Request) ([]secretreference.Resolved, error) {
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	if len(s.state.AgentVaultIDs) == 0 {
		return nil, cliprotocol.NewError(cliprotocol.ErrorNotFound, noAgentVaultMessage)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	source := agentSecretSource{serviceSecretSource{service: s, ctx: ctx}}
	references, err := s.buildAgentReferences(source, request)
	if err != nil {
		return nil, err
	}
	resolvedReferences, err := secretreference.ResolveAll(source, references, s.config.Now())
	var resolveError *secretreference.Error
	if errors.As(err, &resolveError) {
		return nil, cliprotocol.NewError(cliprotocol.ErrorNotFound, err.Error())
	}
	return resolvedReferences, err
}

// LocateAgentSecrets names the secrets of an agent request for the approval window without handing out values.
func (bridge *CommandLineBridge) LocateAgentSecrets(request *cliprotocol.Request) ([]clisocket.AgentSecret, error) {
	s := bridge.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	resolvedReferences, err := s.resolveAgentSecrets(request)
	if err != nil {
		return nil, convertCommandLineError(err)
	}
	secrets := make([]clisocket.AgentSecret, 0, len(resolvedReferences))
	for _, resolved := range resolvedReferences {
		secrets = append(secrets, clisocket.AgentSecret{
			VaultID: resolved.VaultID, ItemID: resolved.ItemID, VaultName: resolved.VaultName, ItemTitle: resolved.ItemTitle, FieldLabel: resolved.FieldLabel,
		})
	}
	return secrets, nil
}

func (bridge *CommandLineBridge) answerAgentRequest(request *cliprotocol.Request) (any, error) {
	s := bridge.service
	switch request.Type {
	case cliprotocol.TypeAgentListVault:
		return s.listAgentVaultSummaries()
	case cliprotocol.TypeAgentSearchItem:
		return s.searchAgentItems(request.Query, request.Vault)
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	resolvedReferences, err := s.resolveAgentSecrets(request)
	if err != nil {
		return nil, err
	}
	if request.Type == cliprotocol.TypeAgentGetCode {
		vaultItem, err := s.findItem(resolvedReferences[0].VaultID, resolvedReferences[0].ItemID)
		if err != nil {
			return nil, err
		}
		code, err := generateOneTimeCode(vaultItem, s.config.Now())
		return cliprotocol.CodeResult{Code: code.Code, ExpiresAt: code.ExpiresAt, Period: code.Period}, err
	}
	result := cliprotocol.ResolveSecretResult{Values: make([]string, 0, len(resolvedReferences))}
	for _, resolved := range resolvedReferences {
		result.Values = append(result.Values, resolved.Value)
	}
	return result, nil
}
