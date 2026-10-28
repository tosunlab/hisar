package app

import (
	"cmp"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"time"

	"github.com/tryhisar/hisar/core/agentharness"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/crypt"
)

// ListAgentGrants returns this account's unlocked grants. Sealed state and a separate
// head preserve them across restarts without accepting a restored older state file.
func (bridge *CommandLineBridge) ListAgentGrants() []clisocket.AgentGrant {
	s := bridge.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.unlockedSession == nil && s.lockedGrants != nil {
		grants := make([]clisocket.AgentGrant, 0, len(s.lockedGrants.entries))
		for _, entry := range s.lockedGrants.entries {
			grants = append(grants, entry.grant)
		}
		return grants
	}
	return slices.Clone(s.agentGrants)
}

// AddAgentGrants commits only while authorization holds. Revocation does not take
// s.mutex, so authorization is checked again after both durable writes and a stale
// grant is removed before it becomes usable.
func (bridge *CommandLineBridge) AddAgentGrants(ctx context.Context, authorization clisocket.AgentAuthorization, grants []clisocket.AgentGrant) error {
	s, err := bridge.requestService(ctx)
	if err != nil {
		return clisocket.ErrAgentAuthorizationEnded
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.requireAgentAuthorization(authorization) != nil {
		return clisocket.ErrAgentAuthorizationEnded
	}
	if s.unlockedSession == nil {
		return ErrLocked
	}
	now := s.config.Now()
	previous := s.agentGrants
	kept := slices.DeleteFunc(slices.Clone(previous), func(current clisocket.AgentGrant) bool {
		return !current.ExpiresAt.IsZero() && !now.Before(current.ExpiresAt) || slices.ContainsFunc(grants, func(grant clisocket.AgentGrant) bool {
			return current.HarnessID == grant.HarnessID && current.GetKey() == grant.GetKey()
		})
	})
	err = s.saveAgentGrants(append(kept, grants...))
	if s.requireAgentAuthorization(authorization) == nil {
		return err
	}
	if err := s.saveAgentGrants(previous); err != nil {
		log.Printf("agent grants: dropping the saved grants after an approval ended: %v", err)
		s.agentGrants, s.state.SealedAgentGrants = previous, nil
		if err := s.state.Save(); err != nil {
			log.Printf("agent grants: %v", err)
		}
	}
	return clisocket.ErrAgentAuthorizationEnded
}

// CanSaveAgentGrants reports whether grants can outlive a restart: Hisar has a head store, and it worked last time.
func (bridge *CommandLineBridge) CanSaveAgentGrants() bool {
	s := bridge.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.config.AgentGrantHeads != nil && !s.isAgentGrantHeadUnavailable
}

// forgetAgentAccount requires s.mutex and ends the account's agent requests and grants.
func (s *Service) forgetAgentAccount() {
	s.dropLockedGrantView()
	if s.commandLineApprovals != nil {
		s.commandLineApprovals.SetAccount("")
	}
	if heads := s.config.AgentGrantHeads; heads != nil {
		if err := s.noteAgentGrantHeadResult(heads.DeleteAgentGrantHead()); err != nil {
			log.Printf("agent grants: %v", err)
		}
	}
}

func (bridge *CommandLineBridge) RemoveAgentGrant(harnessID, key string) error {
	s := bridge.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.unlockedSession == nil {
		return s.dropAgentGrants()
	}
	grants := slices.DeleteFunc(slices.Clone(s.agentGrants), func(grant clisocket.AgentGrant) bool {
		return grant.HarnessID == harnessID && grant.GetKey() == key
	})
	if len(grants) == len(s.agentGrants) {
		return nil
	}
	if err := s.saveAgentGrants(grants); err != nil {
		return errors.Join(err, s.dropAgentGrants())
	}
	return nil
}

// dropAgentGrants needs s.mutex. Without the account key, removing one sealed grant must drop the list. Both stores
// are cleared even if one fails, and the in-memory state never reopens the revoked grants.
func (s *Service) dropAgentGrants() error {
	s.dropLockedGrantView()
	s.agentGrants, s.state.SealedAgentGrants = nil, nil
	var headError error
	if heads := s.config.AgentGrantHeads; heads != nil {
		headError = s.noteAgentGrantHeadResult(heads.DeleteAgentGrantHead())
	}
	return errors.Join(headError, s.state.Save())
}

type savedAgentGrant struct {
	HarnessID         string    `json:"harnessId"`
	HarnessName       string    `json:"harnessName"`
	ProgramIdentifier string    `json:"programIdentifier,omitempty"`
	ProgramTeamID     string    `json:"programTeamId,omitempty"`
	ProgramPath       string    `json:"programPath,omitempty"`
	ProgramEntrypoint string    `json:"programEntrypoint,omitempty"`
	Kind              string    `json:"kind"`
	VaultID           string    `json:"vaultId"`
	ItemID            string    `json:"itemId"`
	FieldLabel        string    `json:"fieldLabel,omitempty"`
	Origin            string    `json:"origin,omitempty"`
	ExpiresAt         time.Time `json:"expiresAt,omitzero"`
}

type sealedAgentGrants struct {
	Head   []byte            `json:"head"`
	Grants []savedAgentGrant `json:"grants"`
}

// saveAgentGrants needs s.mutex and an unlocked session: it seals grants into the device state and keeps them open.
// The new head is in the head store before the device state changes, so a crash between them loses the grants rather
// than keeping older ones; no grant left deletes the head. When the head store fails, neither the open grants nor the
// device state change.
func (s *Service) saveAgentGrants(grants []clisocket.AgentGrant) error {
	heads := s.config.AgentGrantHeads
	if heads == nil {
		return ErrAgentGrantHeadUnavailable
	}
	var sealed []byte
	if len(grants) == 0 {
		if err := s.noteAgentGrantHeadResult(heads.DeleteAgentGrantHead()); err != nil {
			return err
		}
	} else {
		contents := sealedAgentGrants{Head: make([]byte, 32), Grants: make([]savedAgentGrant, 0, len(grants))}
		rand.Read(contents.Head)
		for _, grant := range grants {
			contents.Grants = append(contents.Grants, savedAgentGrant{
				HarnessID: grant.HarnessID, HarnessName: grant.HarnessName, ProgramIdentifier: grant.Program.Signature.Identifier,
				ProgramTeamID: grant.Program.Signature.TeamID, ProgramPath: grant.Program.Path, ProgramEntrypoint: grant.Program.Entrypoint,
				Kind: grant.Kind, VaultID: grant.Secret.VaultID,
				ItemID: grant.Secret.ItemID, FieldLabel: grant.Secret.FieldLabel, Origin: grant.Secret.Origin, ExpiresAt: grant.ExpiresAt.UTC(),
			})
		}
		plaintext, err := json.Marshal(contents)
		if err != nil {
			return err
		}
		defer clear(plaintext)
		if sealed, err = crypt.SealAgentGrants(s.unlockedSession.Keyset.UserKey, s.unlockedSession.AccountID, plaintext); err != nil {
			return err
		}
		if err := s.noteAgentGrantHeadResult(heads.SaveAgentGrantHead(contents.Head)); err != nil {
			return err
		}
	}
	s.agentGrants, s.state.SealedAgentGrants = grants, sealed
	return s.state.Save()
}

// openAgentGrants needs s.mutex and an unlocked session: it opens the grants this account sealed into the device state
// with the head the head store keeps. Grants that do not open are dropped with the head: ones another program wrote
// into the file, an older copy of the file restored over it, or any grant when the head store keeps no head or cannot
// be read. A head another program rewrote, which would ask the user at every unlock, goes with them.
func (s *Service) openAgentGrants() {
	s.agentGrants = []clisocket.AgentGrant{}
	saved, err := s.readAgentGrants()
	if err != nil {
		log.Printf("agent grants: dropping the saved grants: %v", err)
		if heads := s.config.AgentGrantHeads; heads != nil {
			if err := s.noteAgentGrantHeadResult(heads.DeleteAgentGrantHead()); err != nil {
				log.Printf("agent grants: %v", err)
			}
		}
		s.state.SealedAgentGrants = nil
		if err := s.state.Save(); err != nil {
			log.Printf("agent grants: %v", err)
		}
		return
	}
	for _, entry := range saved {
		s.agentGrants = append(s.agentGrants, clisocket.AgentGrant{
			HarnessID: entry.HarnessID, HarnessName: entry.HarnessName, Kind: entry.Kind, ExpiresAt: entry.ExpiresAt,
			Program: agentharness.Program{
				Signature: agentharness.Signature{Identifier: entry.ProgramIdentifier, TeamID: entry.ProgramTeamID}, Path: entry.ProgramPath,
				Entrypoint: entry.ProgramEntrypoint,
			},
			Secret: clisocket.AgentSecret{VaultID: entry.VaultID, ItemID: entry.ItemID, FieldLabel: entry.FieldLabel, Origin: entry.Origin},
		})
	}
}

// readAgentGrants needs s.mutex and an unlocked session. It reads the head at every unlock, grants or not, so
// CanSaveAgentGrants knows whether the head store works.
func (s *Service) readAgentGrants() ([]savedAgentGrant, error) {
	heads := s.config.AgentGrantHeads
	if heads == nil {
		if len(s.state.SealedAgentGrants) == 0 {
			return nil, nil
		}
		return nil, ErrAgentGrantHeadUnavailable
	}
	head, err := heads.LoadAgentGrantHead()
	s.noteAgentGrantHeadResult(err)
	if len(s.state.SealedAgentGrants) == 0 {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	plaintext, err := crypt.OpenAgentGrants(s.unlockedSession.Keyset.UserKey, s.unlockedSession.AccountID, s.state.SealedAgentGrants)
	if err != nil {
		return nil, fmt.Errorf("this account did not seal them: %w", err)
	}
	defer clear(plaintext)
	var contents sealedAgentGrants
	if err := json.Unmarshal(plaintext, &contents); err != nil {
		return nil, err
	}
	if subtle.ConstantTimeCompare(contents.Head, head) != 1 {
		return nil, errAgentGrantsNotCurrent
	}
	return contents.Grants, nil
}

var errAgentGrantsNotCurrent = errors.New("they are not the grants Hisar saved last")

// noteAgentGrantHeadResult requires s.mutex.
func (s *Service) noteAgentGrantHeadResult(err error) error {
	s.isAgentGrantHeadUnavailable = errors.Is(err, ErrAgentGrantHeadUnavailable)
	return err
}

// AgentGrant is what a coding agent may do without asking, as Settings → Coding agents lists it: use (Kind run or code),
// read (reveal) or sign in with (sign-in, at Website only) the item Name names. Scope says who holds it: a recognized
// harness in every session ("harness"), one agent session ("session"), or one connection ("connection"), whose grants
// end with it. ID names it for RevokeAgentGrant. ExpiresAt is RFC 3339, "" for a grant that lasts until revoked.
type AgentGrant struct {
	ID          string `json:"id"`
	Scope       string `json:"scope"`
	HarnessName string `json:"harnessName"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Website     string `json:"website"`
	ExpiresAt   string `json:"expiresAt"`
}

// ListAgentGrants lists the grants of coding agents by agent, then by item.
func (s *Service) ListAgentGrants() []AgentGrant {
	approvals := s.getCommandLineApprovals()
	if approvals == nil {
		return []AgentGrant{}
	}
	grants := approvals.ListAgentGrants()
	s.mutex.Lock()
	defer s.mutex.Unlock()
	listed := make([]AgentGrant, 0, len(grants))
	for _, grant := range grants {
		entry := AgentGrant{
			ID: grant.GetID(), Scope: string(grant.GetScope()), HarnessName: grant.HarnessName, Name: s.describeGrantedSecret(grant.Secret, grant.Kind),
			Kind: grant.Kind, Website: grant.Secret.Origin,
		}
		if !grant.ExpiresAt.IsZero() {
			entry.ExpiresAt = grant.ExpiresAt.UTC().Format(time.RFC3339)
		}
		listed = append(listed, entry)
	}
	slices.SortFunc(listed, func(first, second AgentGrant) int {
		return cmp.Or(cmp.Compare(first.HarnessName, second.HarnessName), cmp.Compare(first.Scope, second.Scope), cmp.Compare(first.Name, second.Name),
			cmp.Compare(first.Kind, second.Kind), cmp.Compare(first.Website, second.Website), cmp.Compare(first.ID, second.ID))
	})
	return listed
}

// describeGrantedSecret needs s.mutex: "vault › item › field" from the vaults Hisar has open, since the device state
// keeps only IDs, or while locked as the grant named it when approved; a sign-in names the item.
func (s *Service) describeGrantedSecret(secret clisocket.AgentSecret, kind string) string {
	if kind == clisocket.AgentRequestSignIn {
		secret.FieldLabel = ""
	}
	if s.unlockedSession == nil && secret.ItemTitle != "" {
		return secret.Describe()
	}
	vault, isVaultOpen := s.vaultsByID[secret.VaultID]
	vaultItem, err := s.findItem(secret.VaultID, secret.ItemID)
	if !isVaultOpen || vault == nil || err != nil {
		return "An item no longer available"
	}
	secret.VaultName, secret.ItemTitle = vault.Name, vaultItem.Overview.Title
	return secret.Describe()
}

// RevokeAgentGrant ends one grant (ID from ListAgentGrants): its agent asks again next time. It returns the grants left.
func (s *Service) RevokeAgentGrant(id string) ([]AgentGrant, error) {
	approvals := s.getCommandLineApprovals()
	if approvals == nil {
		return []AgentGrant{}, nil
	}
	err := approvals.RevokeAgentGrant(id)
	return s.ListAgentGrants(), err
}
