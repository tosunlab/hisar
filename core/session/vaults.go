package session

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
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

// EnsurePersonalVault creates the Personal vault under vaultID unless the account already has one. A sign-up chooses
// vaultID before it registers, so resumes that run at the same time create the same vault and the server refuses all
// but the first; an empty vaultID gets a new one.
func (s *Session) EnsurePersonalVault(ctx context.Context, vaultID string) error {
	if found, err := s.hasPersonalVault(ctx); err != nil || found {
		return err
	}
	_, err := s.createVault(ctx, "Personal", &apitypes.CreateVaultRequest{ID: vaultID, Type: apitypes.VaultPersonal})
	if apiclient.IsStatus(err, http.StatusConflict) {
		if found, listErr := s.hasPersonalVault(ctx); listErr == nil && found {
			return nil
		}
	}
	return err
}

func (s *Session) hasPersonalVault(ctx context.Context) (bool, error) {
	vaults, err := s.Client.ListVaults(ctx)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(vaults, func(vault apitypes.Vault) bool { return vault.Type == apitypes.VaultPersonal }), nil
}

func (s *Session) createVault(ctx context.Context, name string, request *apitypes.CreateVaultRequest) (*Vault, error) {
	vaultID := request.ID
	if vaultID == "" {
		vaultID = uuid.NewString()
	}
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
	record := apitypes.Vault{
		ID: vaultID, Type: request.Type, EncName: encryptedName, Role: apitypes.VaultOwner, WrappedKey: wrappedKey,
		OrganizationID: request.OrganizationID, KeyGeneration: 1,
	}
	return &Vault{
		ID: vaultID, Type: request.Type, Name: name, Role: apitypes.VaultOwner, Key: vaultKey,
		OrganizationID: request.OrganizationID, KeyGeneration: 1, record: record,
	}, nil
}

// ListVaults requires a complete readable inventory. Recovery views use ReadVaults.
func (s *Session) ListVaults(ctx context.Context) ([]Vault, error) {
	result, err := s.ReadVaults(ctx)
	if err != nil {
		return nil, err
	}
	if len(result.Issues) != 0 {
		return nil, ErrIncompleteRead
	}
	return result.Vaults, nil
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
	vault.record.EncName = encryptedName
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
		err := s.sendSealed(ctx, func() error {
			return s.Client.DeleteVault(ctx, vault.ID, &apitypes.DeleteVaultRequest{IsDeletingItems: deletion.IsDeletingItems, KeyGeneration: vault.KeyGeneration})
		}, vault)
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
		op, err := s.Client.BeginVaultTransition(ctx, vault.ID, &apitypes.BeginVaultTransitionRequest{
			Protocol: apitypes.VaultTransitionProtocol, ID: uuid.NewString(), Kind: "delete-move",
			KeyGeneration: vault.KeyGeneration, TargetID: target.ID, TargetKeyGeneration: target.KeyGeneration,
		})
		var moved int
		if err == nil {
			moved, err = s.stageVaultItems(ctx, vault, target, target.Key, op)
			if err == nil {
				_, err = s.commitVaultTransition(ctx, vault.ID, op, nil)
			}
			s.abandonVaultTransition(ctx, vault.ID, op.ID)
		}
		if apiclient.IsStatus(err, http.StatusConflict) && attempt < maxRotationAttemptCount {
			if err := s.reloadVaults(ctx, vault, target); err != nil {
				return 0, err
			}
			continue
		}
		if err != nil {
			return 0, err
		}
		s.noteOwnRemoval(vault.ID)
		return moved, nil
	}
}
