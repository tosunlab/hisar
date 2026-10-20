package api

import (
	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/core/apitypes"
)

func (s *Server) handleListOrganizations(c fiber.Ctx) error {
	return s.sendCollection(c)
}

func (s *Server) handleCreateOrganization(c fiber.Ctx) error {
	var request apitypes.CreateOrganizationRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	organization, err := s.service.CreateOrganization(c.Context(), s.getActor(c), &request)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(organization)
}

func (s *Server) handleGetOrganization(c fiber.Ctx) error {
	return s.sendCollection(c)
}

func (s *Server) handleRenameOrganization(c fiber.Ctx) error {
	var request apitypes.RenameOrganizationRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	if err := s.service.RenameOrganization(c.Context(), s.getActor(c), c.Params("organization"), &request); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleDeleteOrganization(c fiber.Ctx) error {
	if err := s.service.DeleteOrganization(c.Context(), s.getActor(c), c.Params("organization")); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleCreateOrganizationInvitation(c fiber.Ctx) error {
	var request apitypes.OrganizationInvitationRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	if err := s.service.CreateOrganizationInvitation(c.Context(), s.getActor(c), c.Params("organization"), &request); err != nil {
		return err
	}
	return c.Status(fiber.StatusAccepted).JSON(invitationSentResponse)
}

func (s *Server) handleCancelOrganizationInvitation(c fiber.Ctx) error {
	if err := s.service.CancelOrganizationInvitation(c.Context(), s.getActor(c), c.Params("organization"), c.Params("invitation")); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleAcceptOrganizationInvitation(c fiber.Ctx) error {
	if err := s.service.AnswerOrganizationInvitation(c.Context(), s.getActor(c), c.Params("invitation"), true); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleDeclineOrganizationInvitation(c fiber.Ctx) error {
	if err := s.service.AnswerOrganizationInvitation(c.Context(), s.getActor(c), c.Params("invitation"), false); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleSetOrganizationRole(c fiber.Ctx) error {
	var request apitypes.SetOrganizationRoleRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	if err := s.service.SetOrganizationRole(c.Context(), s.getActor(c), c.Params("organization"), c.Params("account"), &request); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleRemoveOrganizationMember(c fiber.Ctx) error {
	if err := s.service.RemoveOrganizationMember(c.Context(), s.getActor(c), c.Params("organization"), c.Params("account")); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleCreateTeam(c fiber.Ctx) error {
	var request apitypes.CreateTeamRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	team, err := s.service.CreateTeam(c.Context(), s.getActor(c), c.Params("organization"), &request)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(team)
}

func (s *Server) handleRenameTeam(c fiber.Ctx) error {
	var request apitypes.RenameTeamRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	if err := s.service.RenameTeam(c.Context(), s.getActor(c), c.Params("organization"), c.Params("team"), &request); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleDeleteTeam(c fiber.Ctx) error {
	if err := s.service.DeleteTeam(c.Context(), s.getActor(c), c.Params("organization"), c.Params("team")); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleAddTeamMember(c fiber.Ctx) error {
	if err := s.service.AddTeamMember(c.Context(), s.getActor(c), c.Params("organization"), c.Params("team"), c.Params("account")); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleRemoveTeamMember(c fiber.Ctx) error {
	if err := s.service.RemoveTeamMember(c.Context(), s.getActor(c), c.Params("organization"), c.Params("team"), c.Params("account")); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleGrantTeamVault(c fiber.Ctx) error {
	var request apitypes.GrantTeamVaultRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	if err := s.service.GrantTeamVault(c.Context(), s.getActor(c), c.Params("organization"), c.Params("team"), c.Params("vault"), &request); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleRevokeTeamVault(c fiber.Ctx) error {
	if err := s.service.RevokeTeamVault(c.Context(), s.getActor(c), c.Params("organization"), c.Params("team"), c.Params("vault")); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleListVaultKeyRequests(c fiber.Ctx) error {
	return s.sendCollection(c)
}
