package api

import (
	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/core/apitypes"
)

func (s *Server) registerAdminSettingsRoutes(adminGroup fiber.Router) {
	adminGroup.Get("/settings", s.requireSession, requireAdmin, s.handleGetSettings)
	adminGroup.Put("/settings", s.requireSession, requireAdmin, s.handleUpdateSettings)
	adminGroup.Post("/settings/test-email", s.requireSession, requireAdmin, s.handleSendTestEmail)
}

func (s *Server) handleGetSettings(c fiber.Ctx) error {
	described, err := s.service.GetSettings(c.Context())
	if err != nil {
		return err
	}
	return c.JSON(described)
}

func (s *Server) handleUpdateSettings(c fiber.Ctx) error {
	var request apitypes.AdminSettingsUpdate
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	described, err := s.service.UpdateSettings(c.Context(), s.getActor(c), &request)
	if err != nil {
		return err
	}
	return c.JSON(described)
}

func (s *Server) handleSendTestEmail(c fiber.Ctx) error {
	var request apitypes.AdminTestEmailRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	if err := s.service.SendTestEmail(c.Context(), s.getActor(c), &request); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
