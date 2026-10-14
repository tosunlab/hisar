package api

import (
	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/core/apitypes"
)

func (s *Server) handleReauthenticate(c fiber.Ctx) error {
	var request apitypes.ReauthenticateRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	response, err := s.service.Reauthenticate(c.Context(), s.getActor(c), &request)
	if err != nil {
		return err
	}
	return c.JSON(response)
}

func (s *Server) handleChangeCredentials(c fiber.Ctx) error {
	var request apitypes.ChangeCredentialsRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	response, err := s.service.ChangeCredentials(c.Context(), s.getActor(c), &request)
	if err != nil {
		return err
	}
	return c.JSON(response)
}

func (s *Server) handleGetAccountDeletionPreview(c fiber.Ctx) error {
	preview, err := s.service.PreviewAccountDeletion(c.Context(), s.getActor(c))
	if err != nil {
		return err
	}
	return c.JSON(preview)
}

func (s *Server) handleDeleteAccount(c fiber.Ctx) error {
	var request apitypes.DeleteAccountRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	if err := s.service.DeleteAccount(c.Context(), s.getActor(c), &request); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
