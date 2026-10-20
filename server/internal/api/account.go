package api

import (
	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/core/apitypes"
)

func (s *Server) handleGetAccount(c fiber.Ctx) error {
	account, err := s.service.GetAccount(c.Context(), s.getActor(c))
	if err != nil {
		return err
	}
	return c.JSON(account)
}

func (s *Server) handleListMembers(c fiber.Ctx) error {
	return s.sendCollection(c)
}

func (s *Server) handleListDevices(c fiber.Ctx) error {
	return s.sendCollection(c)
}

func (s *Server) handleRevokeDevice(c fiber.Ctx) error {
	if err := s.service.RevokeDevice(c.Context(), s.getActor(c), c.Params("id")); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleGetAccountPreferences(c fiber.Ctx) error {
	preferences, err := s.service.GetAccountPreferences(c.Context(), s.getActor(c))
	if err != nil {
		return err
	}
	return c.JSON(preferences)
}

func (s *Server) handlePutAccountPreferences(c fiber.Ctx) error {
	var request apitypes.PutAccountPreferencesRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	response, err := s.service.PutAccountPreferences(c.Context(), s.getActor(c), &request)
	if err != nil {
		return err
	}
	return c.JSON(response)
}
