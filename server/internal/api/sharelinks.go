package api

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"regexp"
	"slices"
	"time"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/time/rate"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/core/sharelink"
	"github.com/tryhisar/hisar/server/internal/store"
)

const (
	shareLinkPerAccountPerDay = 100
	shareLinkReadBurst        = 30
	shareLinkReadInterval     = 2 * time.Second
	shareLinkIDSize           = 16
)

var (
	allShareLinkExpirySeconds = []int64{3600, 86400, 7 * 86400, 30 * 86400}
	shareLinkIDPattern        = regexp.MustCompile(`^[A-Za-z0-9_-]{22}$`)

	errTooManyShareLink  = createAPIError(fiber.StatusTooManyRequests, "too many share links today; try again tomorrow")
	errShareLinkNotFound = createAPIError(fiber.StatusNotFound, "not found")
)

func createShareLinkReadLimiter() *ipLimiter {
	return createIPLimiter(rate.Every(shareLinkReadInterval), shareLinkReadBurst)
}

func generateShareLinkID() string {
	allRandomByte := make([]byte, shareLinkIDSize)
	rand.Read(allRandomByte)
	return base64.RawURLEncoding.EncodeToString(allRandomByte)
}

func validateShareLinkRequest(request *proto.CreateShareLinkRequest) error {
	switch {
	case len(request.Ciphertext) == 0 || len(request.Ciphertext) > sharelink.MaxCiphertextSize:
		return createAPIError(fiber.StatusBadRequest, "the share link must be between 1 byte and 64 KiB")
	case len(request.EncryptedLabel) == 0 || len(request.EncryptedLabel) > sharelink.MaxLabelSize:
		return createAPIError(fiber.StatusBadRequest, "the share link label must be between 1 byte and 4 KiB")
	case !slices.Contains(allShareLinkExpirySeconds, request.ExpiresInSeconds):
		return createAPIError(fiber.StatusBadRequest, "a share link expires after 1 hour, 1 day, 7 days or 30 days")
	case request.MaxViews < 1 || request.MaxViews > sharelink.MaxViews:
		return createAPIError(fiber.StatusBadRequest, "a share link allows 1 to 100 views")
	}
	if request.Passphrase != nil {
		params := crypt.ShareLinkPassphraseParams{Salt: request.Passphrase.Salt, Iterations: request.Passphrase.Iterations}
		if params.Validate() != nil {
			return createAPIError(fiber.StatusBadRequest, "the passphrase needs a 16-byte salt and 600000 to 10000000 iterations")
		}
	}
	return nil
}

func convertToProtoShareLink(link *store.ShareLink) proto.ShareLink {
	return proto.ShareLink{
		ID: link.ID, EncryptedLabel: link.EncryptedLabel, CreatedAt: link.CreatedAt, ExpiresAt: link.ExpiresAt,
		MaxViews: link.MaxViews, ViewCount: link.ViewCount, HasPassphrase: link.PassphraseSalt != nil,
	}
}

func (s *Server) publishShareLinksChanged(accountID string) {
	s.events.publish([]string{accountID}, serverEvent{name: proto.EventShareLinksChanged, data: struct{}{}})
}

func (s *Server) handleCreateShareLink(c fiber.Ctx) error {
	currentSession := getSession(c)
	var request proto.CreateShareLinkRequest
	if err := decodeBodyWithLimit(c, &request, sharelink.MaxCiphertextSize*2); err != nil {
		return err
	}
	if err := validateShareLinkRequest(&request); err != nil {
		return err
	}
	maxLiveCount, planName, err := s.checkShareLinkLimit(c.Context(), currentSession.AccountID, &request)
	if err != nil {
		return err
	}
	if !s.shareLinkAccountLimiter.allow(currentSession.AccountID) {
		return errTooManyShareLink
	}
	link := &store.ShareLink{
		AccountID: currentSession.AccountID, Ciphertext: request.Ciphertext, EncryptedLabel: request.EncryptedLabel, MaxViews: request.MaxViews,
	}
	if request.Passphrase != nil {
		link.PassphraseSalt, link.PassphraseIterations = request.Passphrase.Salt, request.Passphrase.Iterations
	}
	ttl := time.Duration(request.ExpiresInSeconds) * time.Second
	for range 3 {
		link.ID = generateShareLinkID()
		if err = s.store.CreateShareLink(c.Context(), link, ttl, maxLiveCount); !errors.Is(err, store.ErrExists) {
			break
		}
	}
	if errors.Is(err, store.ErrConflict) {
		liveCount, countErr := s.store.CountLiveShareLink(c.Context(), currentSession.AccountID)
		if countErr != nil {
			return countErr
		}
		return createPlanLimitError(proto.PlanLimitLiveShareLinks, int64(maxLiveCount), int64(liveCount), planName)
	}
	if err != nil {
		return err
	}
	s.recordAudit(c, currentSession.AccountID, currentSession.DeviceID, "share-link-create", map[string]string{"shareLinkID": link.ID})
	s.publishShareLinksChanged(currentSession.AccountID)
	response := convertToProtoShareLink(link)
	response.EncryptedLabel = nil
	return c.Status(fiber.StatusCreated).JSON(response)
}

func (s *Server) handleListAllShareLink(c fiber.Ctx) error {
	allLink, err := s.store.ListAllShareLink(c.Context(), getSession(c).AccountID)
	if err != nil {
		return err
	}
	allResponse := make([]proto.ShareLink, 0, len(allLink))
	for index := range allLink {
		allResponse = append(allResponse, convertToProtoShareLink(&allLink[index]))
	}
	return c.JSON(allResponse)
}

func (s *Server) handleRevokeShareLink(c fiber.Ctx) error {
	currentSession := getSession(c)
	linkID := c.Params("id")
	if !shareLinkIDPattern.MatchString(linkID) {
		return errShareLinkNotFound
	}
	if err := s.store.DeleteShareLink(c.Context(), currentSession.AccountID, linkID); errors.Is(err, store.ErrNotFound) {
		return errShareLinkNotFound
	} else if err != nil {
		return err
	}
	s.recordAudit(c, currentSession.AccountID, currentSession.DeviceID, "share-link-revoke", map[string]string{"shareLinkID": linkID})
	s.publishShareLinksChanged(currentSession.AccountID)
	return c.SendStatus(fiber.StatusNoContent)
}

// handleViewShareLink answers a malformed, unknown, expired, used-up and revoked ID with the same 404.
func (s *Server) handleViewShareLink(c fiber.Ctx) error {
	if !s.shareLinkReadLimiter.allow(s.getClientIP(c)) {
		return createAPIError(fiber.StatusTooManyRequests, "too many requests")
	}
	linkID := c.Params("id")
	if !shareLinkIDPattern.MatchString(linkID) {
		return errShareLinkNotFound
	}
	link, err := s.store.ViewShareLink(c.Context(), linkID)
	if errors.Is(err, store.ErrNotFound) {
		return errShareLinkNotFound
	}
	if err != nil {
		return err
	}
	s.publishShareLinksChanged(link.AccountID)
	response := proto.PublicShareLink{Ciphertext: link.Ciphertext, ExpiresAt: link.ExpiresAt, MaxViews: link.MaxViews, ViewCount: link.ViewCount}
	if link.PassphraseSalt != nil {
		response.Passphrase = &proto.ShareLinkPassphrase{Salt: link.PassphraseSalt, Iterations: link.PassphraseIterations}
	}
	c.Set("Referrer-Policy", "no-referrer")
	c.Set("X-Robots-Tag", "noindex, nofollow")
	return c.JSON(response)
}
