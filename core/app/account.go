package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/session"
)

const signedOutByServerReason = "This device was signed out. If your master password or Secret Key was changed on another device, sign in with the new one."

type CredentialChangeResult struct {
	Status Status `json:"status"`
	// SecretKey is set only after replacing the Secret Key; the new Emergency Kit is saved with SaveNewAccountEmergencyKit.
	SecretKey            string `json:"secretKey"`
	SignedOutDeviceCount int    `json:"signedOutDeviceCount"`
	// KeychainError is set when the new Secret Key could not be saved to the keychain; the user must write it down.
	KeychainError string `json:"keychainError"`
	// LocalError reports local finalization failures after the credentials have already changed.
	LocalError string `json:"localError"`
	identity   credentialChangeIdentity
}

type credentialChangeIdentity struct {
	epoch     uint64
	accountID string
}

type DeletionVault struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Type             string `json:"type"`
	Role             string `json:"role"`
	OtherMemberCount int    `json:"otherMemberCount"`
}

type AccountDeletionPreview struct {
	PlanVersion             string          `json:"planVersion"`
	Email                   string          `json:"email"`
	OwnedVaults             []DeletionVault `json:"ownedVaults"`
	MemberVaults            []DeletionVault `json:"memberVaults"`
	ShareLinkCount          int             `json:"shareLinkCount"`
	SentInvitationCount     int             `json:"sentInvitationCount"`
	ReceivedInvitationCount int             `json:"receivedInvitationCount"`
	DeviceCount             int             `json:"deviceCount"`
	// Organizations: IsDeleted when the user is its last member; NewAdminName is who becomes admin when the user is its
	// last admin. Omitted when the user is in none.
	Organizations []DeletionOrganization `json:"organizations,omitempty"`
}

type DeletionOrganization struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	IsDeleted    bool   `json:"isDeleted"`
	NewAdminName string `json:"newAdminName"`
	NewAdminID   string `json:"newAdminId"`
}

func convertAccountError(err error) error {
	switch {
	case errors.Is(err, session.ErrBadCredentials):
		return errors.New("the master password is not correct")
	case errors.Is(err, session.ErrEmailMismatch):
		return errors.New("type the email of this account to confirm")
	case errors.Is(err, session.ErrSamePassword), errors.Is(err, session.ErrPasswordTooWeak):
		return err
	case apiclient.IsStatus(err, http.StatusConflict):
		return errors.New("your master password or Secret Key was just changed on another device; sign in again")
	case apiclient.IsUnreachable(err):
		return errors.New("cannot reach the Hisar server; this needs a connection")
	}
	return convertError(err)
}

// convertAccountDeletionError asks for a new review only when the server says the plan changed; any other deletion
// conflict (busy memberships, changed credentials) keeps the server's reason.
func convertAccountDeletionError(err error) error {
	var apiError *apiclient.APIError
	switch {
	case apiclient.IsDeletionPlanChanged(err):
		return errors.New("the deletion plan changed; review the deletion again before confirming")
	case errors.As(err, &apiError) && apiError.Status == http.StatusConflict:
		return errors.New(apiError.Message)
	}
	return convertAccountError(err)
}

// ChangeMasterPassword keeps the Secret Key; every other device is signed out and must sign in with the new password.
func (s *Service) ChangeMasterPassword(currentPassword, newPassword string) (CredentialChangeResult, error) {
	newPasswordBytes := []byte(newPassword)
	defer clear(newPasswordBytes)
	return s.changeCredentials(currentPassword, func(ctx context.Context, unlockedSession *session.Session, currentPasswordBytes []byte, secretKey crypt.SecretKey) (*session.CredentialChange, error) {
		return unlockedSession.ChangeMasterPassword(ctx, currentPasswordBytes, newPasswordBytes, secretKey)
	})
}

// ReplaceSecretKey makes a new Secret Key; the old one and its Emergency Kit stop working, and other devices are signed out.
func (s *Service) ReplaceSecretKey(password string) (CredentialChangeResult, error) {
	return s.changeCredentials(password, func(ctx context.Context, unlockedSession *session.Session, passwordBytes []byte, secretKey crypt.SecretKey) (*session.CredentialChange, error) {
		return unlockedSession.ReplaceSecretKey(ctx, passwordBytes, secretKey)
	})
}

func (s *Service) changeCredentials(password string, change func(context.Context, *session.Session, []byte, crypt.SecretKey) (*session.CredentialChange, error)) (CredentialChangeResult, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return CredentialChangeResult{}, err
	}
	secretKey, err := s.config.SecretKeyStore.LoadSecretKey(s.state.AccountID)
	if err != nil {
		return CredentialChangeResult{}, fmt.Errorf("the Secret Key is missing from the keychain: %w", err)
	}
	passwordBytes := []byte(password)
	defer clear(passwordBytes)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	credentialChange, err := change(ctx, s.unlockedSession, passwordBytes, secretKey)
	if err != nil {
		return CredentialChangeResult{}, convertAccountError(err)
	}
	defer clear(credentialChange.AccountUnlockKey)
	var cacheErr error
	if credentialChange.IsCachedAccountStale {
		s.unlockedSession.Cache = nil
		cacheErr = s.removeCache()
		if cacheErr == nil {
			cacheErr = s.attachCache(ctx, s.unlockedSession)
		}
	}

	result := CredentialChangeResult{
		SignedOutDeviceCount: credentialChange.SignedOutDeviceCount,
		identity:             credentialChangeIdentity{epoch: s.sessionEpoch, accountID: s.state.AccountID},
	}
	if credentialChange.SecretKey.Encode() != secretKey.Encode() {
		newSecretKey := credentialChange.SecretKey
		s.newAccountSecretKey = &newSecretKey
		result.SecretKey = newSecretKey.Encode()
		if err := s.config.SecretKeyStore.StoreSecretKey(newSecretKey); err != nil {
			result.KeychainError = fmt.Sprintf("the new Secret Key could not be saved to the keychain (%v); save the Emergency Kit now", err)
		}
	}
	if len(s.state.BiometricUnlock) > 0 {
		// Only a platform with a BiometricKeyPreparer needs a prompt to store the key; a keychain stores it silently.
		if s.config.Biometric == nil || !s.config.Biometric.IsAvailable() {
			s.forgetBiometricUnlock()
		} else if s.config.BiometricKeyPreparer != nil && s.authenticateToStoreBiometricKey("keep biometric unlock on for Hisar") != nil {
			s.forgetBiometricUnlock()
		} else if _, err := s.sealBiometricUnlock(credentialChange.AccountUnlockKey); err != nil {
			s.forgetBiometricUnlock()
		}
	}
	s.recordPasswordUnlock()
	result.Status = s.buildStatus()
	if err := errors.Join(cacheErr, s.state.Save()); err != nil {
		result.LocalError = fmt.Sprintf("your credentials were changed, but this device could not finish saving the change: %v", err)
	}
	return result, nil
}

func (s *Service) PreviewAccountDeletion() (AccountDeletionPreview, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return AccountDeletionPreview{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	preview, err := s.unlockedSession.PreviewAccountDeletion(ctx)
	if err != nil {
		return AccountDeletionPreview{}, convertAccountError(err)
	}
	result := AccountDeletionPreview{
		PlanVersion:             preview.PlanVersion,
		Email:                   s.state.Email,
		OwnedVaults:             convertDeletionVaults(preview.OwnedVaults),
		MemberVaults:            convertDeletionVaults(preview.MemberVaults),
		ShareLinkCount:          preview.ShareLinkCount,
		SentInvitationCount:     preview.SentInvitationCount,
		ReceivedInvitationCount: preview.ReceivedInvitationCount,
		DeviceCount:             preview.DeviceCount,
		Organizations:           make([]DeletionOrganization, 0, len(preview.Organizations)),
	}
	for _, organization := range preview.Organizations {
		result.Organizations = append(result.Organizations, DeletionOrganization{
			ID: organization.OrganizationID, Name: organization.Name, IsDeleted: organization.IsDeleted, NewAdminName: organization.NewAdminName, NewAdminID: organization.NewAdminID,
		})
	}
	return result, nil
}

func convertDeletionVaults(vaults []session.DeletionVault) []DeletionVault {
	converted := make([]DeletionVault, 0, len(vaults))
	for _, vault := range vaults {
		name := vault.Name
		if name == "" {
			name = fmt.Sprintf("Vault %s (name unavailable)", vault.ID)
		}
		converted = append(converted, DeletionVault{ID: vault.ID, Name: name, Type: vault.Type, Role: vault.Role, OtherMemberCount: vault.OtherMemberCount})
	}
	return converted
}

// DeleteAccount needs the master password and the account's email; afterwards this device is signed out and wiped.
func (s *Service) DeleteAccount(password, email string) (Status, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return Status{}, err
	}
	return Status{}, errors.New("update Hisar to pass the reviewed account deletion plan")
}

func (s *Service) DeleteAccountWithPlan(password, email, planVersion string) (Status, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return Status{}, err
	}
	if !strings.EqualFold(strings.TrimSpace(email), s.state.Email) {
		return Status{}, convertAccountError(session.ErrEmailMismatch)
	}
	secretKey, err := s.config.SecretKeyStore.LoadSecretKey(s.state.AccountID)
	if err != nil {
		return Status{}, fmt.Errorf("the Secret Key is missing from the keychain: %w", err)
	}
	passwordBytes := []byte(password)
	defer clear(passwordBytes)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := s.unlockedSession.DeleteAccount(ctx, passwordBytes, secretKey, email, planVersion); err != nil {
		return Status{}, convertAccountDeletionError(err)
	}
	forgetErr := s.forgetLocalAccount()
	return s.buildStatus(), forgetErr
}

// forgetLocalAccount removes everything this device keeps for the account, without telling the server.
func (s *Service) forgetLocalAccount() error {
	var secretKeyErr error
	if s.state.AccountID != "" {
		secretKeyErr = s.config.SecretKeyStore.DeleteSecretKey(s.state.AccountID)
	}
	cacheErr := s.resetAccount()
	s.notifySessionChange()
	if err := s.state.Save(); err != nil {
		return err
	}
	return errors.Join(secretKeyErr, cacheErr)
}

// lockSuspendedSession locks Hisar when the server refuses the session because an admin suspended the account. Unlike
// endSignedOutSession it keeps the Secret Key, the state and the cache: the session works again after reactivation, and
// until then unlocking answers the suspension sentence.
func (s *Service) lockSuspendedSession(token string) {
	s.mutex.Lock()
	if token == "" || s.state.Token != token {
		s.mutex.Unlock()
		return
	}
	wasUnlocked := s.unlockedSession != nil
	s.lockSession()
	s.notifyStatusListener()
	s.mutex.Unlock()
	if wasUnlocked && s.config.LockNotifier != nil {
		s.config.LockNotifier.NotifyLocked()
	}
}

// endSignedOutSession runs when the server no longer accepts this device's session (revoked, or credentials changed elsewhere).
func (s *Service) endSignedOutSession(token string) bool {
	s.mutex.Lock()
	if token == "" || s.state.Token != token {
		s.mutex.Unlock()
		return false
	}
	wasUnlocked := s.unlockedSession != nil
	s.forgetLocalAccount()
	s.signedOutReason = signedOutByServerReason
	s.notifyStatusListener()
	s.mutex.Unlock()
	if wasUnlocked && s.config.LockNotifier != nil {
		s.config.LockNotifier.NotifyLocked()
	}
	return true
}

// DismissSignedOutReason clears the notice on the sign-in screen.
func (s *Service) DismissSignedOutReason() Status {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.signedOutReason = ""
	return s.buildStatus()
}
