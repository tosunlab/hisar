package api

import (
	"context"

	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/server/internal/service"
)

// RunServerUpdates runs ServerUpdates.Run; with update checks off it returns at once.
func (s *Server) RunServerUpdates(ctx context.Context) {
	if s.config.ServerUpdates != nil {
		s.config.ServerUpdates.Run(ctx)
	}
}

func (s *Server) registerAdminHealthRoutes(adminGroup fiber.Router) {
	adminGroup.Get("/health", s.requireSession, requireAdmin, s.handleGetAdminHealth)
}

func (s *Server) handleGetAdminHealth(c fiber.Ctx) error {
	health, err := s.service.GetAdminHealth(c.Context(), service.CheckRequest{ForwardedFor: c.Get(fiber.HeaderXForwardedFor), TrustsProxy: s.config.TrustProxy})
	if err != nil {
		return err
	}
	health.Version = s.config.Version
	if s.config.ServerUpdates != nil {
		health.LatestVersion = s.config.ServerUpdates.GetNewerVersion(s.config.Version)
	}
	return c.JSON(health)
}
