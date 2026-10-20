package api

import (
	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/core/apitypes"
)

// invitationSentResponse is the same for every invite and share invitation, whether or not the email has an account.
var invitationSentResponse = apitypes.InvitationSentResponse{Message: apitypes.InvitationSentMessage}

func (s *Server) handleCreateInvite(c fiber.Ctx) error {
	var request apitypes.InviteRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	if err := s.service.CreateInvite(c.Context(), s.getActor(c), &request); err != nil {
		return err
	}
	return c.Status(fiber.StatusAccepted).JSON(invitationSentResponse)
}

func (s *Server) handleCreateShareInvitation(c fiber.Ctx) error {
	// A non-owner, or a personal vault, is refused before the body is read.
	if err := s.service.CheckShareInvitation(c.Context(), s.getActor(c), c.Params("vault")); err != nil {
		return err
	}
	var request apitypes.ShareInvitationRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	if err := s.service.CreateShareInvitation(c.Context(), s.getActor(c), c.Params("vault"), &request); err != nil {
		return err
	}
	return c.Status(fiber.StatusAccepted).JSON(invitationSentResponse)
}

func (s *Server) handleListShareInvitations(c fiber.Ctx) error {
	invitations, err := s.service.ListShareInvitations(c.Context(), s.getActor(c))
	if err != nil {
		return err
	}
	return c.JSON(invitations)
}

func (s *Server) handleAcceptShareInvitation(c fiber.Ctx) error {
	if err := s.service.AnswerShareInvitation(c.Context(), s.getActor(c), c.Params("invitation"), true); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleDeclineShareInvitation(c fiber.Ctx) error {
	if err := s.service.AnswerShareInvitation(c.Context(), s.getActor(c), c.Params("invitation"), false); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleCancelShareInvitation(c fiber.Ctx) error {
	if err := s.service.CancelShareInvitation(c.Context(), s.getActor(c), c.Params("invitation")); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleCompleteSignedShare(c fiber.Ctx) error {
	var request apitypes.CompleteShareRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	if request.Intent.Context.ID != c.Params("invitation") {
		return fiber.ErrBadRequest
	}
	if err := s.service.CompleteSignedShare(c.Context(), s.getActor(c), &request); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
