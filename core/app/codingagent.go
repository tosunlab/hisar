package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/localstate"
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

func (source agentSecretSource) ListVaults() (secretreference.Inventory, error) {
	if err := source.loadVaults(); err != nil {
		return secretreference.Inventory{}, err
	}
	return source.service.listSecretVaults(source.service.isAgentVault), nil
}

func (source agentSecretSource) ListItems(vaultID string) (secretreference.ItemList, error) {
	if !source.service.isAgentVault(vaultID) {
		return secretreference.ItemList{}, fmt.Errorf("vault %w", ErrNotFound)
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
		read, err := s.readVaultItems(ctx, vault)
		if err != nil {
			return result, err
		}
		result.Vaults = append(result.Vaults, cliprotocol.AgentVault{ID: vault.ID, Name: vault.Name, ItemCount: len(read.Items)})
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
	vaultNames := countVaultNames(s.listSecretVaults(isAnyVault))
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	for _, vault := range vaults {
		read, err := s.readVaultItems(ctx, vault)
		if err != nil {
			return result, err
		}
		items := read.Items
		itemTitles := countItemTitles(s.listSecretItems(vault.ID))
		for index := range items {
			vaultItem := &items[index]
			if !isAgentSearchMatch(vault.Name, vaultItem, normalizedQuery) {
				continue
			}
			result.TotalCount++
			if len(result.Items) < maximumAgentSearchResultCount {
				result.Items = append(result.Items, convertToAgentItem(vault, vaultItem, buildItemReference(vault, vaultItem, vaultNames, itemTitles)))
			}
		}
	}
	return result, nil
}

func convertToAgentItem(vault *session.Vault, vaultItem *item.Item, reference secretreference.Reference) cliprotocol.AgentItem {
	agentItem := cliprotocol.AgentItem{
		ID: vaultItem.ID, VaultID: vault.ID, VaultName: vault.Name, Title: vaultItem.Overview.Title, Type: vaultItem.Category(),
		Username: vaultItem.Overview.Username, URLs: vaultItem.Websites(), Tags: append([]string{}, vaultItem.Overview.Tags...),
		HasNotes: strings.TrimSpace(vaultItem.Details.Notes) != "", HasOneTimePassword: vaultItem.Details.OneTimePassword != "",
		Fields: []cliprotocol.AgentField{},
	}
	for _, field := range listFieldReferences(reference, vaultItem) {
		agentItem.Fields = append(agentItem.Fields, cliprotocol.AgentField{Label: field.label, Reference: field.reference})
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
	inventory, err := source.ListVaults()
	if err != nil {
		return nil, err
	}
	for _, vault := range inventory.Vaults {
		items, err := source.ListItems(vault.ID)
		if err != nil {
			return nil, err
		}
		if slices.ContainsFunc(items.Items, func(vaultItem item.Item) bool { return vaultItem.ID == request.ItemID }) {
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

// LocateAgentSecrets names the secrets of an agent request for the approval window without handing out values; for
// hisar get inside an agent harness, the item it asks for.
func (bridge *CommandLineBridge) LocateAgentSecrets(request *cliprotocol.Request) ([]clisocket.AgentSecret, error) {
	s := bridge.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if request.Type == cliprotocol.TypeGetItem {
		secret, err := s.locateAgentItem(request.VaultID, request.ItemID)
		return []clisocket.AgentSecret{secret}, convertCommandLineError(err)
	}
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

// locateAgentItem finds an item in an agent-visible vault; an item elsewhere fails as not found. It needs s.mutex.
func (s *Service) locateAgentItem(vaultID, itemID string) (clisocket.AgentSecret, error) {
	if err := s.requireUnlocked(); err != nil {
		return clisocket.AgentSecret{}, err
	}
	if !s.isAgentVault(vaultID) {
		return clisocket.AgentSecret{}, fmt.Errorf("vault %w", ErrNotFound)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vault, err := s.findVault(ctx, vaultID)
	if err != nil {
		return clisocket.AgentSecret{}, err
	}
	items, err := agentSecretSource{serviceSecretSource{service: s, ctx: ctx}}.ListItems(vaultID)
	if err != nil {
		return clisocket.AgentSecret{}, err
	}
	index := slices.IndexFunc(items.Items, func(vaultItem item.Item) bool { return vaultItem.ID == itemID })
	if index < 0 {
		return clisocket.AgentSecret{}, fmt.Errorf("item %w", ErrNotFound)
	}
	return clisocket.AgentSecret{VaultID: vaultID, ItemID: itemID, VaultName: vault.Name, ItemTitle: items.Items[index].Overview.Title}, nil
}

// answerAgentSessionRequest answers what clisocket lets an ordinary hisar command inside an agent harness send, among
// the agent-visible vaults only: a hidden vault fails like one that does not exist.
func (bridge *CommandLineBridge) answerAgentSessionRequest(epoch uint64, request *cliprotocol.Request) (any, error) {
	s := bridge.service
	switch request.Type {
	case cliprotocol.TypeListVault, cliprotocol.TypeReadVault:
		summaries, err := s.ListVaults()
		if err != nil {
			return nil, err
		}
		agentVaultIDs := s.ListAgentVaultIDs()
		result := cliprotocol.VaultListResult{Vaults: []cliprotocol.Vault{}}
		for _, summary := range summaries {
			if slices.Contains(agentVaultIDs, summary.ID) {
				if request.Type == cliprotocol.TypeListVault && len(summary.ReadIssues) != 0 {
					return nil, session.ErrIncompleteRead
				}
				result.Issues = append(result.Issues, summary.ReadIssues...)
				result.Vaults = append(result.Vaults, convertToCommandLineVault(summary))
			}
		}
		return result, nil
	case cliprotocol.TypeListItem, cliprotocol.TypeReadItem, cliprotocol.TypeGetItem:
		if !slices.Contains(s.ListAgentVaultIDs(), request.VaultID) {
			return nil, fmt.Errorf("vault %w", ErrNotFound)
		}
		return bridge.answerCommandLineRequest(epoch, request)
	case cliprotocol.TypeResolveSecret:
		s.mutex.Lock()
		defer s.mutex.Unlock()
		resolvedReferences, err := s.resolveAgentSecrets(request)
		if err != nil {
			return nil, err
		}
		result := cliprotocol.ResolveSecretResult{Values: make([]string, 0, len(resolvedReferences))}
		for _, resolved := range resolvedReferences {
			result.Values = append(result.Values, resolved.Value)
		}
		return result, nil
	}
	return nil, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, fmt.Sprintf("unknown request type %q", request.Type))
}

// IsAgentHost and AddAgentHost keep the CLI socket's agent hosts in desktop.json (clisocket.AgentHosts).
func (bridge *CommandLineBridge) IsAgentHost(app clisocket.AppIdentity) bool {
	s := bridge.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return slices.Contains(s.state.AgentHosts, localstate.AgentHost{Path: app.Path, TeamID: app.TeamID})
}

func (bridge *CommandLineBridge) AddAgentHost(app clisocket.AppIdentity) error {
	s := bridge.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	host := localstate.AgentHost{Path: app.Path, TeamID: app.TeamID}
	if slices.Contains(s.state.AgentHosts, host) {
		return nil
	}
	s.state.AgentHosts = append(s.state.AgentHosts, host)
	return s.state.Save()
}

// AgentSession is a coding agent harness, such as omp, that sent hisar requests and still runs, as Settings → Coding
// agents lists it. PID and StartTime name it for RevokeAgentSession. IsSigned: the harness was recognized by its code
// signature. The host is the app it runs in, empty when Hisar could not verify one. FirstRequestAt is RFC 3339.
type AgentSession struct {
	PID               int                     `json:"pid"`
	StartTime         int64                   `json:"startTime"`
	HarnessName       string                  `json:"harnessName"`
	IsSigned          bool                    `json:"isSigned"`
	HostName          string                  `json:"hostName"`
	HostIconURL       string                  `json:"hostIconURL"`
	ProjectName       string                  `json:"projectName"`
	ProjectPath       string                  `json:"projectPath"`
	FirstRequestAt    string                  `json:"firstRequestAt"`
	RememberedSecrets []RememberedAgentSecret `json:"allRememberedSecret"`
}

// RememberedAgentSecret is a secret an agent session uses, or reads when IsReveal, without asking until ExpiresAt (RFC
// 3339).
type RememberedAgentSecret struct {
	Name      string `json:"name"`
	IsReveal  bool   `json:"isReveal"`
	ExpiresAt string `json:"expiresAt"`
}

// AgentHost is an app a coding agent harness ran in, as Settings → Coding agents lists it.
type AgentHost struct {
	Path    string `json:"path"`
	TeamID  string `json:"teamID"`
	Name    string `json:"name"`
	IconURL string `json:"iconURL"`
}

func (s *Service) getCommandLineApprovals() *clisocket.SessionApprovals {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.commandLineApprovals
}

func (s *Service) describeApp(bundlePath string) (string, string) {
	if s.config.DescribeApp != nil {
		return s.config.DescribeApp(bundlePath)
	}
	return strings.TrimSuffix(filepath.Base(bundlePath), ".app"), ""
}

// ListAgentSessions lists the running coding agent harnesses that sent hisar requests, in the order they first did.
func (s *Service) ListAgentSessions() []AgentSession {
	approvals := s.getCommandLineApprovals()
	if approvals == nil {
		return []AgentSession{}
	}
	liveSessions := approvals.ListAgentSessions()
	sessions := make([]AgentSession, 0, len(liveSessions))
	for _, live := range liveSessions {
		listed := AgentSession{
			PID: live.Session.PID, StartTime: live.Session.StartTime, HarnessName: live.Session.Harness.Name, IsSigned: live.Session.IsSigned,
			ProjectPath: live.Session.Directory, FirstRequestAt: live.FirstRequestAt.UTC().Format(time.RFC3339),
			RememberedSecrets: make([]RememberedAgentSecret, 0, len(live.RememberedSecrets)),
		}
		if live.Session.Directory != "" {
			listed.ProjectName = filepath.Base(live.Session.Directory)
		}
		if live.Host != "" {
			listed.HostName, listed.HostIconURL = s.describeApp(live.Host)
		}
		for _, remembered := range live.RememberedSecrets {
			listed.RememberedSecrets = append(listed.RememberedSecrets, RememberedAgentSecret{
				Name: remembered.Secret.Describe(), IsReveal: remembered.Kind == clisocket.AgentRequestReveal, ExpiresAt: remembered.ExpiresAt.UTC().Format(time.RFC3339),
			})
		}
		sessions = append(sessions, listed)
	}
	return sessions
}

// RevokeAgentSession ends the session of the harness process (PID and StartTime from ListAgentSessions): its open
// approval windows close, it forgets its secrets, and its next request asks again. It returns the sessions left.
func (s *Service) RevokeAgentSession(pid int, startTime int64) []AgentSession {
	if approvals := s.getCommandLineApprovals(); approvals != nil {
		approvals.RevokeAgentSession(pid, startTime)
	}
	return s.ListAgentSessions()
}

// ListAgentHosts lists the apps a coding agent harness ran in, in the order Hisar first saw one there.
func (s *Service) ListAgentHosts() []AgentHost {
	s.mutex.Lock()
	savedHosts := slices.Clone(s.state.AgentHosts)
	s.mutex.Unlock()
	hosts := make([]AgentHost, 0, len(savedHosts))
	for _, savedHost := range savedHosts {
		host := AgentHost{Path: savedHost.Path, TeamID: savedHost.TeamID}
		host.Name, host.IconURL = s.describeApp(savedHost.Path)
		hosts = append(hosts, host)
	}
	return hosts
}

// ForgetAgentHost lets the app's command-line callers without a terminal share one approval again, and the app be a
// launcher again, until a coding agent harness runs in it again. It returns the hosts left.
func (s *Service) ForgetAgentHost(path, teamID string) ([]AgentHost, error) {
	s.mutex.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mutex.Unlock()
		return nil, err
	}
	forgotten := localstate.AgentHost{Path: path, TeamID: teamID}
	s.state.AgentHosts = slices.DeleteFunc(slices.Clone(s.state.AgentHosts), func(host localstate.AgentHost) bool { return host == forgotten })
	err := s.state.Save()
	s.mutex.Unlock()
	if approvals := s.getCommandLineApprovals(); approvals != nil {
		approvals.ForgetAgentHost(clisocket.AppIdentity{Path: path, TeamID: teamID})
	}
	return s.ListAgentHosts(), err
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
