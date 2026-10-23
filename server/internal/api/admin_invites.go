package api

import (
	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/core/apitypes"
)

func (s *Server) registerAdminInvitesRoutes(adminGroup fiber.Router) {
	adminGroup.Get("/invites", s.requireSession, requireAdmin, s.handleListAdminInvites)
	adminGroup.Post("/invites", s.requireSession, requireAdmin, s.handleCreateAdminInvite)
	adminGroup.Delete("/invites/:invite", s.requireSession, requireAdmin, s.handleRevokeAdminInvite)
	adminGroup.Post("/invites/:invite/resend", s.requireSession, requireAdmin, s.handleResendAdminInvite)
}

func (s *Server) handleListAdminInvites(c fiber.Ctx) error {
	list, err := s.service.ListAdminInvites(c.Context())
	if err != nil {
		return err
	}
	return c.JSON(list)
}

func (s *Server) handleCreateAdminInvite(c fiber.Ctx) error {
	var request apitypes.AdminInviteRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	created, err := s.service.CreateAdminInvite(c.Context(), s.getActor(c), &request)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(created)
}

func (s *Server) handleRevokeAdminInvite(c fiber.Ctx) error {
	if err := s.service.RevokeAdminInvite(c.Context(), s.getActor(c), c.Params("invite")); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleResendAdminInvite(c fiber.Ctx) error {
	var request apitypes.AdminInviteResendRequest
	if len(c.Body()) != 0 {
		if err := decodeBody(c, &request); err != nil {
			return err
		}
	}
	created, err := s.service.ResendAdminInvite(c.Context(), s.getActor(c), c.Params("invite"), &request)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(created)
}
