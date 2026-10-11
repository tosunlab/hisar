package vault

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/client"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/core/session"
)

var (
	errPersonalVault       = errors.New("personal vaults are private and cannot be shared; create a shared vault instead")
	errOwnerOnly           = errors.New("only owners of this vault can share it, change roles or remove members")
	errCurrentDevice       = errors.New("this is the device you are using; sign out instead")
	errOwnAccess           = errors.New("you cannot change your own access to a vault")
	errMemberNotFound      = errors.New("this person no longer shares a vault with you")
	errInvalidEmail        = errors.New("enter the email address of the person")
	errEmailNotConfigured  = errors.New("this server cannot send email yet, so invitations cannot be sent")
	errTooManyInvitation   = errors.New("you sent too many invitations today; try again tomorrow")
	errInvitationNotFound  = errors.New("this invitation no longer exists")
	errAlreadyHasAccess    = errors.New("this person already has access to the vault")
	errShareWithOwnAccount = errors.New("that is your own email; you already have access")
)

func isValidVaultRole(role string) bool {
	return role == proto.VaultOwner || role == proto.VaultEditor || role == proto.VaultReader
}

func convertSharingError(err error) error {
	if client.IsStatus(err, http.StatusForbidden) {
		return errOwnerOnly
	}
	return convertError(err)
}

func convertInvitationError(err error) error {
	switch {
	case client.IsStatus(err, http.StatusNotImplemented):
		return errEmailNotConfigured
	case client.IsStatus(err, http.StatusTooManyRequests):
		return errTooManyInvitation
	case client.IsStatus(err, http.StatusNotFound):
		return errInvitationNotFound
	case client.IsStatus(err, http.StatusConflict):
		return errAlreadyHasAccess
	case client.IsStatus(err, http.StatusBadRequest):
		var apiError *client.APIError
		if errors.As(err, &apiError) {
			return errors.New(apiError.Message)
		}
	}
	return convertSharingError(err)
}

func normalizeEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if at := strings.Index(email, "@"); at < 1 || at == len(email)-1 || strings.ContainsAny(email, " <>,") {
		return "", errInvalidEmail
	}
	return email, nil
}

// ListAllPerson returns the user and the people they share a vault with, with the security code of their signing key.
func (s *Service) ListAllPerson() ([]Person, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	allProtoMember, err := s.unlockedSession.Client.ListAllMember(ctx)
	if err != nil {
		return nil, convertError(err)
	}
	allPerson := make([]Person, 0, len(allProtoMember))
	for _, protoMember := range allProtoMember {
		allPerson = append(allPerson, Person{
			AccountID:     protoMember.AccountID,
			Name:          protoMember.Name,
			Email:         protoMember.Email,
			Role:          protoMember.Role,
			SecurityCode:  crypt.FormatKeyFingerprint(protoMember.PublicKeys.Sign),
			IsCurrentUser: protoMember.AccountID == s.unlockedSession.AccountID,
			IsKeyChanged:  s.state.IsMemberKeyChanged(protoMember.AccountID, protoMember.PublicKeys.Sign),
		})
	}
	slices.SortFunc(allPerson, func(first, second Person) int {
		return strings.Compare(strings.ToLower(first.Name+first.Email), strings.ToLower(second.Name+second.Email))
	})
	return allPerson, nil
}

// ListAllVaultMember returns who has access to a vault, owners first.
func (s *Service) ListAllVaultMember(vaultID string) ([]VaultMember, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.listAllVaultMember(ctx, vaultID)
}

func (s *Service) listAllVaultMember(ctx context.Context, vaultID string) ([]VaultMember, error) {
	allAccess, err := s.unlockedSession.Client.ListAllVaultMember(ctx, vaultID)
	if err != nil {
		return nil, convertError(err)
	}
	allProtoMember, err := s.unlockedSession.Client.ListAllMember(ctx)
	if err != nil {
		return nil, convertError(err)
	}
	allProtoMemberByID := map[string]proto.Member{}
	for _, protoMember := range allProtoMember {
		allProtoMemberByID[protoMember.AccountID] = protoMember
	}
	allVaultMember := make([]VaultMember, 0, len(allAccess))
	for _, access := range allAccess {
		protoMember := allProtoMemberByID[access.AccountID]
		vaultMember := VaultMember{
			AccountID:     access.AccountID,
			Name:          protoMember.Name,
			Email:         protoMember.Email,
			Role:          access.Role,
			IsCurrentUser: access.AccountID == s.unlockedSession.AccountID,
		}
		if len(protoMember.PublicKeys.Sign) > 0 {
			vaultMember.SecurityCode = crypt.FormatKeyFingerprint(protoMember.PublicKeys.Sign)
			vaultMember.IsKeyChanged = s.state.IsMemberKeyChanged(access.AccountID, protoMember.PublicKeys.Sign)
		}
		allVaultMember = append(allVaultMember, vaultMember)
	}
	allRoleOrder := []string{proto.VaultOwner, proto.VaultEditor, proto.VaultReader}
	slices.SortStableFunc(allVaultMember, func(first, second VaultMember) int {
		if order := slices.Index(allRoleOrder, first.Role) - slices.Index(allRoleOrder, second.Role); order != 0 {
			return order
		}
		return strings.Compare(strings.ToLower(first.Name), strings.ToLower(second.Name))
	})
	return allVaultMember, nil
}

// findSharableVault returns a shared vault the current user owns.
func (s *Service) findSharableVault(ctx context.Context, vaultID string) (*session.Vault, error) {
	vault, err := s.findVault(ctx, vaultID)
	if err != nil {
		return nil, err
	}
	if vault.Type == proto.VaultPersonal {
		return nil, errPersonalVault
	}
	if vault.Role != proto.VaultOwner {
		return nil, errOwnerOnly
	}
	return vault, nil
}

// shareWithMember checks the member's pinned signing key, then wraps the vault key for them with the role.
func (s *Service) shareWithMember(ctx context.Context, vault *session.Vault, member *proto.Member, role string) error {
	pinErr := s.state.CheckMemberPin(member)
	if err := s.state.Save(); pinErr == nil && err != nil {
		return err
	}
	if pinErr != nil {
		return pinErr
	}
	return convertSharingError(s.unlockedSession.ShareVault(ctx, vault, member, role))
}

func (s *Service) getVerifiedMember(ctx context.Context, emailOrID string) (*proto.Member, error) {
	member, err := s.unlockedSession.GetVerifiedMember(ctx, emailOrID)
	if errors.Is(err, session.ErrNotFound) {
		return nil, errMemberNotFound
	}
	if err != nil {
		return nil, convertError(err)
	}
	return member, nil
}

// ShareVault emails a share invitation; the person gets access once they accept and this app completes the share.
func (s *Service) ShareVault(vaultID, email, role string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	email, err := normalizeEmail(email)
	if err != nil {
		return err
	}
	if !isValidVaultRole(role) {
		return errors.New("choose owner, editor or reader")
	}
	if email == strings.ToLower(s.unlockedSession.Email) {
		return errShareWithOwnAccount
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vault, err := s.findSharableVault(ctx, vaultID)
	if err != nil {
		return err
	}
	return convertInvitationError(s.unlockedSession.InviteToVault(ctx, vault, email, role))
}

// ChangeVaultMemberRole shares the vault again with the member under the new role.
func (s *Service) ChangeVaultMemberRole(vaultID, accountID, role string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	if !isValidVaultRole(role) {
		return errors.New("choose owner, editor or reader")
	}
	if accountID == s.unlockedSession.AccountID {
		return errOwnAccess
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vault, err := s.findSharableVault(ctx, vaultID)
	if err != nil {
		return err
	}
	allVaultMember, err := s.unlockedSession.Client.ListAllVaultMember(ctx, vaultID)
	if err != nil {
		return convertSharingError(err)
	}
	if !slices.ContainsFunc(allVaultMember, func(vaultMember proto.VaultMember) bool { return vaultMember.AccountID == accountID }) {
		return errors.New("this person no longer has access to the vault; share it with them again")
	}
	member, err := s.getVerifiedMember(ctx, accountID)
	if err != nil {
		return err
	}
	return s.shareWithMember(ctx, vault, member, role)
}

// RemoveVaultMember removes a member and re-encrypts the vault with a new key that only the remaining members get.
func (s *Service) RemoveVaultMember(vaultID, accountID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	if accountID == s.unlockedSession.AccountID {
		return errors.New("you cannot remove yourself from a vault you own")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	vault, err := s.findSharableVault(ctx, vaultID)
	if err != nil {
		return err
	}
	removeErr := s.unlockedSession.RemoveVaultMember(ctx, vault, accountID)
	if err := s.state.Save(); removeErr == nil && err != nil {
		return err
	}
	if errors.Is(removeErr, session.ErrNotFound) {
		return errors.New("this person no longer has access to the vault")
	}
	if client.IsStatus(removeErr, http.StatusConflict) {
		return errors.New("the vault kept changing on other devices; try again")
	}
	if removeErr != nil {
		return convertSharingError(removeErr)
	}
	if err := s.loadAllVault(ctx); err != nil {
		return err
	}
	refreshed, found := s.allVaultByID[vaultID]
	if !found {
		return nil
	}
	allItem, err := s.unlockedSession.ListAllItem(ctx, refreshed)
	if err != nil {
		return convertError(err)
	}
	s.allItemByVaultID[vaultID] = allItem
	return nil
}

// InvitePerson asks the server to email a sign-up invite; the answer is the same whether or not the person has an account.
func (s *Service) InvitePerson(email string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	email, err := normalizeEmail(email)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err = s.unlockedSession.Client.Invite(ctx, &proto.InviteRequest{Email: email})
	return convertInvitationError(err)
}

// ListAllDevice returns the devices signed in to this account, this device first, then the most recently seen.
func (s *Service) ListAllDevice() ([]Device, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	allProtoDevice, err := s.unlockedSession.Client.ListAllDevice(ctx)
	if err != nil {
		return nil, convertError(err)
	}
	slices.SortStableFunc(allProtoDevice, func(first, second proto.Device) int {
		if first.Current != second.Current {
			if first.Current {
				return -1
			}
			return 1
		}
		return second.LastSeenAt.Compare(first.LastSeenAt)
	})
	allDevice := make([]Device, 0, len(allProtoDevice))
	for _, protoDevice := range allProtoDevice {
		allDevice = append(allDevice, Device{
			ID:         protoDevice.ID,
			Name:       protoDevice.Name,
			Platform:   protoDevice.Platform,
			CreatedAt:  protoDevice.CreatedAt.UTC().Format(time.RFC3339),
			LastSeenAt: protoDevice.LastSeenAt.UTC().Format(time.RFC3339),
			IsCurrent:  protoDevice.Current,
		})
	}
	return allDevice, nil
}

// RevokeDevice signs another device out; this device signs out with SignOut instead.
func (s *Service) RevokeDevice(deviceID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	allProtoDevice, err := s.unlockedSession.Client.ListAllDevice(ctx)
	if err != nil {
		return convertError(err)
	}
	index := slices.IndexFunc(allProtoDevice, func(protoDevice proto.Device) bool { return protoDevice.ID == deviceID })
	if index < 0 {
		return errors.New("this device is already signed out")
	}
	if allProtoDevice[index].Current {
		return errCurrentDevice
	}
	if err := s.unlockedSession.Client.RevokeDevice(ctx, deviceID); err != nil && !client.IsStatus(err, http.StatusNotFound) {
		return convertError(err)
	}
	return nil
}
