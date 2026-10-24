package api

import (
	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/core/apitypes"
)

func (s *Server) handleGetPushConfig(c fiber.Ctx) error {
	return c.JSON(s.service.GetPushConfig())
}

func (s *Server) handleSetDevicePush(c fiber.Ctx) error {
	var registration apitypes.PushRegistration
	if err := decodeBody(c, &registration); err != nil {
		return err
	}
	if err := s.service.SetDevicePush(c.Context(), s.getActor(c), &registration); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleDeleteDevicePush(c fiber.Ctx) error {
	if err := s.service.DeleteDevicePush(c.Context(), s.getActor(c)); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
