package app

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/agentharness"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/secretreference"
	"github.com/tryhisar/hisar/core/signin"
	"github.com/tryhisar/hisar/core/totp"
)

// lockedGrantView owns only the values covered by saved Always grants, never account or vault keys.
// All access and erasure happen under the service mutex.
type lockedGrantView struct {
	generation       uint64
	entries          []lockedGrant
	vaultIDs         []string
	itemIDsByVaultID map[string][]string
}

type lockedGrant struct {
	grant        clisocket.AgentGrant
	references   []secretreference.Reference
	builtInField string
	value        []byte
	otp          []byte
	username     []byte
	password     []byte
	revision     string
}

// AlwaysAgentAccessWhileLocked reports whether this device can keep Always permissions active while locked.
type AlwaysAgentAccessWhileLocked struct {
	IsAvailable bool `json:"isAvailable"`
	IsEnabled   bool `json:"isEnabled"`
}

// GetAlwaysAgentAccessWhileLocked returns this device's availability and setting for locked Always access.
func (s *Service) GetAlwaysAgentAccessWhileLocked() AlwaysAgentAccessWhileLocked {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.alwaysAgentAccessWhileLocked()
}

func (s *Service) alwaysAgentAccessWhileLocked() AlwaysAgentAccessWhileLocked {
	return AlwaysAgentAccessWhileLocked{
		IsAvailable: s.config.AgentGrantHeads != nil && !s.isAgentGrantHeadUnavailable,
		IsEnabled:   s.state.IsAlwaysAgentAccessWhileLockedEnabled,
	}
}

// SetAlwaysAgentAccessWhileLocked saves the setting and erases retained secrets when disabled.
func (s *Service) SetAlwaysAgentAccessWhileLocked(isEnabled bool) (AlwaysAgentAccessWhileLocked, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if isEnabled && !s.alwaysAgentAccessWhileLocked().IsAvailable {
		return s.alwaysAgentAccessWhileLocked(), ErrAgentGrantHeadUnavailable
	}
	s.state.IsAlwaysAgentAccessWhileLockedEnabled = isEnabled
	if !isEnabled {
		s.dropLockedGrantView()
	}
	return s.alwaysAgentAccessWhileLocked(), s.state.Save()
}

func (s *Service) dropLockedGrantView() {
	s.lockedGrantGeneration.Add(1)
	if s.lockedGrants == nil {
		return
	}
	for _, entry := range s.lockedGrants.entries {
		clear(entry.value)
		clear(entry.otp)
		clear(entry.username)
		clear(entry.password)
	}
	s.lockedGrants = nil
	s.cancelAgentSignIns()
}

func (s *Service) buildLockedGrantView() {
	s.dropLockedGrantView()
	if !s.state.IsAlwaysAgentAccessWhileLockedEnabled || !s.alwaysAgentAccessWhileLocked().IsAvailable ||
		!slices.ContainsFunc(s.agentGrants, func(grant clisocket.AgentGrant) bool { return grant.ExpiresAt.IsZero() }) {
		return
	}
	source := agentSecretSource{serviceSecretSource{service: s, isMemoryOnly: true}}
	inventory, err := source.ListVaults()
	if err != nil {
		return
	}
	view := &lockedGrantView{generation: s.lockedGrantGeneration.Load(), itemIDsByVaultID: map[string][]string{}}
	for _, vault := range inventory.Vaults {
		view.vaultIDs = append(view.vaultIDs, vault.ID)
	}
	for _, grant := range s.agentGrants {
		if !grant.ExpiresAt.IsZero() || !grant.Program.IsKnown() || !s.isAgentVault(grant.Secret.VaultID) {
			continue
		}
		if _, isRead := view.itemIDsByVaultID[grant.Secret.VaultID]; !isRead {
			vault := s.vaultsByID[grant.Secret.VaultID]
			if vault == nil {
				continue
			}
			if _, isLoaded := s.itemsByVaultID[vault.ID]; !isLoaded {
				continue
			}
			view.itemIDsByVaultID[vault.ID] = []string{}
			for _, vaultItem := range s.itemsByVaultID[vault.ID] {
				view.itemIDsByVaultID[vault.ID] = append(view.itemIDsByVaultID[vault.ID], vaultItem.ID)
			}
		}
		items, err := source.ListItems(grant.Secret.VaultID)
		if err != nil {
			continue
		}
		index := slices.IndexFunc(items.Items, func(candidate item.Item) bool { return candidate.ID == grant.Secret.ItemID })
		if index < 0 {
			continue
		}
		vaultItem := &items.Items[index]
		vault := s.vaultsByID[grant.Secret.VaultID]
		grant.Secret.VaultName, grant.Secret.ItemTitle = vault.Name, vaultItem.Overview.Title
		entry := lockedGrant{grant: grant}
		if grant.Kind == clisocket.AgentRequestSignIn {
			origin, err := signin.ParseOrigin(grant.Secret.Origin)
			if err != nil || !s.isAgentLoginForOrigin(vaultItem, origin) {
				continue
			}
			entry.username, entry.password, entry.otp = []byte(vaultItem.Overview.Username), []byte(vaultItem.Details.Password), []byte(vaultItem.Details.OneTimePassword)
			entry.revision = vaultItem.Revision
		} else if grant.Kind == clisocket.AgentRequestReveal && grant.Secret.FieldLabel == "" {
			entry.value, err = json.Marshal(cliprotocol.ItemResult{Item: convertToCommandLineItem(s.describeItem(vaultItem))})
			if err != nil {
				continue
			}
		} else {
			for _, vaultName := range []string{vault.ID, vault.Name} {
				for _, itemName := range []string{vaultItem.ID, vaultItem.Overview.Title} {
					reference := secretreference.Reference{Vault: vaultName, Item: itemName, Field: grant.Secret.FieldLabel}
					resolved, err := secretreference.ResolveAll(source, []string{reference.String()}, s.config.Now())
					if err != nil || resolved[0].VaultID != grant.Secret.VaultID || resolved[0].ItemID != grant.Secret.ItemID || !strings.EqualFold(resolved[0].FieldLabel, grant.Secret.FieldLabel) {
						continue
					}
					entry.references = append(entry.references, reference)
					if entry.value == nil && entry.otp == nil {
						entry.builtInField, _ = secretreference.GetBuiltInField(resolved[0].FieldLabel)
						if entry.builtInField == secretreference.FieldConnectionString && vaultItem.ConnectionString() == "" {
							entry.builtInField = ""
						}
						if entry.builtInField == secretreference.FieldOneTimeCode {
							entry.otp = []byte(vaultItem.Details.OneTimePassword)
						} else {
							entry.value = []byte(resolved[0].Value)
						}
					}
				}
			}
			if len(entry.references) == 0 {
				continue
			}
		}
		view.entries = append(view.entries, entry)
	}
	s.lockedGrants = view
}

func (entry *lockedGrant) covers(harness agentharness.Session, kind string) bool {
	grant := entry.grant
	return grant.HarnessID == harness.Harness.ID && grant.Program == harness.Program &&
		(grant.Kind == kind || (grant.Kind == clisocket.AgentRequestRun || grant.Kind == clisocket.AgentRequestCode) && (kind == clisocket.AgentRequestRun || kind == clisocket.AgentRequestCode))
}

func (entry *lockedGrant) matches(reference secretreference.Reference, view *lockedGrantView) bool {
	if slices.Contains(view.vaultIDs, reference.Vault) && reference.Vault != entry.grant.Secret.VaultID ||
		slices.Contains(view.itemIDsByVaultID[entry.grant.Secret.VaultID], reference.Item) && reference.Item != entry.grant.Secret.ItemID {
		return false
	}
	field := reference.Field
	if entry.builtInField != "" {
		if builtin, _ := secretreference.GetBuiltInField(field); builtin != entry.builtInField {
			return false
		}
	} else if !strings.EqualFold(entry.grant.Secret.FieldLabel, field) {
		return false
	}
	for _, form := range entry.references {
		vaultMatches := form.Vault == reference.Vault || form.Vault != entry.grant.Secret.VaultID && strings.EqualFold(form.Vault, reference.Vault)
		itemMatches := form.Item == reference.Item || form.Item != entry.grant.Secret.ItemID && strings.EqualFold(form.Item, reference.Item)
		if vaultMatches && itemMatches {
			return true
		}
	}
	return false
}

// AnswerLockedAgentRequest is all-or-nothing and never falls back to the unlocked resolver.
func (bridge *CommandLineBridge) AnswerLockedAgentRequest(ctx context.Context, request *cliprotocol.Request, kind string, harness agentharness.Session, requestAuthorization *clisocket.AgentAuthorization) (answer any, covered bool, err error) {
	authorization := *requestAuthorization
	s, err := bridge.requestService(ctx)
	if err != nil {
		return nil, false, err
	}
	s.mutex.Lock()
	defer func() {
		s.mutex.Unlock()
		if covered {
			*requestAuthorization = authorization
		}
		if covered && (!authorization.IsCurrent() || !s.isSessionEpoch(*s.requestEpoch)) {
			answer, err = nil, errCommandLineSessionEnded
		}
	}()
	if err := s.requireAgentAuthorization(authorization); err != nil {
		return nil, false, err
	}
	if s.unlockedSession != nil || s.lockedGrants == nil {
		return nil, false, nil
	}
	authorization = authorization.BindLockedGrantGeneration(&s.lockedGrantGeneration, s.lockedGrants.generation)
	entries := s.lockedGrants.entries
	if request.Type == cliprotocol.TypeGetItem {
		for _, entry := range entries {
			if entry.covers(harness, kind) && entry.grant.Secret.FieldLabel == "" && entry.grant.Secret.VaultID == request.VaultID && entry.grant.Secret.ItemID == request.ItemID {
				var result cliprotocol.ItemResult
				err := json.Unmarshal(entry.value, &result)
				return result, true, err
			}
		}
		return nil, false, nil
	}
	references := request.References
	if kind == clisocket.AgentRequestCode && request.ItemID != "" {
		for _, entry := range entries {
			if entry.covers(harness, kind) && entry.grant.Secret.ItemID == request.ItemID && len(entry.otp) != 0 {
				references = []string{secretreference.Reference{Vault: entry.grant.Secret.VaultID, Item: request.ItemID, Field: secretreference.FieldOneTimeCode}.String()}
				break
			}
		}
	}
	if len(references) == 0 {
		return nil, false, nil
	}
	matched := make([]*lockedGrant, 0, len(references))
	for _, text := range references {
		reference, err := secretreference.Parse(text)
		if err != nil {
			return nil, false, nil
		}
		if kind == clisocket.AgentRequestCode {
			reference.Field = secretreference.FieldOneTimeCode
		}
		index := slices.IndexFunc(entries, func(entry lockedGrant) bool {
			return entry.covers(harness, kind) && entry.matches(reference, s.lockedGrants)
		})
		if index < 0 {
			return nil, false, nil
		}
		matched = append(matched, &entries[index])
	}
	result := cliprotocol.ResolveSecretResult{Values: make([]string, 0, len(matched))}
	for _, entry := range matched {
		value := string(entry.value)
		if len(entry.otp) != 0 {
			config, err := totp.Parse(string(entry.otp))
			if err != nil {
				return nil, true, err
			}
			code, expiresAt := config.GenerateCode(s.config.Now())
			if kind == clisocket.AgentRequestCode {
				return cliprotocol.CodeResult{Code: code, ExpiresAt: expiresAt.UTC().Format(time.RFC3339), Period: config.Period}, true, nil
			}
			value = code
		}
		result.Values = append(result.Values, value)
	}
	return result, true, nil
}

// PrepareLockedAgentSignIn offers a single login covered by the harness's retained Always grant at this origin.
// The boolean reports whether that login can be used without unlocking.
func (bridge *CommandLineBridge) PrepareLockedAgentSignIn(ctx context.Context, request *cliprotocol.Request, target clisocket.AgentSignInTarget, harness agentharness.Session, authorization clisocket.AgentAuthorization) (clisocket.AgentSignInTarget, bool, error) {
	s, err := bridge.requestService(ctx)
	if err != nil {
		return target, false, err
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireAgentAuthorization(authorization); err != nil {
		return target, false, err
	}
	if s.unlockedSession != nil || s.lockedGrants == nil {
		return target, false, nil
	}
	for _, entry := range s.lockedGrants.entries {
		if entry.covers(harness, clisocket.AgentRequestSignIn) && entry.grant.Secret.Origin == target.Destination.Origin.String() && (request.ItemID == "" || request.ItemID == entry.grant.Secret.ItemID) {
			target.Logins = append(target.Logins, clisocket.AgentLogin{AgentSecret: entry.grant.Secret, Username: string(entry.username), Revision: entry.revision})
		}
	}
	if len(target.Logins) != 1 {
		target.Logins = nil
		return target, false, nil
	}
	target.LockedGrantGeneration = s.lockedGrants.generation
	return target, true, nil
}

func (s *Service) readLockedAgentSignInLogin(target clisocket.AgentSignInTarget, login clisocket.AgentLogin) (signin.Login, error) {
	if s.lockedGrants == nil || s.lockedGrants.generation != target.LockedGrantGeneration {
		return signin.Login{}, errCommandLineSessionEnded
	}
	for _, entry := range s.lockedGrants.entries {
		if entry.grant.Kind == clisocket.AgentRequestSignIn && entry.grant.Secret == login.AgentSecret && entry.revision == login.Revision {
			return signin.Login{Username: string(entry.username), Password: signin.NewSecret(string(entry.password)), OneTimePassword: signin.NewSecret(string(entry.otp))}, nil
		}
	}
	return signin.Login{}, errCommandLineSessionEnded
}
