package api

import (
	"context"

	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/core/apitypes"
)

// registerSetupRoutes adds the setup page's API. GET /setup answers before anyone signs in, so it says only which step
// setup is at; each step after the first admin's sign-up needs that admin.
func (s *Server) registerSetupRoutes(apiGroup, adminGroup fiber.Router) {
	apiGroup.Get("/setup", s.handleGetSetup)
	adminGroup.Get("/setup/checks", s.requireSession, requireAdmin, s.handleListSetupChecks)
	adminGroup.Post("/setup/checks", s.requireSession, requireAdmin, s.handleFinishChecksStep)
	adminGroup.Post("/setup/invite-policy", s.requireSession, requireAdmin, s.handleFinishInvitePolicyStep)
}

// PrepareSetup starts or resumes setup when the server starts, and returns the setup page's URL while setup needs its
// first admin, or "".
func (s *Server) PrepareSetup(ctx context.Context) (string, error) {
	return s.service.PrepareSetup(ctx)
}

func (s *Server) handleGetSetup(c fiber.Ctx) error {
	return c.JSON(s.service.GetSetup())
}

func (s *Server) handleListSetupChecks(c fiber.Ctx) error {
	return c.JSON(apitypes.SetupChecks{Checks: s.service.RunInstanceChecks(c.Context(), s.getCheckRequest(c))})
}

func (s *Server) handleFinishChecksStep(c fiber.Ctx) error {
	if err := s.service.FinishChecksStep(c.Context(), s.getActor(c), s.getCheckRequest(c)); err != nil {
		return err
	}
	return c.JSON(s.service.GetSetup())
}

func (s *Server) handleFinishInvitePolicyStep(c fiber.Ctx) error {
	var request apitypes.SetupInvitePolicyRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	if err := s.service.FinishInvitePolicyStep(c.Context(), s.getActor(c), &request); err != nil {
		return err
	}
	return c.JSON(s.service.GetSetup())
}
