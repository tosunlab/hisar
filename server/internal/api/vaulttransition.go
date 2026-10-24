package api

import (
	"github.com/gofiber/fiber/v3"
	"github.com/tryhisar/hisar/core/apitypes"
)

func (s *Server) handleVaultTransitionProtocol(c fiber.Ctx) error {
	if err := s.requireVaultOwner(c); err != nil {
		return err
	}
	return c.JSON(struct {
		Protocol int `json:"protocol"`
	}{apitypes.VaultTransitionProtocol})
}

func (s *Server) handleBeginVaultTransition(c fiber.Ctx) error {
	var request apitypes.BeginVaultTransitionRequest
	if err := s.decodeOwnerBody(c, &request, maxBody); err != nil {
		return err
	}
	result, err := s.service.BeginVaultTransition(c.Context(), s.getActor(c), c.Params("vault"), &request)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(result)
}

func (s *Server) handleGetVaultTransition(c fiber.Ctx) error {
	result, err := s.service.GetVaultTransition(c.Context(), s.getActor(c), c.Params("vault"), c.Params("transition"))
	if err != nil {
		return err
	}
	return c.JSON(result)
}

func (s *Server) handleListVaultTransitionItems(c fiber.Ctx) error {
	result, err := s.service.ListVaultTransitionItems(c.Context(), s.getActor(c), c.Params("vault"), c.Params("transition"), c.Query("after"))
	if err != nil {
		return err
	}
	return c.JSON(result)
}

func (s *Server) handleStageVaultItems(c fiber.Ctx) error {
	var request apitypes.StageVaultItemsRequest
	if err := s.decodeOwnerBody(c, &request, apitypes.VaultTransitionPageBytes); err != nil {
		return err
	}
	if err := s.service.StageVaultItems(c.Context(), s.getActor(c), c.Params("vault"), c.Params("transition"), &request); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleStageVaultAccess(c fiber.Ctx) error {
	var request apitypes.StageVaultAccessRequest
	if err := s.decodeOwnerBody(c, &request, apitypes.VaultTransitionPageBytes); err != nil {
		return err
	}
	if err := s.service.StageVaultAccess(c.Context(), s.getActor(c), c.Params("vault"), c.Params("transition"), &request); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleCommitVaultTransition(c fiber.Ctx) error {
	var request apitypes.CommitVaultTransitionRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	result, err := s.service.CommitVaultTransition(c.Context(), s.getActor(c), c.Params("vault"), c.Params("transition"), &request)
	if err != nil {
		return err
	}
	return c.JSON(result)
}

func (s *Server) handleAbortVaultTransition(c fiber.Ctx) error {
	if err := s.service.AbortVaultTransition(c.Context(), s.getActor(c), c.Params("vault"), c.Params("transition")); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
