package api

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/server/internal/store"
)

const (
	maxEncName    = 1024
	maxWrappedKey = 4096
	maxOverview   = 64 << 10
	maxDetails    = 1 << 20
	defaultLimit  = 500
	maxLimit      = 1000
)

func isValidUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil && len(value) == 36
}

func isValidVaultRole(role string) bool {
	return role == proto.VaultOwner || role == proto.VaultEditor || role == proto.VaultReader
}

// getVaultRole returns the caller's role; missing vaults and non-members both get 404.
func (s *Server) getVaultRole(c fiber.Ctx) (string, error) {
	vaultID := c.Params("vault")
	if !isValidUUID(vaultID) {
		return "", errNotFound
	}
	role, err := s.store.GetVaultRole(c.Context(), vaultID, getSession(c).AccountID)
	if errors.Is(err, store.ErrNotFound) {
		return "", errNotFound
	}
	return role, err
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

func (s *Server) handleListAllVault(c fiber.Ctx) error {
	allStoredVault, err := s.store.ListAllVault(c.Context(), getSession(c).AccountID)
	if err != nil {
		return err
	}
	allVault := make([]proto.Vault, 0, len(allStoredVault))
	for _, vault := range allStoredVault {
		allVault = append(allVault, proto.Vault{
			ID: vault.ID, Type: vault.Type, EncName: vault.EncName, Seq: vault.Seq, Role: vault.Role, WrappedKey: vault.WrappedKey,
		})
	}
	return c.JSON(allVault)
}

func (s *Server) handleCreateVault(c fiber.Ctx) error {
	currentSession := getSession(c)
	var request proto.CreateVaultRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	if !isValidUUID(request.ID) || (request.Type != proto.VaultPersonal && request.Type != proto.VaultShared) ||
		len(request.EncName) == 0 || len(request.EncName) > maxEncName ||
		len(request.WrappedKey) == 0 || len(request.WrappedKey) > maxWrappedKey {
		return createAPIError(fiber.StatusBadRequest, "invalid vault")
	}
	vault := &store.Vault{ID: request.ID, Type: request.Type, EncName: request.EncName, CreatedBy: currentSession.AccountID}
	switch err := s.store.CreateVault(c.Context(), vault, request.WrappedKey); {
	case errors.Is(err, store.ErrExists):
		return createAPIError(fiber.StatusConflict, "vault exists")
	case err != nil:
		return err
	}
	s.recordAudit(c, currentSession.AccountID, currentSession.DeviceID, "vault_created", map[string]string{"vault": vault.ID, "type": vault.Type})
	s.publishVaultsChanged(currentSession.AccountID)
	return c.Status(fiber.StatusCreated).JSON(proto.Vault{ID: vault.ID, Type: vault.Type, EncName: vault.EncName, Role: proto.VaultOwner, WrappedKey: request.WrappedKey})
}

func (s *Server) handleListAllVaultMember(c fiber.Ctx) error {
	if _, err := s.getVaultRole(c); err != nil {
		return err
	}
	allStoredMember, err := s.store.ListAllMember(c.Context(), c.Params("vault"))
	if err != nil {
		return err
	}
	allMember := make([]proto.VaultMember, 0, len(allStoredMember))
	for _, member := range allStoredMember {
		allMember = append(allMember, proto.VaultMember{AccountID: member.AccountID, Role: member.Role, GrantedBy: member.GrantedBy})
	}
	return c.JSON(allMember)
}

func (s *Server) handleSetAccess(c fiber.Ctx) error {
	currentSession := getSession(c)
	role, err := s.getVaultRole(c)
	if err != nil {
		return err
	}
	if role != proto.VaultOwner {
		return createAPIError(fiber.StatusForbidden, "owner only")
	}
	target := c.Params("account")
	if target == currentSession.AccountID {
		return createAPIError(fiber.StatusBadRequest, "cannot change own access")
	}
	var request proto.SetAccessRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	if !isValidVaultRole(request.Role) || len(request.WrappedKey) == 0 || len(request.WrappedKey) > maxWrappedKey {
		return createAPIError(fiber.StatusBadRequest, "invalid access")
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
	vaultID := c.Params("vault")
	isNewMember, err := s.store.GrantAccess(c.Context(), vaultID, store.Member{AccountID: target, Role: request.Role, WrappedKey: request.WrappedKey, GrantedBy: currentSession.AccountID})
	if errors.Is(err, store.ErrNoInvitation) {
		return createAPIError(fiber.StatusConflict, "this person has not accepted an invitation to this vault")
	}
	if err != nil {
		return err
	}
	s.recordAudit(c, currentSession.AccountID, currentSession.DeviceID, "vault_access_set", map[string]string{"vault": vaultID, "account": target, "role": request.Role})
	s.publishVaultsChanged(target)
	if isNewMember {
		s.publishShareInvitationsChangedToOwner(c.Context(), vaultID)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleRemoveAccess(c fiber.Ctx) error {
	currentSession := getSession(c)
	role, err := s.getVaultRole(c)
	if err != nil {
		return err
	}
	target := c.Params("account")
	vaultID := c.Params("vault")
	if target == currentSession.AccountID {
		return s.leaveVault(c, vaultID)
	}
	if role != proto.VaultOwner {
		return createAPIError(fiber.StatusForbidden, "owner only")
	}
	switch err := s.store.RemoveAccess(c.Context(), vaultID, target); {
	case errors.Is(err, store.ErrNotFound):
		return errNotFound
	case err != nil:
		return err
	}
	s.recordAudit(c, currentSession.AccountID, currentSession.DeviceID, "vault_access_removed", map[string]string{"vault": vaultID, "account": target})
	s.publishVaultsChanged(target)
	return c.SendStatus(fiber.StatusNoContent)
}

func convertToProtoItem(storeItem *store.Item) proto.Item {
	return proto.Item{ID: storeItem.ID, Revision: storeItem.Revision, Seq: storeItem.Seq, Deleted: storeItem.Deleted, Overview: storeItem.Overview, Details: storeItem.Details, UpdatedAt: storeItem.UpdatedAt, UpdatedBy: storeItem.UpdatedBy}
}

func (s *Server) handleListAllChange(c fiber.Ctx) error {
	if _, err := s.getVaultRole(c); err != nil {
		return err
	}
	var since int64
	if sinceQuery := c.Query("since"); sinceQuery != "" {
		var err error
		if since, err = strconv.ParseInt(sinceQuery, 10, 64); err != nil || since < 0 {
			return createAPIError(fiber.StatusBadRequest, "invalid since")
		}
	}
	limit := defaultLimit
	if limitQuery := c.Query("limit"); limitQuery != "" {
		var err error
		if limit, err = strconv.Atoi(limitQuery); err != nil || limit < 1 || limit > maxLimit {
			return createAPIError(fiber.StatusBadRequest, "invalid limit")
		}
	}

	allItem, vaultSeq, err := s.store.ListAllChange(c.Context(), c.Params("vault"), since, limit)
	if err != nil {
		return err
	}
	// NextSeq below the client's since signals a server rollback; it is not clamped.
	response := proto.ChangesResponse{Items: make([]proto.Item, 0, len(allItem)), NextSeq: vaultSeq}
	for index := range allItem {
		response.Items = append(response.Items, convertToProtoItem(&allItem[index]))
	}
	if len(allItem) == limit {
		response.More = true
		response.NextSeq = allItem[len(allItem)-1].Seq
	} else if len(allItem) > 0 {
		response.NextSeq = max(vaultSeq, allItem[len(allItem)-1].Seq)
	}
	return c.JSON(response)
}

func (s *Server) handlePutItem(c fiber.Ctx) error {
	currentSession := getSession(c)
	role, err := s.getVaultRole(c)
	if err != nil {
		return err
	}
	if role == proto.VaultReader {
		return createAPIError(fiber.StatusForbidden, "read only")
	}
	itemID := c.Params("item")
	var request proto.PutItemRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	baseRevision := strings.Trim(strings.TrimPrefix(c.Get("If-Match"), "W/"), `"`)
	if !isValidUUID(itemID) || !isValidUUID(request.Revision) || (baseRevision != "" && !isValidUUID(baseRevision)) ||
		len(request.Overview) == 0 || len(request.Overview) > maxOverview ||
		len(request.Details) == 0 || len(request.Details) > maxDetails {
		return createAPIError(fiber.StatusBadRequest, "invalid item")
	}

	storeItem := &store.Item{ID: itemID, Revision: request.Revision, Deleted: request.Deleted, Overview: request.Overview, Details: request.Details, UpdatedBy: currentSession.AccountID}
	currentStoreItem, err := s.store.PutItem(c.Context(), c.Params("vault"), baseRevision, storeItem)
	switch {
	case errors.Is(err, store.ErrConflict):
		conflictResponse := proto.ConflictResponse{}
		if currentStoreItem != nil {
			currentItem := convertToProtoItem(currentStoreItem)
			conflictResponse.Current = &currentItem
		}
		return c.Status(fiber.StatusConflict).JSON(conflictResponse)
	case err != nil:
		return err
	}
	s.publishVaultChanged(c.Context(), c.Params("vault"), storeItem.Seq)
	return c.JSON(proto.PutItemResponse{Seq: storeItem.Seq})
}

func (s *Server) handleGetItemHistory(c fiber.Ctx) error {
	if _, err := s.getVaultRole(c); err != nil {
		return err
	}
	itemID := c.Params("item")
	if !isValidUUID(itemID) {
		return errNotFound
	}
	allItem, err := s.store.GetItemHistory(c.Context(), c.Params("vault"), itemID)
	if err != nil {
		return err
	}
	allProtoItem := make([]proto.Item, 0, len(allItem))
	for index := range allItem {
		allProtoItem = append(allProtoItem, convertToProtoItem(&allItem[index]))
	}
	return c.JSON(allProtoItem)
}

// leaveVault removes the caller's access and wrapped key; the vault key is not rotated, since only owners can rotate.
func (s *Server) leaveVault(c fiber.Ctx, vaultID string) error {
	currentSession := getSession(c)
	switch err := s.store.LeaveVault(c.Context(), vaultID, currentSession.AccountID); {
	case errors.Is(err, store.ErrOnlyOwner):
		return createAPIError(fiber.StatusBadRequest, "the only owner can't leave a vault; delete it, or make someone else an owner first")
	case errors.Is(err, store.ErrNotFound):
		return errNotFound
	case err != nil:
		return err
	}
	s.recordAudit(c, currentSession.AccountID, currentSession.DeviceID, "vault_left", map[string]string{"vault": vaultID})
	s.publishVaultsChanged(currentSession.AccountID)
	s.publishVaultsChangedToAllMember(c, vaultID)
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleRenameVault(c fiber.Ctx) error {
	currentSession := getSession(c)
	if err := s.requireVaultOwner(c); err != nil {
		return err
	}
	var request proto.RenameVaultRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	if len(request.EncName) == 0 || len(request.EncName) > maxEncName {
		return createAPIError(fiber.StatusBadRequest, "invalid vault name")
	}
	vaultID := c.Params("vault")
	switch err := s.store.RenameVault(c.Context(), vaultID, request.EncName); {
	case errors.Is(err, store.ErrNotFound):
		return errNotFound
	case err != nil:
		return err
	}
	s.recordAudit(c, currentSession.AccountID, currentSession.DeviceID, "vault_renamed", map[string]string{"vault": vaultID})
	s.publishVaultsChangedToAllMember(c, vaultID)
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) publishVaultsChangedToAllMember(c fiber.Ctx, vaultID string) {
	allMember, err := s.store.ListAllMember(c.Context(), vaultID)
	if err != nil {
		return
	}
	allAccountID := make([]string, 0, len(allMember))
	for _, member := range allMember {
		allAccountID = append(allAccountID, member.AccountID)
	}
	s.publishVaultsChanged(allAccountID...)
}

func (s *Server) handleDeleteVault(c fiber.Ctx) error {
	currentSession := getSession(c)
	if err := s.requireVaultOwner(c); err != nil {
		return err
	}
	var request proto.DeleteVaultRequest
	if err := decodeBodyWithLimit(c, &request, maxRotationBody); err != nil {
		return err
	}
	errInvalidDeletion := createAPIError(fiber.StatusBadRequest, "invalid vault deletion")
	vaultID := c.Params("vault")
	if request.MoveToVaultID == "" && len(request.AllMovedItem) > 0 ||
		request.MoveToVaultID != "" && (!isValidUUID(request.MoveToVaultID) || request.MoveToVaultID == vaultID || request.IsDeletingItems) ||
		len(request.AllMovedItem) > maxRotatedItemCount {
		return errInvalidDeletion
	}
	allSeenItemID := map[string]bool{}
	allMovedItem := make([]store.MovedItem, 0, len(request.AllMovedItem))
	for _, movedItem := range request.AllMovedItem {
		if !isValidUUID(movedItem.ID) || !isValidUUID(movedItem.BaseRevision) || !isValidUUID(movedItem.Revision) ||
			allSeenItemID[movedItem.ID] ||
			len(movedItem.Overview) == 0 || len(movedItem.Overview) > maxOverview ||
			len(movedItem.Details) == 0 || len(movedItem.Details) > maxDetails {
			return errInvalidDeletion
		}
		allSeenItemID[movedItem.ID] = true
		allMovedItem = append(allMovedItem, store.MovedItem{
			ID: movedItem.ID, BaseRevision: movedItem.BaseRevision, Revision: movedItem.Revision, Overview: movedItem.Overview, Details: movedItem.Details,
		})
	}

	allFormerMemberID, err := s.store.DeleteVault(c.Context(), store.VaultDeletion{
		VaultID: vaultID, AccountID: currentSession.AccountID, MoveToVaultID: request.MoveToVaultID,
		AllMovedItem: allMovedItem, IsDeletingItems: request.IsDeletingItems,
	})
	switch {
	case errors.Is(err, store.ErrNotOwner):
		return createAPIError(fiber.StatusForbidden, "owner only")
	case errors.Is(err, store.ErrPersonalVault):
		return createAPIError(fiber.StatusBadRequest, "personal vaults can't be deleted")
	case errors.Is(err, store.ErrOnlyVault):
		return createAPIError(fiber.StatusBadRequest, "your only vault can't be deleted")
	case errors.Is(err, store.ErrVaultNotEmpty):
		return createAPIError(fiber.StatusConflict, "the vault has items; move them or delete them with the vault")
	case errors.Is(err, store.ErrConflict):
		return createAPIError(fiber.StatusConflict, "the vault changed while its items were moved; try again")
	case errors.Is(err, store.ErrExists):
		return createAPIError(fiber.StatusConflict, "the target vault already has one of these items")
	case errors.Is(err, store.ErrTargetNotWritable):
		return createAPIError(fiber.StatusForbidden, "you can't add items to the target vault")
	case errors.Is(err, store.ErrNotFound):
		return errNotFound
	case err != nil:
		return err
	}
	detail := map[string]string{"vault": vaultID, "members": strconv.Itoa(len(allFormerMemberID))}
	if request.MoveToVaultID != "" {
		detail["movedTo"], detail["items"] = request.MoveToVaultID, strconv.Itoa(len(allMovedItem))
	} else if request.IsDeletingItems {
		detail["itemsDeleted"] = "true"
	}
	s.recordAudit(c, currentSession.AccountID, currentSession.DeviceID, "vault_deleted", detail)
	s.publishVaultsChanged(allFormerMemberID...)
	if request.MoveToVaultID != "" {
		s.publishVaultChanged(c.Context(), request.MoveToVaultID, 0)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
