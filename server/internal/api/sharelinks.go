package api

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/time/rate"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/server/internal/ratelimit"
)

const (
	shareLinkReadBurst    = 30
	shareLinkReadInterval = 2 * time.Second
)

func newShareLinkReadLimiter() *ratelimit.Limiter {
	return ratelimit.New(rate.Every(shareLinkReadInterval), shareLinkReadBurst)
}

func (s *Server) handleCreateShareLink(c fiber.Ctx) error {
	var request apitypes.CreateShareLinkRequest
	if err := decodeBodyWithLimit(c, &request, apitypes.ShareLinkMaxCiphertextSize*2); err != nil {
		return err
	}
	link, err := s.service.CreateShareLink(c.Context(), s.getActor(c), &request)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(link)
}

func (s *Server) handleListShareLinks(c fiber.Ctx) error {
	links, err := s.service.ListShareLinks(c.Context(), s.getActor(c))
	if err != nil {
		return err
	}
	return c.JSON(links)
}

func (s *Server) handleRevokeShareLink(c fiber.Ctx) error {
	if err := s.service.RevokeShareLink(c.Context(), s.getActor(c), c.Params("id")); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// handleViewShareLink is public: anyone with the link's ID may fetch it, so reads are limited per IP.
func (s *Server) handleViewShareLink(c fiber.Ctx) error {
	if !s.shareLinkReadLimiter.Allow(s.getClientIP(c)) {
		return createAPIError(fiber.StatusTooManyRequests, "too many requests")
	}
	link, err := s.service.ViewShareLink(c.Context(), c.Params("id"))
	if err != nil {
		return err
	}
	c.Set("Referrer-Policy", "no-referrer")
	c.Set("X-Robots-Tag", "noindex, nofollow")
	return c.JSON(link)
}
