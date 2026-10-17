package api

import "github.com/gofiber/fiber/v3"

func (s *Server) registerAdminServerRoutes(apiGroup fiber.Router) {
	apiGroup.Group("/admin").Get("/server", s.requireSession, requireAdmin, s.handleGetAdminServer)
}

func (s *Server) handleGetAdminServer(c fiber.Ctx) error {
	return c.JSON(s.service.GetAdminServer())
}
