package api

import (
	"context"
	"github.com/tryhisar/hisar/server/internal/ratelimit"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/server/internal/service"
)

// RunApprovalSweeps runs bounded maintenance until shutdown, including authentication state.
func (s *Server) RunApprovalSweeps(ctx context.Context) {
	go s.service.Approvals.Run(ctx, service.ApprovalRetention)
	go s.service.RunAuthenticationMaintenance(ctx)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, limiter := range []*ratelimit.Limiter{s.limiter, s.updateLimiter, s.shareLinkReadLimiter, s.invitePageLimiter, s.versionCheckLimiter} {
				limiter.Maintain()
			}
		}
	}
}

func (s *Server) limitApprovalRate(c fiber.Ctx) error {
	if !s.service.Approvals.Allow(getSession(c).AccountID) {
		return createAPIError(fiber.StatusTooManyRequests, "too many approval requests")
	}
	return c.Next()
}

func (s *Server) handleCreateApproval(c fiber.Ctx) error {
	var request apitypes.CreateApprovalRequest
	if err := decodeBodyWithLimit(c, &request, service.ApprovalMaximumBody); err != nil {
		return err
	}
	if err := s.service.CreateApproval(s.getActor(c), &request); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusCreated)
}

func (s *Server) handleListPendingApprovals(c fiber.Ctx) error {
	return c.JSON(s.service.ListPendingApprovals(s.getActor(c)))
}

func (s *Server) handleGetApproval(c fiber.Ctx) error {
	approval, err := s.service.GetApproval(s.getActor(c), c.Params("id"))
	if err != nil {
		return err
	}
	return c.JSON(approval)
}

func (s *Server) handleAnswerApproval(c fiber.Ctx) error {
	var request apitypes.AnswerApprovalRequest
	if err := decodeBodyWithLimit(c, &request, service.ApprovalMaximumBody); err != nil {
		return err
	}
	if err := s.service.AnswerApproval(s.getActor(c), c.Params("id"), &request); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleResolveApproval(c fiber.Ctx) error {
	if err := s.service.ResolveApproval(s.getActor(c), c.Params("id")); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
