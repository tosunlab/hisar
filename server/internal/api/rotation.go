package api

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
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
	if role != apitypes.VaultOwner {
		return createAPIError(fiber.StatusForbidden, "owner only")
	}
	var request apitypes.RotateVaultRequest
	if err := decodeBodyWithLimit(c, &request, maxRotationBody); err != nil {
		return err
	}
	errInvalidRotation := createAPIError(fiber.StatusBadRequest, "invalid rotation")
	if len(request.EncName) == 0 || len(request.EncName) > maxEncName ||
		len(request.Accesses) == 0 || len(request.Accesses) > maxRotatedAccessCount ||
		len(request.Items) > maxRotatedItemCount {
		return errInvalidRotation
	}

	isRotatingOwnerKept := false
	seenAccountIDs := map[string]bool{}
	members := make([]store.Member, 0, len(request.Accesses))
	for _, access := range request.Accesses {
		if !crypt.IsValidAccountID(access.AccountID) || !apitypes.IsValidVaultRole(access.Role) || seenAccountIDs[access.AccountID] ||
			len(access.WrappedKey) == 0 || len(access.WrappedKey) > maxWrappedKey {
			return errInvalidRotation
		}
		seenAccountIDs[access.AccountID] = true
		if _, err := s.store.GetAccountByID(c.Context(), access.AccountID); errors.Is(err, store.ErrNotFound) {
			return createAPIError(fiber.StatusNotFound, "account not found")
		} else if err != nil {
			return err
		}
		if access.AccountID == currentSession.AccountID && access.Role == apitypes.VaultOwner {
			isRotatingOwnerKept = true
		}
		members = append(members, store.Member{AccountID: access.AccountID, Role: access.Role, WrappedKey: access.WrappedKey})
	}
	if !isRotatingOwnerKept {
		return createAPIError(fiber.StatusBadRequest, "the owner rotating the key must keep owner access")
	}

	seenItemIDs := map[string]bool{}
	rotatedItems := make([]store.RotatedItem, 0, len(request.Items))
	for _, rotatedItem := range request.Items {
		if !isValidUUID(rotatedItem.ID) || !isValidUUID(rotatedItem.BaseRevision) || !isValidUUID(rotatedItem.Revision) ||
			rotatedItem.Revision == rotatedItem.BaseRevision || seenItemIDs[rotatedItem.ID] ||
			len(rotatedItem.Overview) == 0 || len(rotatedItem.Overview) > maxOverview ||
			len(rotatedItem.Details) == 0 || len(rotatedItem.Details) > maxDetails {
			return errInvalidRotation
		}
		seenItemIDs[rotatedItem.ID] = true
		rotatedItems = append(rotatedItems, store.RotatedItem{
			ID: rotatedItem.ID, BaseRevision: rotatedItem.BaseRevision, Revision: rotatedItem.Revision,
			Deleted: rotatedItem.Deleted, Overview: rotatedItem.Overview, Details: rotatedItem.Details,
		})
	}

	vaultID := c.Params("vault")
	previousMembers, err := s.store.ListMembers(c.Context(), vaultID)
	if err != nil {
		return err
	}
	err = s.store.RotateVault(c.Context(), store.VaultRotation{
		VaultID: vaultID, EncName: request.EncName, Accesses: members, Items: rotatedItems, RotatedBy: currentSession.AccountID,
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
		"vault": vaultID, "members": strconv.Itoa(len(members)), "items": strconv.Itoa(len(rotatedItems)),
	})
	previousAccountIDs := make([]string, 0, len(previousMembers))
	for _, member := range previousMembers {
		previousAccountIDs = append(previousAccountIDs, member.AccountID)
	}
	s.publishVaultsChanged(previousAccountIDs...)
	s.publishVaultChanged(c.Context(), vaultID, 0)
	return c.SendStatus(fiber.StatusNoContent)
}
