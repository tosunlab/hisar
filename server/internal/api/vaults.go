package api

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/server/internal/service"
)

func (s *Server) handleListVaults(c fiber.Ctx) error {
	return s.sendCollection(c)
}

func (s *Server) handleCreateVault(c fiber.Ctx) error {
	var request apitypes.CreateVaultRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	vault, err := s.service.CreateVault(c.Context(), s.getActor(c), &request)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(vault)
}

func (s *Server) handleListVaultMembers(c fiber.Ctx) error {
	return s.sendCollection(c)
}

func (s *Server) handleSetAccess(c fiber.Ctx) error {
	var request apitypes.SetAccessRequest
	if err := s.decodeOwnerBody(c, &request, maxBody); err != nil {
		return err
	}
	if err := s.service.SetAccess(c.Context(), s.getActor(c), c.Params("vault"), c.Params("account"), &request); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleRemoveAccess(c fiber.Ctx) error {
	if err := s.service.RemoveAccess(c.Context(), s.getActor(c), c.Params("vault"), c.Params("account")); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// handleListChanges leaves the range checks to the service, which answers not found first for a vault the caller
// cannot see; a malformed number becomes an out-of-range one.
func (s *Server) handleListChanges(c fiber.Ctx) error {
	var since int64
	if sinceQuery := c.Query("since"); sinceQuery != "" {
		var err error
		if since, err = strconv.ParseInt(sinceQuery, 10, 64); err != nil {
			since = -1
		}
	}
	limit := service.DefaultChangeLimit
	if limitQuery := c.Query("limit"); limitQuery != "" {
		var err error
		if limit, err = strconv.Atoi(limitQuery); err != nil {
			limit = 0
		}
	}
	response, err := s.service.ListChanges(c.Context(), s.getActor(c), c.Params("vault"), since, limit)
	if err != nil {
		return err
	}
	return c.JSON(response)
}

func (s *Server) handlePutItem(c fiber.Ctx) error {
	// A reader is refused before the body is read; PutItem checks again.
	if err := s.service.CheckItemWrite(c.Context(), s.getActor(c), c.Params("vault")); err != nil {
		return err
	}
	var request apitypes.PutItemRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	baseRevision := strings.Trim(strings.TrimPrefix(c.Get("If-Match"), "W/"), `"`)
	seq, err := s.service.PutItem(c.Context(), s.getActor(c), c.Params("vault"), c.Params("item"), baseRevision, &request)
	if err != nil {
		return err
	}
	return c.JSON(apitypes.PutItemResponse{Seq: seq})
}

func (s *Server) handleMoveItem(c fiber.Ctx) error {
	// A reader is refused before the body is read; MoveItem checks again.
	if err := s.service.CheckItemWrite(c.Context(), s.getActor(c), c.Params("vault")); err != nil {
		return err
	}
	var request apitypes.MoveItemRequest
	if err := decodeBodyWithLimit(c, &request, maxMoveBody); err != nil {
		return err
	}
	response, err := s.service.MoveItem(c.Context(), s.getActor(c), c.Params("vault"), c.Params("item"), &request)
	if err != nil {
		return err
	}
	return c.JSON(response)
}

func (s *Server) handleGetItemHistory(c fiber.Ctx) error {
	return s.sendCollection(c)
}

func (s *Server) handleRenameVault(c fiber.Ctx) error {
	var request apitypes.RenameVaultRequest
	if err := s.decodeOwnerBody(c, &request, maxBody); err != nil {
		return err
	}
	if err := s.service.RenameVault(c.Context(), s.getActor(c), c.Params("vault"), &request); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleDeleteVault(c fiber.Ctx) error {
	var request apitypes.DeleteVaultRequest
	if err := s.decodeOwnerBody(c, &request, maxRotationBody); err != nil {
		return err
	}
	if err := s.service.DeleteVault(c.Context(), s.getActor(c), c.Params("vault"), &request); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleRotateVault(c fiber.Ctx) error {
	var request apitypes.RotateVaultRequest
	if err := s.decodeOwnerBody(c, &request, maxRotationBody); err != nil {
		return err
	}
	if err := s.service.RotateVault(c.Context(), s.getActor(c), c.Params("vault"), &request); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) requireVaultOwner(c fiber.Ctx) error {
	role, err := s.service.GetVaultRole(c.Context(), s.getActor(c), c.Params("vault"))
	if err != nil {
		return err
	}
	if role != apitypes.VaultOwner {
		return service.ErrOwnerOnly
	}
	return nil
}

// decodeOwnerBody refuses a caller who is not an owner of the vault before reading the body.
func (s *Server) decodeOwnerBody(c fiber.Ctx, target any, limit int) error {
	if err := s.requireVaultOwner(c); err != nil {
		return err
	}
	return decodeBodyWithLimit(c, target, limit)
}
