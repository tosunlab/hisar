package api

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/server/internal/store"
)

const (
	maxRotatedAccessCount = 1000
	maxRotatedItemCount   = 50000
)

func (s *Server) handleRotateVault(c fiber.Ctx) error {
	currentSession := getSession(c)
	role, err := s.getVaultRole(c)
	if err != nil {
		return err
	}
	if role != proto.VaultOwner {
		return createAPIError(fiber.StatusForbidden, "owner only")
	}
	var request proto.RotateVaultRequest
	if err := decodeBodyWithLimit(c, &request, maxRotationBody); err != nil {
		return err
	}
	errInvalidRotation := createAPIError(fiber.StatusBadRequest, "invalid rotation")
	if len(request.EncName) == 0 || len(request.EncName) > maxEncName ||
		len(request.AllAccess) == 0 || len(request.AllAccess) > maxRotatedAccessCount ||
		len(request.AllItem) > maxRotatedItemCount || len(request.RecoveryWrappedKey) > maxWrappedKey {
		return errInvalidRotation
	}

	isRotatingOwnerKept := false
	allSeenAccountID := map[string]bool{}
	allMember := make([]store.Member, 0, len(request.AllAccess))
	for _, access := range request.AllAccess {
		if !crypt.IsValidAccountID(access.AccountID) || !isValidVaultRole(access.Role) || allSeenAccountID[access.AccountID] ||
			len(access.WrappedKey) == 0 || len(access.WrappedKey) > maxWrappedKey {
			return errInvalidRotation
		}
		allSeenAccountID[access.AccountID] = true
		if _, err := s.store.GetAccountByID(c.Context(), access.AccountID); errors.Is(err, store.ErrNotFound) {
			return createAPIError(fiber.StatusNotFound, "account not found")
		} else if err != nil {
			return err
		}
		if access.AccountID == currentSession.AccountID && access.Role == proto.VaultOwner {
			isRotatingOwnerKept = true
		}
		allMember = append(allMember, store.Member{AccountID: access.AccountID, Role: access.Role, WrappedKey: access.WrappedKey})
	}
	if !isRotatingOwnerKept {
		return createAPIError(fiber.StatusBadRequest, "the owner rotating the key must keep owner access")
	}

	allSeenItemID := map[string]bool{}
	allRotatedItem := make([]store.RotatedItem, 0, len(request.AllItem))
	for _, rotatedItem := range request.AllItem {
		if !isValidUUID(rotatedItem.ID) || !isValidUUID(rotatedItem.BaseRevision) || !isValidUUID(rotatedItem.Revision) ||
			rotatedItem.Revision == rotatedItem.BaseRevision || allSeenItemID[rotatedItem.ID] ||
			len(rotatedItem.Overview) == 0 || len(rotatedItem.Overview) > maxOverview ||
			len(rotatedItem.Details) == 0 || len(rotatedItem.Details) > maxDetails {
			return errInvalidRotation
		}
		allSeenItemID[rotatedItem.ID] = true
		allRotatedItem = append(allRotatedItem, store.RotatedItem{
			ID: rotatedItem.ID, BaseRevision: rotatedItem.BaseRevision, Revision: rotatedItem.Revision,
			Deleted: rotatedItem.Deleted, Overview: rotatedItem.Overview, Details: rotatedItem.Details,
		})
	}

	vaultID := c.Params("vault")
	allPreviousMember, err := s.store.ListAllMember(c.Context(), vaultID)
	if err != nil {
		return err
	}
	err = s.store.RotateVault(c.Context(), store.VaultRotation{
		VaultID: vaultID, EncName: request.EncName, AllAccess: allMember, AllItem: allRotatedItem, RotatedBy: currentSession.AccountID,
		RecoveryWrappedKey: request.RecoveryWrappedKey,
	})
	if errors.Is(err, store.ErrConflict) {
		return createAPIError(fiber.StatusConflict, "the vault changed during the key rotation; try again")
	}
	if errors.Is(err, store.ErrNotMember) {
		return createAPIError(fiber.StatusBadRequest, "a key rotation can only keep existing members")
	}
	if err != nil {
		return err
	}
	s.recordAudit(c, currentSession.AccountID, currentSession.DeviceID, "vault_rotated", map[string]string{
		"vault": vaultID, "members": strconv.Itoa(len(allMember)), "items": strconv.Itoa(len(allRotatedItem)),
	})
	allPreviousAccountID := make([]string, 0, len(allPreviousMember))
	for _, member := range allPreviousMember {
		allPreviousAccountID = append(allPreviousAccountID, member.AccountID)
	}
	s.publishVaultsChanged(allPreviousAccountID...)
	s.publishVaultChanged(c.Context(), vaultID, 0)
	return c.SendStatus(fiber.StatusNoContent)
}
