package api

import (
	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/core/apitypes"
)

func refuseLegacyAuthentication(c fiber.Ctx) error {
	return c.Status(fiber.StatusBadRequest).JSON(apitypes.Error{Error: "Update your Hisar app or CLI to use origin-bound authentication. Legacy online login and master password checks are no longer supported."})
}

func (s *Server) handlePrelogin(c fiber.Ctx) error {
	response, err := s.service.Prelogin(c.Context(), c.Query("email"))
	if err != nil {
		return err
	}
	return c.JSON(response)
}

func (s *Server) handleRegisterStart(c fiber.Ctx) error {
	var request apitypes.RegisterStartRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	response, err := s.service.RegisterStart(c.Context(), &request)
	if err != nil {
		return err
	}
	return c.JSON(response)
}

func (s *Server) handleRegisterFinish(c fiber.Ctx) error {
	var request apitypes.RegisterFinishRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	response, err := s.service.RegisterFinish(c.Context(), s.getActor(c), &request)
	if err != nil {
		return err
	}
	return c.JSON(response)
}

func (s *Server) handleLoginStart(c fiber.Ctx) error {
	var request apitypes.LoginStartRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	response, err := s.service.LoginStart(c.Context(), &request)
	if err != nil {
		return err
	}
	return c.JSON(response)
}

func (s *Server) handleLoginFinish(c fiber.Ctx) error {
	var request apitypes.LoginFinishRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	response, err := s.service.LoginFinish(c.Context(), s.getActor(c), &request)
	if err != nil {
		return err
	}
	return c.JSON(response)
}

func (s *Server) handleLogout(c fiber.Ctx) error {
	if err := s.service.Logout(c.Context(), s.getActor(c)); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
