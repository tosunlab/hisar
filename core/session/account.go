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
	return verifyCredentials(ctx, s.Client, password, secretKey, s.Keyset)
}

// verifyCredentials opens the keyset the server holds for the signed-in account; a non-nil sessionKeyset must be that same keyset.
func verifyCredentials(ctx context.Context, client *apiclient.Client, password []byte, secretKey crypt.SecretKey, sessionKeyset *crypt.Keyset) (*verifiedAccount, error) {
	account, err := client.GetAccount(ctx)
	if err != nil {
		return nil, err
	}
	if account.AccountID != secretKey.AccountID {
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
	isSessionKeyset := sessionKeyset == nil || bytes.Equal(sessionKeyset.GetPublicKeys(account.AccountID).Sign, publicKeys.Sign)
	if !bytes.Equal(publicKeys.KEM, account.PublicKeys.KEM) || !bytes.Equal(publicKeys.Sign, account.PublicKeys.Sign) || !isSessionKeyset {
		currentKeys.Wipe()
		keyset.Wipe()
		return nil, ErrKeyMismatch
	}
	return &verifiedAccount{account: account, currentKeys: currentKeys, keyset: keyset}, nil
}

// Reauthenticate proves to the server, with OPAQUE, that the signed-in client knows the account's email, master password
// and Secret Key; neither secret leaves the device. Changes such as deleting the account carry the proof.
func Reauthenticate(ctx context.Context, client *apiclient.Client, email string, password []byte, secretKey crypt.SecretKey) (apitypes.ReauthenticationProof, error) {
	return reauthenticate(ctx, client, email, password, secretKey, nil)
}

// reauthenticate is Reauthenticate for a caller that holds the account's keyset, which the server's keyset must match.
func reauthenticate(ctx context.Context, client *apiclient.Client, email string, password []byte, secretKey crypt.SecretKey, sessionKeyset *crypt.Keyset) (apitypes.ReauthenticationProof, error) {
	verified, err := verifyCredentials(ctx, client, password, secretKey, sessionKeyset)
	if err != nil {
		return apitypes.ReauthenticationProof{}, err
	}
	defer verified.wipe()
	if !strings.EqualFold(verified.account.Email, strings.TrimSpace(email)) {
		return apitypes.ReauthenticationProof{}, ErrEmailMismatch
	}
	proof, _, err := proveCredentials(ctx, client, verified.currentKeys.AuthKey, verified.account.AccountID, nil)
	return proof, err
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

	var proof apitypes.ReauthenticationProof
	err = credential.registerOpaque(newKeys, s.AccountID, func(registrationRequest []byte) ([]byte, error) {
		var registrationResponse []byte
		var proveErr error
		proof, registrationResponse, proveErr = proveCredentials(ctx, s.Client, verified.currentKeys.AuthKey, s.AccountID, registrationRequest)
		return registrationResponse, proveErr
	})
	if err != nil {
		return nil, err
	}
	response, err := s.Client.ChangeCredentials(ctx, &apitypes.ChangeCredentialsRequest{
		ReauthenticationID: proof.ReauthenticationID,
		KE3:                proof.KE3,
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
	if secretKey.AccountID != s.AccountID {
		return ErrBadCredentials
	}
	proof, err := reauthenticate(ctx, s.Client, email, password, secretKey, s.Keyset)
	if err != nil {
		return err
	}
	err = s.Client.DeleteAccount(ctx, &apitypes.DeleteAccountRequest{ReauthenticationID: proof.ReauthenticationID, KE3: proof.KE3, Email: strings.TrimSpace(email)})
	return convertReauthenticationError(err)
}
