package session

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
)

var (
	ErrNotVaultOwner      = errors.New("only an owner of the vault can do that")
	ErrPersonalVault      = errors.New("your Personal vault can't be deleted: it is the vault Hisar keeps for you and nobody else")
	ErrTargetNotWritable  = errors.New("choose a vault you can add items to")
	ErrVaultNameEmpty     = errors.New("enter a vault name")
	ErrVaultNotEmpty      = errors.New("the vault has items; move them to another vault or delete them with the vault")
	ErrLeavePersonalVault = errors.New("you can't leave your Personal vault")
	// ErrOrganizationVaultAccess refuses leaving an organization vault or removing someone from it.
	ErrOrganizationVaultAccess = errors.New("access to an organization vault comes from its teams; ask an organization admin to change them")
)

func (s *Session) CreateVault(ctx context.Context, name, vaultType string) (*Vault, error) {
	return s.createVault(ctx, name, &apitypes.CreateVaultRequest{Type: vaultType})
}

// CreateOrganizationVault creates a vault of the organization and gives it to the team as owner;
// the caller must be in the team. The team's other members get the key from an owner's app (CompleteVaultKeyRequests).
func (s *Session) CreateOrganizationVault(ctx context.Context, name, organizationID, teamID string) (*Vault, error) {
	return s.createVault(ctx, name, &apitypes.CreateVaultRequest{Type: apitypes.VaultOrganization, OrganizationID: organizationID, TeamID: teamID})
}

func (s *Session) createVault(ctx context.Context, name string, request *apitypes.CreateVaultRequest) (*Vault, error) {
	vaultID := uuid.NewString()
	vaultKey := crypt.GenerateKey()
	encryptedName, err := crypt.SealVaultName(vaultKey, vaultID, []byte(name))
	if err != nil {
		return nil, err
	}
	wrappedKey, err := crypt.WrapVaultKey(s.Keyset.GetPublicKeys(s.AccountID).KEM, vaultID, s.AccountID, vaultKey)
	if err != nil {
		return nil, err
	}
	request.ID, request.EncName, request.WrappedKey = vaultID, encryptedName, wrappedKey
	if err := s.Client.CreateVault(ctx, request); err != nil {
		return nil, err
	}
	return &Vault{
		ID: vaultID, Type: request.Type, Name: name, Role: apitypes.VaultOwner, Key: vaultKey,
		OrganizationID: request.OrganizationID, KeyGeneration: 1,
	}, nil
}

func (s *Session) ListVaults(ctx context.Context) ([]Vault, error) {
	protoVaults, err := s.listProtoVaults(ctx)
	if err != nil {
		return nil, err
	}
	vaults := make([]Vault, 0, len(protoVaults))
	for _, protoVault := range protoVaults {
		vaultKey, err := s.Keyset.UnwrapVaultKey(protoVault.ID, s.AccountID, protoVault.WrappedKey)
		if err != nil {
			return nil, fmt.Errorf("vault %s: %w", protoVault.ID, err)
		}
		name, err := crypt.OpenVaultName(vaultKey, protoVault.ID, protoVault.EncName)
		if err != nil {
			return nil, fmt.Errorf("vault %s: %w", protoVault.ID, err)
		}
		vaults = append(vaults, Vault{
			ID: protoVault.ID, Type: protoVault.Type, Name: string(name), Role: protoVault.Role, Key: vaultKey,
			OrganizationID: protoVault.OrganizationID, KeyGeneration: protoVault.KeyGeneration, NeedsRotation: protoVault.NeedsRotation,
		})
	}
	return vaults, nil
}

func (s *Session) FindVault(ctx context.Context, nameOrID string) (*Vault, error) {
	vaults, err := s.ListVaults(ctx)
	if err != nil {
		return nil, err
	}
	for i := range vaults {
		if vaults[i].ID == nameOrID || strings.EqualFold(vaults[i].Name, nameOrID) {
			return &vaults[i], nil
		}
	}
	return nil, fmt.Errorf("vault %q: %w", nameOrID, ErrNotFound)
}

// RemovedVault is a cached vault that the server no longer lists: someone deleted it, or the user lost access.
// Name is empty when the cached key no longer opens it.
type RemovedVault struct {
	ID   string
	Name string
}

func (s *Session) noteRemovedVaults(cachedVaults, currentVaults []apitypes.Vault) {
	currentIDs := map[string]bool{}
	for _, vault := range currentVaults {
		currentIDs[vault.ID] = true
	}
	s.removalMutex.Lock()
	defer s.removalMutex.Unlock()
	for _, cachedVault := range cachedVaults {
		if currentIDs[cachedVault.ID] {
			continue
		}
		if s.ownRemovalIDs[cachedVault.ID] {
			delete(s.ownRemovalIDs, cachedVault.ID)
			continue
		}
		removed := RemovedVault{ID: cachedVault.ID}
		if s.Keyset != nil {
			if vaultKey, err := s.Keyset.UnwrapVaultKey(cachedVault.ID, s.AccountID, cachedVault.WrappedKey); err == nil {
				if name, err := crypt.OpenVaultName(vaultKey, cachedVault.ID, cachedVault.EncName); err == nil {
					removed.Name = string(name)
				}
				clear(vaultKey)
			}
		}
		s.removedVaults = append(s.removedVaults, removed)
	}
}

func (s *Session) noteOwnRemoval(vaultID string) {
	s.removalMutex.Lock()
	defer s.removalMutex.Unlock()
	if s.ownRemovalIDs == nil {
		s.ownRemovalIDs = map[string]bool{}
	}
	s.ownRemovalIDs[vaultID] = true
}

// TakeRemovedVaults returns the vaults that disappeared since the last call, other than those this session deleted or left.
func (s *Session) TakeRemovedVaults() []RemovedVault {
	s.removalMutex.Lock()
	defer s.removalMutex.Unlock()
	removed := s.removedVaults
	s.removedVaults = nil
	return removed
}

// VaultDeletion says what happens to a vault's items: moved into MoveTo, or deleted with the vault when IsDeletingItems.
// An empty vault needs neither.
type VaultDeletion struct {
	MoveTo          *Vault
	IsDeletingItems bool
}

// RenameVault re-encrypts the vault name under the vault key; the server only stores the new ciphertext, and refuses it
// (409) when the vault's key was rotated since this copy was listed.
func (s *Session) RenameVault(ctx context.Context, vault *Vault, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrVaultNameEmpty
	}
	if vault.Role != apitypes.VaultOwner {
		return ErrNotVaultOwner
	}
	encryptedName, err := crypt.SealVaultName(vault.Key, vault.ID, []byte(name))
	if err != nil {
		return err
	}
	request := &apitypes.RenameVaultRequest{EncName: encryptedName, KeyGeneration: vault.KeyGeneration}
	if err := s.Client.RenameVault(ctx, vault.ID, request); err != nil {
		return err
	}
	vault.Name = name
	return nil
}

// LeaveVault removes the caller's own access and wrapped key.
func (s *Session) LeaveVault(ctx context.Context, vault *Vault) error {
	if vault.Type == apitypes.VaultPersonal {
		return ErrLeavePersonalVault
	}
	if vault.Type == apitypes.VaultOrganization {
		return ErrOrganizationVaultAccess
	}
	if err := s.Client.RemoveAccess(ctx, vault.ID, s.AccountID); err != nil {
		return err
	}
	s.noteOwnRemoval(vault.ID)
	return nil
}

// DeleteVault deletes a vault the caller owns. Moving re-encrypts every live item under the target vault's key here,
// so the server never sees plaintext, and the server moves them and deletes the vault in one transaction.
// It returns how many items were moved, and retries when the vault changes during the move. When another device rotated
// the target's key since it was listed, the target is reloaded and the items sealed again.
func (s *Session) DeleteVault(ctx context.Context, vault *Vault, deletion VaultDeletion) (int, error) {
	if vault.Role != apitypes.VaultOwner {
		return 0, ErrNotVaultOwner
	}
	if vault.Type == apitypes.VaultPersonal {
		return 0, ErrPersonalVault
	}
	if deletion.MoveTo == nil {
		err := s.Client.DeleteVault(ctx, vault.ID, &apitypes.DeleteVaultRequest{IsDeletingItems: deletion.IsDeletingItems})
		if apiclient.IsStatus(err, http.StatusConflict) {
			return 0, ErrVaultNotEmpty
		}
		if err == nil {
			s.noteOwnRemoval(vault.ID)
		}
		return 0, err
	}
	target := deletion.MoveTo
	if target.ID == vault.ID || !apitypes.CanWriteItems(target.Role) {
		return 0, ErrTargetNotWritable
	}
	for attempt := 1; ; attempt++ {
		var request *apitypes.DeleteVaultRequest
		err := s.sendSealed(ctx, func() error {
			var err error
			if request, err = s.buildVaultMove(ctx, vault, target); err != nil {
				return err
			}
			return s.Client.DeleteVault(ctx, vault.ID, request)
		}, target)
		if apiclient.IsStatus(err, http.StatusConflict) && attempt < maxRotationAttemptCount {
			continue
		}
		if err != nil {
			return 0, err
		}
		s.noteOwnRemoval(vault.ID)
		return len(request.MovedItems), nil
	}
}

func (s *Session) buildVaultMove(ctx context.Context, vault, target *Vault) (*apitypes.DeleteVaultRequest, error) {
	protoItems, err := s.listProtoItems(ctx, vault)
	if err != nil {
		return nil, err
	}
	request := &apitypes.DeleteVaultRequest{MoveToVaultID: target.ID, MoveToKeyGeneration: target.KeyGeneration, MovedItems: []apitypes.MovedItem{}}
	for index := range protoItems {
		protoItem := &protoItems[index]
		if protoItem.Deleted {
			continue
		}
		// The item keeps its ID; the new AAD binds the target vault ID and a new revision.
		moved, err := reencryptItem(vault.Key, vault.ID, target.Key, target.ID, protoItem)
		if err != nil {
			return nil, err
		}
		request.MovedItems = append(request.MovedItems, apitypes.MovedItem{
			ID: protoItem.ID, BaseRevision: protoItem.Revision, Revision: moved.revision, Overview: moved.overview, Details: moved.details,
		})
	}
	return request, nil
}
