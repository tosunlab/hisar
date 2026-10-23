package api

import (
	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/core/apitypes"
)

func (s *Server) registerAdminAccountRoutes(adminGroup fiber.Router) {
	adminGroup.Get("/accounts", s.requireSession, requireAdmin, s.handleListAdminAccounts)
	adminGroup.Get("/accounts/:account", s.requireSession, requireAdmin, s.handleGetAdminAccount)
	adminGroup.Get("/accounts/:account/deletion-preview", s.requireSession, requireAdmin, s.handleGetAdminAccountDeletionPreview)
	adminGroup.Post("/accounts/:account/suspend", s.requireSession, requireAdmin, s.handleSuspendAccount)
	adminGroup.Post("/accounts/:account/reactivate", s.requireSession, requireAdmin, s.handleReactivateAccount)
	adminGroup.Put("/accounts/:account/role", s.requireSession, requireAdmin, s.handleChangeAccountRole)
	adminGroup.Post("/accounts/:account/delete", s.requireSession, requireAdmin, s.handleDeleteAccountAsAdmin)
	adminGroup.Post("/accounts/:account/sign-out", s.requireSession, requireAdmin, s.handleSignOutAccount)
	adminGroup.Delete("/accounts/:account/devices/:device", s.requireSession, requireAdmin, s.handleRevokeAccountDevice)
}

func (s *Server) handleListAdminAccounts(c fiber.Ctx) error {
	page, err := s.service.ListAdminAccounts(c.Context(), c.Query("query"), c.Query("cursor"))
	if err != nil {
		return err
	}
	return c.JSON(page)
}

func (s *Server) handleGetAdminAccount(c fiber.Ctx) error {
	detail, err := s.service.GetAdminAccount(c.Context(), c.Params("account"))
	if err != nil {
		return err
	}
	return c.JSON(detail)
}

func (s *Server) handleGetAdminAccountDeletionPreview(c fiber.Ctx) error {
	preview, err := s.service.PreviewAccountDeletionAsAdmin(c.Context(), s.getActor(c), c.Params("account"))
	if err != nil {
		return err
	}
	return c.JSON(preview)
}

func (s *Server) handleSuspendAccount(c fiber.Ctx) error {
	if err := s.service.SuspendAccount(c.Context(), s.getActor(c), c.Params("account")); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleReactivateAccount(c fiber.Ctx) error {
	if err := s.service.ReactivateAccount(c.Context(), s.getActor(c), c.Params("account")); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleChangeAccountRole(c fiber.Ctx) error {
	var request apitypes.AdminRoleChangeRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	if err := s.service.ChangeAccountRole(c.Context(), s.getActor(c), c.Params("account"), &request); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleDeleteAccountAsAdmin(c fiber.Ctx) error {
	var request apitypes.AdminAccountDeleteRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	if err := s.service.DeleteAccountAsAdmin(c.Context(), s.getActor(c), c.Params("account"), &request); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleSignOutAccount(c fiber.Ctx) error {
	if err := s.service.SignOutAccount(c.Context(), s.getActor(c), c.Params("account")); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleRevokeAccountDevice(c fiber.Ctx) error {
	if err := s.service.RevokeAccountDevice(c.Context(), s.getActor(c), c.Params("account"), c.Params("device")); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
