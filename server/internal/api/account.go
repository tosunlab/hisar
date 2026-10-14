package api

import (
	"github.com/gofiber/fiber/v3"
)

func (s *Server) handleGetAccount(c fiber.Ctx) error {
	account, err := s.service.GetAccount(c.Context(), s.getActor(c))
	if err != nil {
		return err
	}
	return c.JSON(account)
}

func (s *Server) handleListMembers(c fiber.Ctx) error {
	members, err := s.service.ListMembers(c.Context(), s.getActor(c))
	if err != nil {
		return err
	}
	return c.JSON(members)
}

func (s *Server) handleListDevices(c fiber.Ctx) error {
	devices, err := s.service.ListDevices(c.Context(), s.getActor(c))
	if err != nil {
		return err
	}
	return c.JSON(devices)
}

func (s *Server) handleRevokeDevice(c fiber.Ctx) error {
	if err := s.service.RevokeDevice(c.Context(), s.getActor(c), c.Params("id")); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
