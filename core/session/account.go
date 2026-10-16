package session

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
)

var (
	ErrEmailMismatch   = errors.New("the email does not match this account")
	ErrSamePassword    = errors.New("the new master password is the same as the current one")
	ErrPasswordTooWeak = errors.New("use a master password with at least 10 characters")
)

const MinimumPasswordLength = 10

type CredentialChange struct {
	SecretKey crypt.SecretKey
	// AccountUnlockKey is the new key; the caller wipes it.
	AccountUnlockKey     []byte
	SignedOutDeviceCount int
}

type DeletionVault struct {
	ID               string
	Name             string
	Type             string
	Role             string
	OtherMemberCount int
}

type AccountDeletionPreview struct {
	OwnedVaults             []DeletionVault
	MemberVaults            []DeletionVault
	ShareLinkCount          int
	SentInvitationCount     int
	ReceivedInvitationCount int
	DeviceCount             int
	// Organizations is empty for servers older than organizations.
	Organizations []apitypes.DeletionOrganization
}

// ChangeMasterPassword keeps the Secret Key and the keyset; only the keyset's encryption and the OPAQUE record change.
func (s *Session) ChangeMasterPassword(ctx context.Context, currentPassword, newPassword []byte, secretKey crypt.SecretKey) (*CredentialChange, error) {
	if len([]rune(string(newPassword))) < MinimumPasswordLength {
		return nil, ErrPasswordTooWeak
	}
	if bytes.Equal(currentPassword, newPassword) {
		return nil, ErrSamePassword
	}
	return s.changeCredentials(ctx, apitypes.CredentialChangeMasterPassword, currentPassword, secretKey, newPassword, secretKey)
}

// ReplaceSecretKey keeps the master password and the account ID; the old Secret Key and Emergency Kit stop working.
func (s *Session) ReplaceSecretKey(ctx context.Context, password []byte, currentSecretKey crypt.SecretKey) (*CredentialChange, error) {
	newSecretKey, err := crypt.GenerateSecretKey(s.AccountID)
	if err != nil {
		return nil, err
	}
	return s.changeCredentials(ctx, apitypes.CredentialChangeSecretKey, password, currentSecretKey, password, newSecretKey)
}

type verifiedAccount struct {
	account     *apitypes.Account
	currentKeys *crypt.DerivedKeys
	keyset      *crypt.Keyset
}

func (verified *verifiedAccount) wipe() {
	verified.currentKeys.Wipe()
	verified.keyset.Wipe()
}

// verifyCurrentCredentials needs the server: credentials never change, and an account is never deleted, from cached data.
func (s *Session) verifyCurrentCredentials(ctx context.Context, password []byte, secretKey crypt.SecretKey) (*verifiedAccount, error) {
	if secretKey.AccountID != s.AccountID {
		return nil, ErrBadCredentials
	}
	account, err := s.Client.GetAccount(ctx)
	if err != nil {
		return nil, err
	}
	if account.AccountID != s.AccountID {
		return nil, ErrBadCredentials
	}
	currentKeys, err := crypt.DeriveKeys(password, account.Salt, account.Argon2, secretKey)
	if err != nil {
		return nil, err
	}
	keyset, err := crypt.DecryptKeyset(currentKeys.AccountUnlockKey, account.AccountID, account.EncKeyset)
	if err != nil {
		currentKeys.Wipe()
		return nil, ErrBadCredentials
	}
	publicKeys := keyset.GetPublicKeys(account.AccountID)
	isSessionKeyset := s.Keyset == nil || bytes.Equal(s.Keyset.GetPublicKeys(s.AccountID).Sign, publicKeys.Sign)
	if !bytes.Equal(publicKeys.KEM, account.PublicKeys.KEM) || !bytes.Equal(publicKeys.Sign, account.PublicKeys.Sign) || !isSessionKeyset {
		currentKeys.Wipe()
		keyset.Wipe()
		return nil, ErrKeyMismatch
	}
	return &verifiedAccount{account: account, currentKeys: currentKeys, keyset: keyset}, nil
}

// strongerArgon2Params never lowers a cost the account already uses.
func strongerArgon2Params(current crypt.Argon2Params) crypt.Argon2Params {
	params := crypt.DefaultArgon2Params()
	if current.Validate() != nil {
		return params
	}
	params.Time = max(params.Time, current.Time)
	params.MemoryKiB = max(params.MemoryKiB, current.MemoryKiB)
	params.Threads = max(params.Threads, current.Threads)
	return params
}

func (s *Session) changeCredentials(ctx context.Context, change string, currentPassword []byte, currentSecretKey crypt.SecretKey, newPassword []byte, newSecretKey crypt.SecretKey) (*CredentialChange, error) {
	verified, err := s.verifyCurrentCredentials(ctx, currentPassword, currentSecretKey)
	if err != nil {
		return nil, err
	}
	defer verified.wipe()

	credential := &accountCredential{secretKey: newSecretKey, salt: crypt.GenerateSalt(), argon2Params: strongerArgon2Params(verified.account.Argon2), keyset: verified.keyset}
	newKeys, err := crypt.DeriveKeys(newPassword, credential.salt, credential.argon2Params, newSecretKey)
	if err != nil {
		return nil, err
	}
	defer newKeys.Wipe()

	login, keyExchange1, err := startOpaqueLogin(verified.currentKeys.AuthKey)
	if err != nil {
		return nil, err
	}
	defer login.clear()
	var reauthentication *apitypes.ReauthenticateResponse
	err = credential.registerOpaque(newKeys, s.AccountID, func(registrationRequest []byte) ([]byte, error) {
		var requestErr error
		reauthentication, requestErr = s.Client.Reauthenticate(ctx, &apitypes.ReauthenticateRequest{KE1: keyExchange1, RegistrationRequest: registrationRequest})
		if requestErr != nil {
			return nil, requestErr
		}
		return reauthentication.RegistrationResponse, nil
	})
	if err != nil {
		return nil, err
	}
	keyExchange3, err := login.finish(reauthentication.KE2, s.AccountID)
	if err != nil {
		return nil, err
	}
	response, err := s.Client.ChangeCredentials(ctx, &apitypes.ChangeCredentialsRequest{
		ReauthenticationID: reauthentication.ReauthenticationID,
		KE3:                keyExchange3,
		Change:             change,
		Record:             credential.record,
		Salt:               credential.salt,
		Argon2:             credential.argon2Params,
		EncKeyset:          credential.encryptedKeyset,
	})
	if err != nil {
		return nil, convertReauthenticationError(err)
	}
	if s.Cache != nil {
		updatedAccount := *verified.account
		updatedAccount.Salt, updatedAccount.Argon2, updatedAccount.EncKeyset = credential.salt, credential.argon2Params, credential.encryptedKeyset
		s.Cache.SaveAccount(&updatedAccount)
	}
	return &CredentialChange{
		SecretKey:            newSecretKey,
		AccountUnlockKey:     append([]byte(nil), newKeys.AccountUnlockKey...),
		SignedOutDeviceCount: response.SignedOutDeviceCount,
	}, nil
}

// A failed proof is 403, never 401: clients treat 401 as a signed-out device.
func convertReauthenticationError(err error) error {
	if apiclient.IsStatus(err, http.StatusForbidden) {
		return ErrBadCredentials
	}
	return err
}

func (s *Session) PreviewAccountDeletion(ctx context.Context) (*AccountDeletionPreview, error) {
	response, err := s.Client.GetAccountDeletionPreview(ctx)
	if err != nil {
		return nil, err
	}
	vaultNames := map[string]string{}
	if vaults, err := s.ListVaults(ctx); err == nil {
		for index := range vaults {
			vaultNames[vaults[index].ID] = vaults[index].Name
			clear(vaults[index].Key)
		}
	}
	preview := &AccountDeletionPreview{
		OwnedVaults:             []DeletionVault{},
		MemberVaults:            []DeletionVault{},
		ShareLinkCount:          response.ShareLinkCount,
		SentInvitationCount:     response.SentInvitationCount,
		ReceivedInvitationCount: response.ReceivedInvitationCount,
		DeviceCount:             response.DeviceCount,
		Organizations:           append([]apitypes.DeletionOrganization{}, response.Organizations...),
	}
	for _, ownedVault := range response.OwnedVaults {
		preview.OwnedVaults = append(preview.OwnedVaults, DeletionVault{
			ID: ownedVault.VaultID, Name: vaultNames[ownedVault.VaultID], Type: ownedVault.Type, Role: apitypes.VaultOwner, OtherMemberCount: ownedVault.OtherMemberCount,
		})
	}
	for _, memberVault := range response.MemberVaults {
		preview.MemberVaults = append(preview.MemberVaults, DeletionVault{
			ID: memberVault.VaultID, Name: vaultNames[memberVault.VaultID], Type: memberVault.Type, Role: memberVault.Role,
		})
	}
	return preview, nil
}

// DeleteAccount proves the master password to the server with OPAQUE; the password itself never leaves the device.
func (s *Session) DeleteAccount(ctx context.Context, password []byte, secretKey crypt.SecretKey, email string) error {
	if !strings.EqualFold(strings.TrimSpace(email), s.Email) {
		return ErrEmailMismatch
	}
	verified, err := s.verifyCurrentCredentials(ctx, password, secretKey)
	if err != nil {
		return err
	}
	defer verified.wipe()
	if !strings.EqualFold(verified.account.Email, strings.TrimSpace(email)) {
		return ErrEmailMismatch
	}
	login, keyExchange1, err := startOpaqueLogin(verified.currentKeys.AuthKey)
	if err != nil {
		return err
	}
	defer login.clear()
	reauthentication, err := s.Client.Reauthenticate(ctx, &apitypes.ReauthenticateRequest{KE1: keyExchange1})
	if err != nil {
		return err
	}
	keyExchange3, err := login.finish(reauthentication.KE2, s.AccountID)
	if err != nil {
		return err
	}
	err = s.Client.DeleteAccount(ctx, &apitypes.DeleteAccountRequest{ReauthenticationID: reauthentication.ReauthenticationID, KE3: keyExchange3, Email: strings.TrimSpace(email)})
	return convertReauthenticationError(err)
}
