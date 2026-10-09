package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/server/internal/store"
)

var (
	errInvalidRecoveryLink = createAPIError(fiber.StatusForbidden, "invalid recovery link")
	errRecoveryNotSetUp    = createAPIError(fiber.StatusBadRequest, "recovery is not set up")
	errRecoverSelf         = createAPIError(fiber.StatusBadRequest, "you cannot recover your own account; ask another admin")
)

func (s *Server) checkRecoverySetUp(c fiber.Ctx) (bool, error) {
	_, err := s.store.GetRecoveryKeyset(c.Context())
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

func (s *Server) handleGetRecoveryKeyset(c fiber.Ctx) error {
	recoveryKeyset, err := s.store.GetRecoveryKeyset(c.Context())
	if errors.Is(err, store.ErrNotFound) {
		return errNotFound
	}
	if err != nil {
		return err
	}
	response := proto.RecoveryKeyset{
		PublicKeys: crypt.PublicKeys{KEM: recoveryKeyset.KEMPub, Sign: recoveryKeyset.SignPub, KEMSig: recoveryKeyset.KEMSig},
		CreatedBy:  recoveryKeyset.CreatedBy,
	}
	if recoveryKeyset.CreatedBy == getSession(c).AccountID {
		response.EncKeyset = recoveryKeyset.EncKeyset
	}
	return c.JSON(response)
}

func (s *Server) handleCreateRecoveryKeyset(c fiber.Ctx) error {
	currentSession := getSession(c)
	var request proto.CreateRecoveryKeysetRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	account, err := s.store.GetAccountByID(c.Context(), currentSession.AccountID)
	if err != nil {
		return err
	}
	if len(request.EncKeyset) == 0 || len(request.EncKeyset) > maxKeysetSize ||
		len(request.PublicKeys.KEM) == 0 || len(request.PublicKeys.KEM) > maxWrappedKey ||
		!bytes.Equal(request.PublicKeys.Sign, account.SignPub) ||
		request.PublicKeys.Verify(crypt.RecoveryRecipientID) != nil {
		return createAPIError(fiber.StatusBadRequest, "invalid recovery keyset")
	}
	err = s.store.CreateRecoveryKeyset(c.Context(), &store.RecoveryKeyset{
		KEMPub: request.PublicKeys.KEM, SignPub: request.PublicKeys.Sign, KEMSig: request.PublicKeys.KEMSig,
		EncKeyset: request.EncKeyset, CreatedBy: currentSession.AccountID,
	})
	if errors.Is(err, store.ErrExists) {
		return createAPIError(fiber.StatusConflict, "recovery is already set up")
	}
	if err != nil {
		return err
	}
	s.recordAudit(c, currentSession.AccountID, currentSession.DeviceID, "recovery_keyset_created", nil)
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) requireVaultOwner(c fiber.Ctx) error {
	role, err := s.getVaultRole(c)
	if err != nil {
		return err
	}
	if role != proto.VaultOwner {
		return createAPIError(fiber.StatusForbidden, "owner only")
	}
	return nil
}

func (s *Server) handleSetVaultRecovery(c fiber.Ctx) error {
	currentSession := getSession(c)
	if err := s.requireVaultOwner(c); err != nil {
		return err
	}
	var request proto.SetVaultRecoveryRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	if len(request.WrappedKey) == 0 || len(request.WrappedKey) > maxWrappedKey {
		return createAPIError(fiber.StatusBadRequest, "invalid wrapped key")
	}
	isRecoverySetUp, err := s.checkRecoverySetUp(c)
	if err != nil {
		return err
	}
	if !isRecoverySetUp {
		return errRecoveryNotSetUp
	}
	vaultID := c.Params("vault")
	if err := s.store.SetVaultRecovery(c.Context(), vaultID, request.WrappedKey); err != nil {
		return err
	}
	s.recordAudit(c, currentSession.AccountID, currentSession.DeviceID, "vault_recovery_set", map[string]string{"vault": vaultID})
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleRemoveVaultRecovery(c fiber.Ctx) error {
	currentSession := getSession(c)
	if err := s.requireVaultOwner(c); err != nil {
		return err
	}
	vaultID := c.Params("vault")
	vaultType, err := s.store.GetVaultType(c.Context(), vaultID)
	if err != nil {
		return err
	}
	if vaultType != proto.VaultPersonal {
		return createAPIError(fiber.StatusBadRequest, "only personal vaults can opt out of recovery")
	}
	if err := s.store.OptOutVaultRecovery(c.Context(), vaultID); err != nil {
		return err
	}
	s.recordAudit(c, currentSession.AccountID, currentSession.DeviceID, "vault_recovery_opt_out", map[string]string{"vault": vaultID})
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleCreateRecoveryLink(c fiber.Ctx) error {
	currentSession := getSession(c)
	target := c.Params("account")
	if target == currentSession.AccountID {
		return errRecoverSelf
	}
	errNoAccount := createAPIError(fiber.StatusNotFound, "account not found")
	if !crypt.IsValidAccountID(target) {
		return errNoAccount
	}
	if _, err := s.store.GetAccountByID(c.Context(), target); errors.Is(err, store.ErrNotFound) {
		return errNoAccount
	} else if err != nil {
		return err
	}
	token := generateToken()
	if err := s.store.CreateRecoveryToken(c.Context(), target, hashToken(token), currentSession.AccountID, s.config.RecoveryTTL); err != nil {
		return err
	}
	s.recordAudit(c, currentSession.AccountID, currentSession.DeviceID, "recovery_started", map[string]string{"account": target})
	return c.JSON(proto.RecoveryLinkResponse{Token: token, URL: strings.TrimRight(s.config.PublicURL, "/") + "/recover/" + token})
}

// getRecoveryAccount returns the account of a live recovery token, which must match email.
func (s *Server) getRecoveryAccount(c fiber.Ctx, token, email string) (*store.Account, error) {
	if token == "" {
		return nil, errInvalidRecoveryLink
	}
	account, err := s.store.GetRecoveryTokenAccount(c.Context(), hashToken(token))
	if errors.Is(err, store.ErrNotFound) || (err == nil && account.Email != normalizeEmail(email)) {
		return nil, errInvalidRecoveryLink
	}
	return account, err
}

func (s *Server) handleFindRecoveryAccount(c fiber.Ctx) error {
	var request proto.RecoveryAccountRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	account, err := s.getRecoveryAccount(c, request.Token, request.Email)
	if err != nil {
		return err
	}
	return c.JSON(proto.RecoveryAccountResponse{AccountID: account.ID})
}

func (s *Server) handleRecoveryStart(c fiber.Ctx) error {
	var request proto.RecoveryStartRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	account, err := s.getRecoveryAccount(c, request.Token, request.Email)
	if err != nil {
		return err
	}
	registrationRequest, err := s.opaque.Deserialize.RegistrationRequest(request.Request)
	if err != nil {
		return createAPIError(fiber.StatusBadRequest, "invalid registration request")
	}
	response, err := s.opaque.RegistrationResponse(registrationRequest, []byte(account.ID), nil)
	if err != nil {
		return createAPIError(fiber.StatusBadRequest, "invalid registration request")
	}
	return c.JSON(proto.RecoveryStartResponse{Response: response.Serialize()})
}

func (s *Server) handleRecoveryFinish(c fiber.Ctx) error {
	var request proto.RecoveryFinishRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	account, err := s.getRecoveryAccount(c, request.Token, request.Email)
	if err != nil {
		return err
	}
	switch {
	case len(request.Salt) != crypt.SaltSize,
		request.Argon2.Validate() != nil,
		len(request.EncKeyset) == 0 || len(request.EncKeyset) > maxKeysetSize,
		request.PublicKeys.Verify(account.ID) != nil,
		!isValidDevice(request.Device):
		return createAPIError(fiber.StatusBadRequest, "invalid recovery")
	}
	if _, err := s.opaque.Deserialize.RegistrationRecord(request.Record); err != nil {
		return createAPIError(fiber.StatusBadRequest, "invalid registration record")
	}
	argon2ParamsJSON, _ := json.Marshal(request.Argon2)
	err = s.store.SubmitRecovery(c.Context(), hashToken(request.Token), &store.PendingRecovery{
		AccountID:    account.ID,
		Argon2Params: argon2ParamsJSON,
		Salt:         request.Salt,
		OpaqueRecord: request.Record,
		EncKeyset:    request.EncKeyset,
		KEMPub:       request.PublicKeys.KEM,
		SignPub:      request.PublicKeys.Sign,
		KEMSig:       request.PublicKeys.KEMSig,
	})
	if errors.Is(err, store.ErrNotFound) {
		return errInvalidRecoveryLink
	}
	if err != nil {
		return err
	}
	s.recordAudit(c, account.ID, "", "recovery_submitted", map[string]string{"device": request.Device.Name, "platform": request.Device.Platform})
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleListAllPendingRecovery(c fiber.Ctx) error {
	allStoredPending, err := s.store.ListAllPendingRecovery(c.Context())
	if err != nil {
		return err
	}
	allPendingRecovery := make([]proto.PendingRecovery, 0, len(allStoredPending))
	for _, stored := range allStoredPending {
		pending := proto.PendingRecovery{
			AccountID:   stored.AccountID,
			Email:       stored.Email,
			Name:        stored.Name,
			PublicKeys:  crypt.PublicKeys{KEM: stored.KEMPub, Sign: stored.SignPub, KEMSig: stored.KEMSig},
			SubmittedAt: stored.SubmittedAt,
			AllVault:    make([]proto.RecoveryVault, 0, len(stored.AllVault)),
		}
		for _, recoveryVault := range stored.AllVault {
			pending.AllVault = append(pending.AllVault, proto.RecoveryVault{VaultID: recoveryVault.VaultID, Type: recoveryVault.Type, WrappedKey: recoveryVault.WrappedKey})
		}
		allPendingRecovery = append(allPendingRecovery, pending)
	}
	return c.JSON(allPendingRecovery)
}

func (s *Server) handleCompleteRecovery(c fiber.Ctx) error {
	currentSession := getSession(c)
	target := c.Params("account")
	if target == currentSession.AccountID {
		return errRecoverSelf
	}
	var request proto.CompleteRecoveryRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	errInvalidCompletion := createAPIError(fiber.StatusBadRequest, "invalid recovery completion")
	if len(request.AllVaultKey) > maxRotatedAccessCount {
		return errInvalidCompletion
	}
	allSeenVaultID := map[string]bool{}
	allVaultKey := make([]store.VaultKey, 0, len(request.AllVaultKey))
	for _, vaultKey := range request.AllVaultKey {
		if !isValidUUID(vaultKey.VaultID) || allSeenVaultID[vaultKey.VaultID] || len(vaultKey.WrappedKey) == 0 || len(vaultKey.WrappedKey) > maxWrappedKey {
			return errInvalidCompletion
		}
		allSeenVaultID[vaultKey.VaultID] = true
		allVaultKey = append(allVaultKey, store.VaultKey{VaultID: vaultKey.VaultID, WrappedKey: vaultKey.WrappedKey})
	}
	allUnrecoverableVaultID, err := s.store.CompleteRecovery(c.Context(), target, allVaultKey)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return createAPIError(fiber.StatusNotFound, "no pending recovery for this account")
	case errors.Is(err, store.ErrConflict):
		return createAPIError(fiber.StatusConflict, "the account's vaults changed; try again")
	case err != nil:
		return err
	}
	if allUnrecoverableVaultID == nil {
		allUnrecoverableVaultID = []string{}
	}
	s.recordAudit(c, currentSession.AccountID, currentSession.DeviceID, "recovery_completed", map[string]string{
		"account": target, "recovered": strconv.Itoa(len(allVaultKey)), "unrecoverable": strconv.Itoa(len(allUnrecoverableVaultID)),
	})
	s.publishVaultsChanged(target)
	return c.JSON(proto.CompleteRecoveryResponse{AllUnrecoverableVaultID: allUnrecoverableVaultID})
}
