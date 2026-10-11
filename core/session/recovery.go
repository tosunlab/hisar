package session

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/tryhisar/hisar/core/client"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/proto"
)

var (
	ErrNoRecoveryKeyset = errors.New("recovery is not set up")
	ErrBadRecoveryKey   = errors.New("the recovery public key is not signed by the admin who created it")
	ErrRecoverSelf      = errors.New("you cannot recover your own account; ask another admin")
)

// CreateRecoveryKeyset generates the recovery keyset, encrypted with the admin's user key, and uploads it.
// Afterwards call EnableRecoveryForAllVault to wrap existing vaults.
func (s *Session) CreateRecoveryKeyset(ctx context.Context) error {
	recoveryKeyset, err := crypt.GenerateKeyset()
	if err != nil {
		return err
	}
	defer recoveryKeyset.Wipe()
	encryptedKeyset, err := recoveryKeyset.EncryptRecoveryKeyset(s.Keyset.UserKey, s.AccountID)
	if err != nil {
		return err
	}
	return s.Client.CreateRecoveryKeyset(ctx, &proto.CreateRecoveryKeysetRequest{
		PublicKeys: s.Keyset.SignRecoveryKeyset(recoveryKeyset),
		EncKeyset:  encryptedKeyset,
	})
}

// GetRecoveryPublicKey returns the recovery public keys after checking that the admin who created them signed them.
func (s *Session) GetRecoveryPublicKey(ctx context.Context) (*crypt.PublicKeys, error) {
	recoveryKeyset, err := s.getVerifiedRecoveryKeyset(ctx)
	if err != nil {
		return nil, err
	}
	return &recoveryKeyset.PublicKeys, nil
}

func (s *Session) getVerifiedRecoveryKeyset(ctx context.Context) (*proto.RecoveryKeyset, error) {
	recoveryKeyset, err := s.Client.GetRecoveryKeyset(ctx)
	if client.IsStatus(err, http.StatusNotFound) {
		return nil, ErrNoRecoveryKeyset
	}
	if err != nil {
		return nil, err
	}
	creator := recoveryKeyset.Creator
	if creator == nil || creator.AccountID != recoveryKeyset.CreatedBy || creator.Role != proto.RoleAdmin || creator.PublicKeys.Verify(creator.AccountID) != nil ||
		!bytes.Equal(creator.PublicKeys.Sign, recoveryKeyset.PublicKeys.Sign) ||
		recoveryKeyset.PublicKeys.Verify(crypt.RecoveryRecipientID) != nil {
		return nil, ErrBadRecoveryKey
	}
	if s.MemberVerifier != nil {
		if err := s.MemberVerifier(creator); err != nil {
			return nil, err
		}
	}
	if s.RecoveryKeyVerifier != nil {
		if err := s.RecoveryKeyVerifier(recoveryKeyset.PublicKeys.KEM); err != nil {
			return nil, err
		}
	}
	return recoveryKeyset, nil
}

// ListAllVaultWithoutRecovery returns the vaults the user owns whose key is not wrapped to the recovery keyset.
func (s *Session) ListAllVaultWithoutRecovery(ctx context.Context) ([]Vault, error) {
	allVault, err := s.ListAllVault(ctx)
	if err != nil {
		return nil, err
	}
	var allVaultWithoutRecovery []Vault
	for _, vault := range allVault {
		if vault.Role == proto.VaultOwner && !vault.Recoverable {
			allVaultWithoutRecovery = append(allVaultWithoutRecovery, vault)
		}
	}
	return allVaultWithoutRecovery, nil
}

// EnableRecoveryForAllVault wraps every owned vault that lacks a recovery wrapping, except opted-out ones, and returns them.
func (s *Session) EnableRecoveryForAllVault(ctx context.Context) ([]Vault, error) {
	recoveryPublicKeys, err := s.GetRecoveryPublicKey(ctx)
	if err != nil {
		return nil, err
	}
	allVault, err := s.ListAllVaultWithoutRecovery(ctx)
	if err != nil {
		return nil, err
	}
	var allEnabledVault []Vault
	for _, vault := range allVault {
		if vault.RecoveryOptOut {
			continue
		}
		if err := s.wrapVaultForRecovery(ctx, &vault, recoveryPublicKeys); err != nil {
			return allEnabledVault, fmt.Errorf("vault %s: %w", vault.Name, err)
		}
		allEnabledVault = append(allEnabledVault, vault)
	}
	return allEnabledVault, nil
}

// EnableVaultRecovery wraps one owned vault to the recovery keyset, also undoing an opt-out.
func (s *Session) EnableVaultRecovery(ctx context.Context, vault *Vault) error {
	recoveryPublicKeys, err := s.GetRecoveryPublicKey(ctx)
	if err != nil {
		return err
	}
	return s.wrapVaultForRecovery(ctx, vault, recoveryPublicKeys)
}

func (s *Session) wrapVaultForRecovery(ctx context.Context, vault *Vault, recoveryPublicKeys *crypt.PublicKeys) error {
	wrappedKey, err := crypt.WrapVaultKey(recoveryPublicKeys.KEM, vault.ID, crypt.RecoveryRecipientID, vault.Key)
	if err != nil {
		return err
	}
	if err := s.Client.SetVaultRecovery(ctx, vault.ID, &proto.SetVaultRecoveryRequest{WrappedKey: wrappedKey}); err != nil {
		return err
	}
	vault.Recoverable, vault.RecoveryOptOut = true, false
	return nil
}

// DisableVaultRecovery opts a personal vault out of recovery: if the user loses access, nobody can recover it.
func (s *Session) DisableVaultRecovery(ctx context.Context, vault *Vault) error {
	if err := s.Client.RemoveVaultRecovery(ctx, vault.ID); err != nil {
		return err
	}
	vault.Recoverable, vault.RecoveryOptOut = false, true
	return nil
}

// StartRecovery registers a new Secret Key, master password and keyset for the account the recovery token belongs to.
// They stay pending, and the old credentials keep working, until an admin completes the recovery.
func StartRecovery(ctx context.Context, apiClient *client.Client, token, email string, newPassword []byte, device proto.DeviceInfo) (crypt.SecretKey, error) {
	accountResponse, err := apiClient.FindRecoveryAccount(ctx, &proto.RecoveryAccountRequest{Token: token, Email: email})
	if err != nil {
		return crypt.SecretKey{}, err
	}
	accountID := accountResponse.AccountID
	if !crypt.IsValidAccountID(accountID) {
		return crypt.SecretKey{}, crypt.ErrBadAccountID
	}
	credential, err := createAccountCredential(accountID, newPassword, func(registrationRequest []byte) ([]byte, error) {
		startResponse, err := apiClient.StartRecovery(ctx, &proto.RecoveryStartRequest{Token: token, Email: email, Request: registrationRequest})
		if err != nil {
			return nil, err
		}
		return startResponse.Response, nil
	})
	if err != nil {
		return crypt.SecretKey{}, err
	}
	defer credential.keyset.Wipe()
	err = apiClient.FinishRecovery(ctx, &proto.RecoveryFinishRequest{
		Token:      token,
		Email:      email,
		Record:     credential.record,
		Salt:       credential.salt,
		Argon2:     credential.argon2Params,
		EncKeyset:  credential.encryptedKeyset,
		PublicKeys: credential.keyset.GetAllPublicKey(accountID),
		Device:     device,
	})
	if err != nil {
		return crypt.SecretKey{}, err
	}
	return credential.secretKey, nil
}

func (s *Session) ListAllPendingRecovery(ctx context.Context) ([]proto.PendingRecovery, error) {
	return s.Client.ListAllPendingRecovery(ctx)
}

// CompleteRecovery re-wraps every recoverable vault of accountID to its new public key and returns the vaults it loses.
// Only the admin who created the recovery keyset can complete recoveries.
func (s *Session) CompleteRecovery(ctx context.Context, accountID string) ([]string, error) {
	if accountID == s.AccountID {
		return nil, ErrRecoverSelf
	}
	allPendingRecovery, err := s.Client.ListAllPendingRecovery(ctx)
	if err != nil {
		return nil, err
	}
	var pending *proto.PendingRecovery
	for index := range allPendingRecovery {
		if allPendingRecovery[index].AccountID == accountID {
			pending = &allPendingRecovery[index]
		}
	}
	if pending == nil {
		return nil, fmt.Errorf("pending recovery for %s: %w", accountID, ErrNotFound)
	}
	if err := pending.PublicKeys.Verify(accountID); err != nil {
		return nil, fmt.Errorf("new public keys of %s: %w", pending.Email, err)
	}

	protoRecoveryKeyset, err := s.getVerifiedRecoveryKeyset(ctx)
	if err != nil {
		return nil, err
	}
	if protoRecoveryKeyset.CreatedBy != s.AccountID || len(protoRecoveryKeyset.EncKeyset) == 0 {
		return nil, errors.New("only the admin who set up recovery can complete it")
	}
	recoveryKeyset, err := crypt.DecryptRecoveryKeyset(s.Keyset.UserKey, s.AccountID, protoRecoveryKeyset.EncKeyset)
	if err != nil {
		return nil, fmt.Errorf("recovery keyset: %w", err)
	}
	defer recoveryKeyset.Wipe()
	if !bytes.Equal(recoveryKeyset.GetKEMPublicKey(), protoRecoveryKeyset.PublicKeys.KEM) {
		return nil, ErrKeyMismatch
	}

	request := &proto.CompleteRecoveryRequest{AllVaultKey: []proto.VaultKey{}}
	for _, recoveryVault := range pending.AllVault {
		if recoveryVault.WrappedKey == nil {
			continue
		}
		vaultKey, err := recoveryKeyset.UnwrapVaultKey(recoveryVault.VaultID, crypt.RecoveryRecipientID, recoveryVault.WrappedKey)
		if err != nil {
			continue
		}
		wrappedKey, err := crypt.WrapVaultKey(pending.PublicKeys.KEM, recoveryVault.VaultID, accountID, vaultKey)
		clear(vaultKey)
		if err != nil {
			return nil, err
		}
		request.AllVaultKey = append(request.AllVaultKey, proto.VaultKey{VaultID: recoveryVault.VaultID, WrappedKey: wrappedKey})
	}
	response, err := s.Client.CompleteRecovery(ctx, accountID, request)
	if err != nil {
		return nil, err
	}
	return response.AllUnrecoverableVaultID, nil
}
