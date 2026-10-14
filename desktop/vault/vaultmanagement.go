package vault

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/session"
)

var (
	errConfirmVaultName = errors.New("type the vault's name to delete it together with its items")
	errChooseItemAction = errors.New("this vault has items; move them to another vault or delete them with the vault")
	errLeaveRefused     = errors.New("you can't leave this vault")
)

// VaultActions is what the vault's menu needs: who may rename, delete or leave it, what deleting it means, and why not.
// AllMemberLosingAccess leaves out the current user; AllTargetVault holds the vaults its items can move to.
type VaultActions struct {
	Vault                 VaultSummary   `json:"vault"`
	CanRename             bool           `json:"canRename"`
	CanDelete             bool           `json:"canDelete"`
	DeleteBlockedReason   string         `json:"deleteBlockedReason"`
	CanLeave              bool           `json:"canLeave"`
	LeaveBlockedReason    string         `json:"leaveBlockedReason"`
	AllMemberLosingAccess []VaultMember  `json:"membersLosingAccess"`
	AllTargetVault        []VaultSummary `json:"targetVaults"`
}

// VaultDeletionRequest: MoveToVaultID moves every item there first; IsDeletingItems needs ConfirmedName to be the vault's name.
type VaultDeletionRequest struct {
	MoveToVaultID   string `json:"moveToVaultId"`
	IsDeletingItems bool   `json:"isDeletingItems"`
	ConfirmedName   string `json:"confirmedName"`
}

type VaultDeletionResult struct {
	MovedCount      int    `json:"movedCount"`
	TargetVaultName string `json:"targetVaultName"`
}

// RemovedVaultNotice is a vault that disappeared on a sync: deleted by an owner, or the user's access was removed.
type RemovedVaultNotice struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Message string `json:"message"`
}

func (s *Service) loadVaultItemCount(ctx context.Context, vault *session.Vault) (int, error) {
	allItem, found := s.allItemByVaultID[vault.ID]
	if !found {
		var err error
		if allItem, err = s.unlockedSession.ListAllItem(ctx, vault); err != nil {
			return 0, convertError(err)
		}
		s.allItemByVaultID[vault.ID] = allItem
	}
	return len(allItem), nil
}

func (s *Service) GetVaultActions(vaultID string) (VaultActions, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return VaultActions{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.loadAllVault(ctx); err != nil {
		return VaultActions{}, err
	}
	vault, err := s.findVault(ctx, vaultID)
	if err != nil {
		return VaultActions{}, err
	}
	itemCount, err := s.loadVaultItemCount(ctx, vault)
	if err != nil {
		return VaultActions{}, err
	}
	actions := VaultActions{
		Vault:                 convertToVaultSummary(vault, itemCount),
		AllMemberLosingAccess: []VaultMember{},
		AllTargetVault:        []VaultSummary{},
	}
	allMember := []VaultMember{}
	if vault.Type != apitypes.VaultPersonal {
		if allMember, err = s.listAllVaultMember(ctx, vault.ID); err != nil {
			return VaultActions{}, err
		}
	}
	for _, member := range allMember {
		if !member.IsCurrentUser {
			actions.AllMemberLosingAccess = append(actions.AllMemberLosingAccess, member)
		}
	}

	ownerCount := 0
	for _, member := range allMember {
		if member.Role == apitypes.VaultOwner {
			ownerCount++
		}
	}
	permissions := session.GetVaultPermissions(vault.Type, vault.Role, len(s.allVaultByID), ownerCount)
	actions.CanRename, actions.CanDelete, actions.DeleteBlockedReason = permissions.CanRename, permissions.CanDelete, permissions.DeleteBlockedReason
	actions.CanLeave, actions.LeaveBlockedReason = permissions.CanLeave, permissions.LeaveBlockedReason

	if actions.CanDelete {
		for _, candidate := range s.allVaultByID {
			if candidate.ID == vault.ID || !apitypes.CanWriteItems(candidate.Role) {
				continue
			}
			candidateCount, err := s.loadVaultItemCount(ctx, candidate)
			if err != nil {
				return VaultActions{}, err
			}
			actions.AllTargetVault = append(actions.AllTargetVault, convertToVaultSummary(candidate, candidateCount))
		}
		sortAllVaultSummary(actions.AllTargetVault)
	}
	return actions, nil
}

func (s *Service) RenameVault(vaultID, name string) (VaultSummary, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return VaultSummary{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vault, err := s.findVault(ctx, vaultID)
	if err != nil {
		return VaultSummary{}, err
	}
	if err := s.unlockedSession.RenameVault(ctx, vault, name); err != nil {
		return VaultSummary{}, convertVaultError(err)
	}
	return convertToVaultSummary(vault, len(s.allItemByVaultID[vault.ID])), nil
}

func (s *Service) DeleteVault(vaultID string, request VaultDeletionRequest) (VaultDeletionResult, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return VaultDeletionResult{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := s.loadAllVault(ctx); err != nil {
		return VaultDeletionResult{}, err
	}
	vault, err := s.findVault(ctx, vaultID)
	if err != nil {
		return VaultDeletionResult{}, err
	}
	deletion := session.VaultDeletion{}
	result := VaultDeletionResult{}
	switch {
	case request.MoveToVaultID != "":
		if deletion.MoveTo, err = s.findVault(ctx, request.MoveToVaultID); err != nil {
			return VaultDeletionResult{}, err
		}
		result.TargetVaultName = deletion.MoveTo.Name
	case request.IsDeletingItems:
		if strings.TrimSpace(request.ConfirmedName) != vault.Name {
			return VaultDeletionResult{}, errConfirmVaultName
		}
		deletion.IsDeletingItems = true
	}
	result.MovedCount, err = s.unlockedSession.DeleteVault(ctx, vault, deletion)
	if errors.Is(err, session.ErrVaultNotEmpty) {
		return VaultDeletionResult{}, errChooseItemAction
	}
	if err != nil {
		return VaultDeletionResult{}, convertVaultError(err)
	}
	s.forgetVault(vault.ID)
	if deletion.MoveTo != nil {
		delete(s.allItemByVaultID, deletion.MoveTo.ID)
	}
	return result, nil
}

func (s *Service) LeaveVault(vaultID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vault, err := s.findVault(ctx, vaultID)
	if err != nil {
		return err
	}
	if err := s.unlockedSession.LeaveVault(ctx, vault); err != nil {
		if apiclient.IsStatus(err, http.StatusBadRequest) {
			return errors.New(session.OnlyOwnerLeaveReason)
		}
		return convertVaultError(err)
	}
	s.forgetVault(vault.ID)
	return nil
}

// forgetVault drops a vault this device deleted or left, including its coding agent switch.
func (s *Service) forgetVault(vaultID string) {
	if vault, found := s.allVaultByID[vaultID]; found {
		clear(vault.Key)
		delete(s.allVaultByID, vaultID)
	}
	delete(s.allItemByVaultID, vaultID)
	if slices.Contains(s.state.AllAgentVaultID, vaultID) {
		s.state.AllAgentVaultID = slices.DeleteFunc(s.state.AllAgentVaultID, func(candidate string) bool { return candidate == vaultID })
		s.state.Save()
	}
}

// ListAllRemovedVault returns, once, the vaults that disappeared since the last call because someone else deleted
// them or removed the user.
func (s *Service) ListAllRemovedVault() []RemovedVaultNotice {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	allNotice := []RemovedVaultNotice{}
	if s.unlockedSession == nil {
		return allNotice
	}
	for _, removed := range s.unlockedSession.TakeAllRemovedVault() {
		s.forgetVault(removed.ID)
		notice := RemovedVaultNotice{ID: removed.ID, Name: removed.Name, Message: "A vault is no longer available: it was deleted, or you no longer have access to it."}
		if removed.Name != "" {
			notice.Message = "“" + removed.Name + "” is no longer available: it was deleted, or you no longer have access to it."
		}
		allNotice = append(allNotice, notice)
	}
	return allNotice
}

func convertVaultError(err error) error {
	switch {
	case errors.Is(err, session.ErrNotVaultOwner), errors.Is(err, session.ErrPersonalVault), errors.Is(err, session.ErrTargetNotWritable),
		errors.Is(err, session.ErrVaultNameEmpty), errors.Is(err, session.ErrLeavePersonalVault):
		return err
	case apiclient.IsStatus(err, http.StatusForbidden):
		return errors.New("only an owner of this vault can do that")
	case apiclient.IsStatus(err, http.StatusConflict):
		return errors.New("the vault changed on another device; try again")
	case apiclient.IsStatus(err, http.StatusBadRequest):
		var apiError *apiclient.APIError
		if errors.As(err, &apiError) {
			return errors.New(apiError.Message)
		}
	}
	return convertError(err)
}
