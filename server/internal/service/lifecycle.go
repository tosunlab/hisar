package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/bytemare/opaque"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/server/internal/mail"
	"github.com/tryhisar/hisar/server/internal/store"
)

var (
	errReauthenticationFailed = newError(KindForbidden, "incorrect master password")
	errCredentialsChanged     = newError(KindConflict, "the master password or Secret Key was changed on another device; try again")
)

// Reauthenticate starts a master password check for the caller's session, with a registration for new credentials
// when RegistrationRequest is set; ChangeCredentials and DeleteAccount finish it.
func (s *Service) Reauthenticate(ctx context.Context, actor Actor, request *apitypes.ReauthenticateRequest) (apitypes.ReauthenticateResponse, error) {
	if !s.Limiters.Reauthentication.Allow(actor.AccountID) {
		return apitypes.ReauthenticateResponse{}, newError(KindTooManyRequests, "too many attempts; wait a minute and try again")
	}
	keyExchange1, err := s.keys.OpaqueServer.Deserialize.KE1(request.KE1)
	if err != nil {
		return apitypes.ReauthenticateResponse{}, errInvalidLogin
	}
	account, err := s.store.GetAccountByID(ctx, actor.AccountID)
	if err != nil {
		return apitypes.ReauthenticateResponse{}, err
	}
	registrationRecord, err := s.keys.OpaqueServer.Deserialize.RegistrationRecord(account.OpaqueRecord)
	if err != nil {
		return apitypes.ReauthenticateResponse{}, err
	}
	var registrationResponse []byte
	if len(request.RegistrationRequest) > 0 {
		registrationRequest, err := s.keys.OpaqueServer.Deserialize.RegistrationRequest(request.RegistrationRequest)
		if err != nil {
			return apitypes.ReauthenticateResponse{}, newError(KindInvalid, "invalid registration request")
		}
		response, err := s.keys.OpaqueServer.RegistrationResponse(registrationRequest, []byte(account.ID), nil)
		if err != nil {
			return apitypes.ReauthenticateResponse{}, newError(KindInvalid, "invalid registration request")
		}
		registrationResponse = response.Serialize()
	}
	clientRecord := &opaque.ClientRecord{RegistrationRecord: registrationRecord, CredentialIdentifier: []byte(account.ID), ClientIdentity: []byte(account.ID)}
	keyExchange2, serverOutput, err := s.keys.OpaqueServer.GenerateKE2(keyExchange1, clientRecord)
	if err != nil {
		return apitypes.ReauthenticateResponse{}, errInvalidLogin
	}
	reauthenticationID := s.loginCache.put(&pendingLogin{
		accountID:        account.ID,
		clientMAC:        serverOutput.ClientMAC,
		sessionTokenHash: actor.TokenHash,
		recordHash:       store.HashOpaqueRecord(account.OpaqueRecord),
	})
	clear(serverOutput.SessionSecret)
	return apitypes.ReauthenticateResponse{ReauthenticationID: reauthenticationID, KE2: keyExchange2.Serialize(), RegistrationResponse: registrationResponse}, nil
}

// finishReauthentication accepts a proof only from the session that started it.
func (s *Service) finishReauthentication(ctx context.Context, actor Actor, reauthenticationID string, encodedKeyExchange3 []byte) (*pendingLogin, error) {
	pending := s.loginCache.take(reauthenticationID)
	if pending == nil || pending.sessionTokenHash == nil || pending.accountID != actor.AccountID || !bytes.Equal(pending.sessionTokenHash, actor.TokenHash) {
		return nil, newError(KindForbidden, "the master password check expired; try again")
	}
	keyExchange3, err := s.keys.OpaqueServer.Deserialize.KE3(encodedKeyExchange3)
	if err != nil || s.keys.OpaqueServer.LoginFinish(keyExchange3, pending.clientMAC) != nil {
		s.recordAudit(ctx, actor, "reauthentication_failed", nil)
		return nil, errReauthenticationFailed
	}
	return pending, nil
}

// ChangeCredentials replaces the master password or the Secret Key and signs out every other device.
func (s *Service) ChangeCredentials(ctx context.Context, actor Actor, request *apitypes.ChangeCredentialsRequest) (apitypes.ChangeCredentialsResponse, error) {
	switch {
	case request.Change != apitypes.CredentialChangeMasterPassword && request.Change != apitypes.CredentialChangeSecretKey,
		len(request.Salt) != crypt.SaltSize,
		request.Argon2.Validate() != nil,
		len(request.EncKeyset) == 0 || len(request.EncKeyset) > maxKeysetSize:
		return apitypes.ChangeCredentialsResponse{}, newError(KindInvalid, "invalid credentials")
	}
	if _, err := s.keys.OpaqueServer.Deserialize.RegistrationRecord(request.Record); err != nil {
		return apitypes.ChangeCredentialsResponse{}, newError(KindInvalid, "invalid registration record")
	}
	pending, err := s.finishReauthentication(ctx, actor, request.ReauthenticationID, request.KE3)
	if err != nil {
		return apitypes.ChangeCredentialsResponse{}, err
	}
	argon2ParamsJSON, _ := json.Marshal(request.Argon2)
	signedOutDeviceCount, err := s.store.UpdateAccountCredentials(ctx, &store.CredentialUpdate{
		AccountID:        actor.AccountID,
		TokenHash:        actor.TokenHash,
		ProvenRecordHash: pending.recordHash,
		Argon2Params:     argon2ParamsJSON,
		Salt:             request.Salt,
		OpaqueRecord:     request.Record,
		EncKeyset:        request.EncKeyset,
	})
	switch {
	case errors.Is(err, store.ErrConflict):
		return apitypes.ChangeCredentialsResponse{}, errCredentialsChanged
	case errors.Is(err, store.ErrNotFound):
		return apitypes.ChangeCredentialsResponse{}, ErrUnauthorized
	case err != nil:
		return apitypes.ChangeCredentialsResponse{}, err
	}
	s.events.EndStreams(actor.AccountID, actor.TokenHash)
	s.recordAudit(ctx, actor, "credentials_changed", map[string]any{"change": request.Change, "signedOutDevices": signedOutDeviceCount})
	if s.isEmailConfigured() {
		if account, err := s.store.GetAccountByID(ctx, actor.AccountID); err == nil {
			message := composeCredentialsChangedEmail(account.Email, request.Change, signedOutDeviceCount, time.Now())
			s.runInBackground(func(ctx context.Context) { s.sendEmail(ctx, message) })
		}
	}
	return apitypes.ChangeCredentialsResponse{SignedOutDeviceCount: signedOutDeviceCount}, nil
}

func (s *Service) PreviewAccountDeletion(ctx context.Context, actor Actor) (apitypes.AccountDeletionPreview, error) {
	preview, err := s.store.PreviewAccountDeletion(ctx, actor.AccountID)
	if err != nil {
		return apitypes.AccountDeletionPreview{}, err
	}
	response := apitypes.AccountDeletionPreview{
		OwnedVaults:             make([]apitypes.DeletionOwnedVault, 0, len(preview.OwnedVaults)),
		MemberVaults:            make([]apitypes.DeletionMemberVault, 0, len(preview.MemberVaults)),
		ShareLinkCount:          preview.ShareLinkCount,
		SentInvitationCount:     preview.SentInvitationCount,
		ReceivedInvitationCount: preview.ReceivedInvitationCount,
		DeviceCount:             preview.DeviceCount,
	}
	for _, ownedVault := range preview.OwnedVaults {
		response.OwnedVaults = append(response.OwnedVaults, apitypes.DeletionOwnedVault{VaultID: ownedVault.ID, Type: ownedVault.Type, OtherMemberCount: ownedVault.OtherMemberCount})
	}
	for _, memberVault := range preview.MemberVaults {
		response.MemberVaults = append(response.MemberVaults, apitypes.DeletionMemberVault{VaultID: memberVault.ID, Type: memberVault.Type, Role: memberVault.Role})
	}
	return response, nil
}

// DeleteAccount deletes the account after a master password check and the typed email.
func (s *Service) DeleteAccount(ctx context.Context, actor Actor, request *apitypes.DeleteAccountRequest) error {
	account, err := s.store.GetAccountByID(ctx, actor.AccountID)
	if err != nil {
		return err
	}
	if apitypes.NormalizeEmail(request.Email) != account.Email {
		return newError(KindInvalid, "the email does not match this account")
	}
	pending, err := s.finishReauthentication(ctx, actor, request.ReauthenticationID, request.KE3)
	if err != nil {
		return err
	}
	deleted, err := s.store.DeleteAccount(ctx, actor.AccountID, actor.TokenHash, pending.recordHash)
	switch {
	case errors.Is(err, store.ErrConflict):
		return errCredentialsChanged
	case errors.Is(err, store.ErrNotFound):
		return ErrUnauthorized
	case err != nil:
		return err
	}
	s.events.EndStreams(actor.AccountID, nil)
	s.Approvals.removeForAccount(actor.AccountID)
	s.PublishVaultsChanged(deleted.VaultMemberAccountIDs...)
	s.PublishShareInvitationsChanged(deleted.InvitationAccountIDs...)
	s.recordAudit(ctx, Actor{AccountID: actor.AccountID, IP: actor.IP}, "account_deleted", map[string]int{"deletedVaults": deleted.DeletedVaultCount})
	if s.isEmailConfigured() {
		message := composeAccountDeletedEmail(deleted.Email, time.Now())
		s.runInBackground(func(ctx context.Context) { s.sendEmail(ctx, message) })
	}
	return nil
}

func composeCredentialsChangedEmail(recipient, change string, signedOutDeviceCount int, changedAt time.Time) mail.Message {
	signedOut := "No other devices were signed in."
	if signedOutDeviceCount == 1 {
		signedOut = "Your other device was signed out."
	} else if signedOutDeviceCount > 1 {
		signedOut = fmt.Sprintf("Your %d other devices were signed out.", signedOutDeviceCount)
	}
	if change == apitypes.CredentialChangeSecretKey {
		return mail.Message{
			To:      recipient,
			Subject: "Your Hisar Secret Key was replaced",
			Body: fmt.Sprintf(`The Secret Key of your Hisar account %s was replaced on %s.

Your old Secret Key and Emergency Kit no longer work. Keep the new Emergency Kit somewhere safe.
%s To use Hisar on them again, sign in with your master password and the new Secret Key.

If you did not do this, contact support@tryhisar.com.
`, recipient, formatChangeTime(changedAt), signedOut),
		}
	}
	return mail.Message{
		To:      recipient,
		Subject: "Your Hisar master password was changed",
		Body: fmt.Sprintf(`The master password of your Hisar account %s was changed on %s.

%s To use Hisar on them again, sign in with the new master password.
Your Secret Key and Emergency Kit stay the same.

If you did not do this, contact support@tryhisar.com.
`, recipient, formatChangeTime(changedAt), signedOut),
	}
}

func composeAccountDeletedEmail(recipient string, deletedAt time.Time) mail.Message {
	return mail.Message{
		To:      recipient,
		Subject: "Your Hisar account was deleted",
		Body: fmt.Sprintf(`Your Hisar account %s was deleted on %s.

Deleted with it: your vaults that had no other owner and their items, your access to
vaults other people own, your devices and sessions, your share links, and the
invitations you sent or received. Vaults that have another owner stay with them.

The encrypted backups of the server are kept for 30 days, and its audit log for up to
a year, as the privacy policy says. Nobody, including Hisar, can read your items in them.

If you did not do this, contact support@tryhisar.com.
`, recipient, formatChangeTime(deletedAt)),
	}
}

func formatChangeTime(changedAt time.Time) string {
	return changedAt.UTC().Format("2 January 2006 at 15:04 UTC")
}
