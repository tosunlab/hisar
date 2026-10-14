package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/bytemare/opaque"
	"github.com/gofiber/fiber/v3"
	"golang.org/x/time/rate"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/server/internal/mail"
	"github.com/tryhisar/hisar/server/internal/store"
)

func createReauthenticationLimiter() *ipLimiter {
	return createIPLimiter(rate.Every(30*time.Second), 5)
}

var (
	errReauthenticationFailed = createAPIError(fiber.StatusForbidden, "incorrect master password")
	errCredentialsChanged     = createAPIError(fiber.StatusConflict, "the master password or Secret Key was changed on another device; try again")
	errSessionEnded           = createAPIError(fiber.StatusUnauthorized, "unauthorized")
)

func (s *Server) handleReauthenticate(c fiber.Ctx) error {
	var request apitypes.ReauthenticateRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	currentSession := getSession(c)
	if !s.reauthenticationLimiter.allow(currentSession.AccountID) {
		return createAPIError(fiber.StatusTooManyRequests, "too many attempts; wait a minute and try again")
	}
	keyExchange1, err := s.opaque.Deserialize.KE1(request.KE1)
	if err != nil {
		return errInvalidLogin
	}
	account, err := s.store.GetAccountByID(c.Context(), currentSession.AccountID)
	if err != nil {
		return err
	}
	registrationRecord, err := s.opaque.Deserialize.RegistrationRecord(account.OpaqueRecord)
	if err != nil {
		return err
	}
	var registrationResponse []byte
	if len(request.RegistrationRequest) > 0 {
		registrationRequest, err := s.opaque.Deserialize.RegistrationRequest(request.RegistrationRequest)
		if err != nil {
			return createAPIError(fiber.StatusBadRequest, "invalid registration request")
		}
		response, err := s.opaque.RegistrationResponse(registrationRequest, []byte(account.ID), nil)
		if err != nil {
			return createAPIError(fiber.StatusBadRequest, "invalid registration request")
		}
		registrationResponse = response.Serialize()
	}
	clientRecord := &opaque.ClientRecord{RegistrationRecord: registrationRecord, CredentialIdentifier: []byte(account.ID), ClientIdentity: []byte(account.ID)}
	keyExchange2, serverOutput, err := s.opaque.GenerateKE2(keyExchange1, clientRecord)
	if err != nil {
		return errInvalidLogin
	}
	reauthenticationID := s.loginCache.put(&pendingLogin{
		accountID:        account.ID,
		clientMAC:        serverOutput.ClientMAC,
		sessionTokenHash: currentSession.TokenHash,
		recordHash:       store.HashOpaqueRecord(account.OpaqueRecord),
	})
	clear(serverOutput.SessionSecret)
	return c.JSON(apitypes.ReauthenticateResponse{ReauthenticationID: reauthenticationID, KE2: keyExchange2.Serialize(), RegistrationResponse: registrationResponse})
}

// finishReauthentication accepts a proof only from the session that started it.
func (s *Server) finishReauthentication(c fiber.Ctx, reauthenticationID string, encodedKeyExchange3 []byte) (*pendingLogin, error) {
	currentSession := getSession(c)
	pending := s.loginCache.take(reauthenticationID)
	if pending == nil || pending.sessionTokenHash == nil || pending.accountID != currentSession.AccountID || !bytes.Equal(pending.sessionTokenHash, currentSession.TokenHash) {
		return nil, createAPIError(fiber.StatusForbidden, "the master password check expired; try again")
	}
	keyExchange3, err := s.opaque.Deserialize.KE3(encodedKeyExchange3)
	if err != nil || s.opaque.LoginFinish(keyExchange3, pending.clientMAC) != nil {
		s.recordAudit(c, currentSession.AccountID, currentSession.DeviceID, "reauthentication_failed", nil)
		return nil, errReauthenticationFailed
	}
	return pending, nil
}

func (s *Server) handleChangeCredentials(c fiber.Ctx) error {
	var request apitypes.ChangeCredentialsRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	switch {
	case request.Change != apitypes.CredentialChangeMasterPassword && request.Change != apitypes.CredentialChangeSecretKey,
		len(request.Salt) != crypt.SaltSize,
		request.Argon2.Validate() != nil,
		len(request.EncKeyset) == 0 || len(request.EncKeyset) > maxKeysetSize:
		return createAPIError(fiber.StatusBadRequest, "invalid credentials")
	}
	if _, err := s.opaque.Deserialize.RegistrationRecord(request.Record); err != nil {
		return createAPIError(fiber.StatusBadRequest, "invalid registration record")
	}
	pending, err := s.finishReauthentication(c, request.ReauthenticationID, request.KE3)
	if err != nil {
		return err
	}
	currentSession := getSession(c)
	argon2ParamsJSON, _ := json.Marshal(request.Argon2)
	signedOutDeviceCount, err := s.store.UpdateAccountCredentials(c.Context(), &store.CredentialUpdate{
		AccountID:        currentSession.AccountID,
		TokenHash:        currentSession.TokenHash,
		ProvenRecordHash: pending.recordHash,
		Argon2Params:     argon2ParamsJSON,
		Salt:             request.Salt,
		OpaqueRecord:     request.Record,
		EncKeyset:        request.EncKeyset,
	})
	switch {
	case errors.Is(err, store.ErrConflict):
		return errCredentialsChanged
	case errors.Is(err, store.ErrNotFound):
		return errSessionEnded
	case err != nil:
		return err
	}
	s.events.endAllStream(currentSession.AccountID, currentSession.TokenHash)
	s.recordAudit(c, currentSession.AccountID, currentSession.DeviceID, "credentials_changed", map[string]any{"change": request.Change, "signedOutDevices": signedOutDeviceCount})
	if s.isEmailConfigured() {
		if account, err := s.store.GetAccountByID(c.Context(), currentSession.AccountID); err == nil {
			message := composeCredentialsChangedEmail(account.Email, request.Change, signedOutDeviceCount, time.Now())
			s.runInBackground(func(ctx context.Context) { s.sendEmail(ctx, message) })
		}
	}
	return c.JSON(apitypes.ChangeCredentialsResponse{SignedOutDeviceCount: signedOutDeviceCount})
}

func (s *Server) handleGetAccountDeletionPreview(c fiber.Ctx) error {
	preview, err := s.store.PreviewAccountDeletion(c.Context(), getSession(c).AccountID)
	if err != nil {
		return err
	}
	response := apitypes.AccountDeletionPreview{
		AllOwnedVault:           make([]apitypes.DeletionOwnedVault, 0, len(preview.AllOwnedVault)),
		AllMemberVault:          make([]apitypes.DeletionMemberVault, 0, len(preview.AllMemberVault)),
		ShareLinkCount:          preview.ShareLinkCount,
		SentInvitationCount:     preview.SentInvitationCount,
		ReceivedInvitationCount: preview.ReceivedInvitationCount,
		DeviceCount:             preview.DeviceCount,
	}
	for _, ownedVault := range preview.AllOwnedVault {
		response.AllOwnedVault = append(response.AllOwnedVault, apitypes.DeletionOwnedVault{VaultID: ownedVault.ID, Type: ownedVault.Type, OtherMemberCount: ownedVault.OtherMemberCount})
	}
	for _, memberVault := range preview.AllMemberVault {
		response.AllMemberVault = append(response.AllMemberVault, apitypes.DeletionMemberVault{VaultID: memberVault.ID, Type: memberVault.Type, Role: memberVault.Role})
	}
	return c.JSON(response)
}

func (s *Server) handleDeleteAccount(c fiber.Ctx) error {
	var request apitypes.DeleteAccountRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	currentSession := getSession(c)
	account, err := s.store.GetAccountByID(c.Context(), currentSession.AccountID)
	if err != nil {
		return err
	}
	if normalizeEmail(request.Email) != account.Email {
		return createAPIError(fiber.StatusBadRequest, "the email does not match this account")
	}
	pending, err := s.finishReauthentication(c, request.ReauthenticationID, request.KE3)
	if err != nil {
		return err
	}
	deleted, err := s.store.DeleteAccount(c.Context(), currentSession.AccountID, currentSession.TokenHash, pending.recordHash)
	switch {
	case errors.Is(err, store.ErrConflict):
		return errCredentialsChanged
	case errors.Is(err, store.ErrNotFound):
		return errSessionEnded
	case err != nil:
		return err
	}
	s.events.endAllStream(currentSession.AccountID, nil)
	s.approvals.removeAllForAccount(currentSession.AccountID)
	s.publishVaultsChanged(deleted.AllVaultMemberAccountID...)
	s.publishShareInvitationsChanged(deleted.AllInvitationAccountID...)
	s.recordAudit(c, currentSession.AccountID, "", "account_deleted", map[string]int{"deletedVaults": deleted.DeletedVaultCount})
	if s.isEmailConfigured() {
		message := composeAccountDeletedEmail(deleted.Email, time.Now())
		s.runInBackground(func(ctx context.Context) { s.sendEmail(ctx, message) })
	}
	return c.SendStatus(fiber.StatusNoContent)
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
