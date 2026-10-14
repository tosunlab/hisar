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

// ListAllAgentVaultID returns the vaults that coding agents may see through hisar mcp on this device.
func (s *Service) ListAllAgentVaultID() []string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return append([]string{}, s.state.AllAgentVaultID...)
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
	allVaultID := slices.DeleteFunc(append([]string{}, s.state.AllAgentVaultID...), func(candidate string) bool { return candidate == vaultID })
	if isAvailable {
		allVaultID = append(allVaultID, vaultID)
	}
	s.state.AllAgentVaultID = allVaultID
	return append([]string{}, allVaultID...), s.state.Save()
}

// agentSecretSource is serviceSecretSource limited to the agent-visible vaults: the others do not exist for it.
type agentSecretSource struct {
	serviceSecretSource
}

func (source agentSecretSource) ListAllVault() ([]secretreference.Vault, error) {
	allVault, err := source.serviceSecretSource.ListAllVault()
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(allVault, func(vault secretreference.Vault) bool { return !source.service.isAgentVault(vault.ID) }), nil
}

func (source agentSecretSource) ListAllItem(vaultID string) ([]item.Item, error) {
	if !source.service.isAgentVault(vaultID) {
		return nil, fmt.Errorf("vault %w", ErrNotFound)
	}
	return source.serviceSecretSource.ListAllItem(vaultID)
}

// isAgentVault needs s.mutex.
func (s *Service) isAgentVault(vaultID string) bool {
	return slices.Contains(s.state.AllAgentVaultID, vaultID)
}

// listAllAgentVault loads the vaults and returns the agent-visible ones, sorted like the app's list; it needs s.mutex.
func (s *Service) listAllAgentVault(ctx context.Context) ([]*session.Vault, error) {
	if err := s.loadAllVault(ctx); err != nil {
		return nil, err
	}
	var allVault []*session.Vault
	for _, vault := range s.allVaultByID {
		if s.isAgentVault(vault.ID) {
			allVault = append(allVault, vault)
		}
	}
	slices.SortFunc(allVault, func(first, second *session.Vault) int {
		return strings.Compare(strings.ToLower(first.Name), strings.ToLower(second.Name))
	})
	return allVault, nil
}

func (s *Service) listAllAgentVaultSummary() (cliprotocol.AgentVaultListResult, error) {
	result := cliprotocol.AgentVaultListResult{Vaults: []cliprotocol.AgentVault{}}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	allVault, err := s.listAllAgentVault(ctx)
	if err != nil {
		return result, err
	}
	for _, vault := range allVault {
		allItem, err := s.unlockedSession.ListAllItem(ctx, vault)
		if err != nil {
			return result, convertError(err)
		}
		s.allItemByVaultID[vault.ID] = allItem
		result.Vaults = append(result.Vaults, cliprotocol.AgentVault{ID: vault.ID, Name: vault.Name, ItemCount: len(allItem)})
	}
	return result, nil
}

func isAgentSearchMatch(vaultName string, vaultItem *item.Item, query string) bool {
	if query == "" {
		return true
	}
	allText := append([]string{vaultName, vaultItem.Overview.Title, vaultItem.Overview.Username}, vaultItem.Overview.URLs...)
	return slices.ContainsFunc(allText, func(text string) bool { return strings.Contains(strings.ToLower(text), query) })
}

// searchAllAgentItem returns metadata only: never a value and never the notes, which may hold secrets.
func (s *Service) searchAllAgentItem(query, vaultQuery string) (cliprotocol.AgentItemSearchResult, error) {
	result := cliprotocol.AgentItemSearchResult{Items: []cliprotocol.AgentItem{}}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	allVault, err := s.listAllAgentVault(ctx)
	if err != nil {
		return result, err
	}
	if vaultQuery != "" {
		allVault = slices.DeleteFunc(allVault, func(vault *session.Vault) bool {
			return vault.ID != vaultQuery && !strings.EqualFold(vault.Name, vaultQuery)
		})
		if len(allVault) == 0 {
			return result, cliprotocol.CreateError(cliprotocol.ErrorNotFound, fmt.Sprintf("no vault available to agents is named %q", vaultQuery))
		}
	}
	allVaultName := make([]string, 0, len(s.allVaultByID))
	for _, vault := range s.allVaultByID {
		allVaultName = append(allVaultName, vault.Name)
	}
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	for _, vault := range allVault {
		allItem, err := s.unlockedSession.ListAllItem(ctx, vault)
		if err != nil {
			return result, convertError(err)
		}
		s.allItemByVaultID[vault.ID] = allItem
		allItemTitle := make([]string, 0, len(allItem))
		for _, vaultItem := range allItem {
			allItemTitle = append(allItemTitle, vaultItem.Overview.Title)
		}
		for index := range allItem {
			vaultItem := &allItem[index]
			if !isAgentSearchMatch(vault.Name, vaultItem, normalizedQuery) {
				continue
			}
			result.TotalCount++
			if len(result.Items) < maximumAgentSearchResultCount {
				result.Items = append(result.Items, convertToAgentItem(vault, vaultItem, isSharedName(allVaultName, vault.Name), isSharedName(allItemTitle, vaultItem.Overview.Title)))
			}
		}
	}
	return result, nil
}

func convertToAgentItem(vault *session.Vault, vaultItem *item.Item, isVaultNameShared, isItemTitleShared bool) cliprotocol.AgentItem {
	itemType := vaultItem.Overview.Category
	if itemType == "" {
		itemType = "login"
	}
	agentItem := cliprotocol.AgentItem{
		ID: vaultItem.ID, VaultID: vault.ID, VaultName: vault.Name, Title: vaultItem.Overview.Title, Type: itemType,
		Username: vaultItem.Overview.Username, URLs: append([]string{}, vaultItem.Overview.URLs...),
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
	for _, field := range vaultItem.Details.Fields {
		if secretreference.CheckCustomFieldLabel(vaultItem, field.Name) == nil {
			addField(field.Name, field.Name, field.Value)
		}
	}
	return agentItem
}

// buildAllAgentReference turns a code request into an otp reference; it needs s.mutex and the vaults loaded.
func (s *Service) buildAllAgentReference(source agentSecretSource, request *cliprotocol.Request) ([]string, error) {
	if request.Type != cliprotocol.TypeAgentGetCode {
		return request.References, nil
	}
	if request.ItemID == "" {
		reference, err := secretreference.Parse(request.References[0])
		if err != nil {
			return nil, cliprotocol.CreateError(cliprotocol.ErrorNotFound, request.References[0]+": "+err.Error())
		}
		reference.Field = secretreference.FieldOneTimeCode
		return []string{reference.String()}, nil
	}
	allVault, err := source.ListAllVault()
	if err != nil {
		return nil, err
	}
	for _, vault := range allVault {
		allItem, err := source.ListAllItem(vault.ID)
		if err != nil {
			return nil, err
		}
		if slices.ContainsFunc(allItem, func(vaultItem item.Item) bool { return vaultItem.ID == request.ItemID }) {
			return []string{secretreference.Reference{Vault: vault.ID, Item: request.ItemID, Field: secretreference.FieldOneTimeCode}.String()}, nil
		}
	}
	return nil, cliprotocol.CreateError(cliprotocol.ErrorNotFound, fmt.Sprintf("no item available to agents has the ID %q", request.ItemID))
}

// resolveAllAgentSecret resolves among the agent-visible vaults only; a reference elsewhere fails as not found.
func (s *Service) resolveAllAgentSecret(request *cliprotocol.Request) ([]secretreference.Resolved, error) {
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	if len(s.state.AllAgentVaultID) == 0 {
		return nil, cliprotocol.CreateError(cliprotocol.ErrorNotFound, noAgentVaultMessage)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	source := agentSecretSource{serviceSecretSource{service: s, ctx: ctx}}
	allReference, err := s.buildAllAgentReference(source, request)
	if err != nil {
		return nil, err
	}
	allResolved, err := secretreference.ResolveAll(source, allReference, s.config.Now())
	var resolveError *secretreference.Error
	if errors.As(err, &resolveError) {
		return nil, cliprotocol.CreateError(cliprotocol.ErrorNotFound, err.Error())
	}
	return allResolved, err
}

// LocateAllAgentSecret names the secrets of an agent request for the approval window without handing out values.
func (bridge *CommandLineBridge) LocateAllAgentSecret(request *cliprotocol.Request) ([]clisocket.AgentSecret, error) {
	s := bridge.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	allResolved, err := s.resolveAllAgentSecret(request)
	if err != nil {
		return nil, convertCommandLineError(err)
	}
	allSecret := make([]clisocket.AgentSecret, 0, len(allResolved))
	for _, resolved := range allResolved {
		allSecret = append(allSecret, clisocket.AgentSecret{
			VaultID: resolved.VaultID, ItemID: resolved.ItemID, VaultName: resolved.VaultName, ItemTitle: resolved.ItemTitle, FieldLabel: resolved.FieldLabel,
		})
	}
	return allSecret, nil
}

func (bridge *CommandLineBridge) answerAgentRequest(request *cliprotocol.Request) (any, error) {
	s := bridge.service
	switch request.Type {
	case cliprotocol.TypeAgentListVault:
		return s.listAllAgentVaultSummary()
	case cliprotocol.TypeAgentSearchItem:
		return s.searchAllAgentItem(request.Query, request.Vault)
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	allResolved, err := s.resolveAllAgentSecret(request)
	if err != nil {
		return nil, err
	}
	if request.Type == cliprotocol.TypeAgentGetCode {
		vaultItem, err := s.findItem(allResolved[0].VaultID, allResolved[0].ItemID)
		if err != nil {
			return nil, err
		}
		code, err := generateOneTimeCode(vaultItem, s.config.Now())
		return cliprotocol.CodeResult{Code: code.Code, ExpiresAt: code.ExpiresAt, Period: code.Period}, err
	}
	result := cliprotocol.ResolveSecretResult{Values: make([]string, 0, len(allResolved))}
	for _, resolved := range allResolved {
		result.Values = append(result.Values, resolved.Value)
	}
	return result, nil
}
