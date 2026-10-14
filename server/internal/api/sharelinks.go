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

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/server/internal/store"
)

const (
	shareLinkPerAccountPerDay = 100
	maxLiveShareLinkCount     = 500
	shareLinkReadBurst        = 30
	shareLinkReadInterval     = 2 * time.Second
	shareLinkIDSize           = 16
)

var (
	shareLinkExpirySeconds = []int64{
		int64(apitypes.ShareLinkExpiryHour / time.Second), int64(apitypes.ShareLinkExpiryDay / time.Second),
		int64(apitypes.ShareLinkExpiryWeek / time.Second), int64(apitypes.ShareLinkExpiryMonth / time.Second),
	}
	shareLinkIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{22}$`)

	errTooManyShareLink     = createAPIError(fiber.StatusTooManyRequests, "too many share links today; try again tomorrow")
	errTooManyLiveShareLink = createAPIError(fiber.StatusTooManyRequests, "you have too many live share links; revoke some first")
	errShareLinkNotFound    = createAPIError(fiber.StatusNotFound, "not found")
)

func createShareLinkReadLimiter() *ipLimiter {
	return createIPLimiter(rate.Every(shareLinkReadInterval), shareLinkReadBurst)
}

func generateShareLinkID() string {
	randomBytes := make([]byte, shareLinkIDSize)
	rand.Read(randomBytes)
	return base64.RawURLEncoding.EncodeToString(randomBytes)
}

func validateShareLinkRequest(request *apitypes.CreateShareLinkRequest) error {
	switch {
	case len(request.Ciphertext) == 0 || len(request.Ciphertext) > apitypes.ShareLinkMaxCiphertextSize:
		return createAPIError(fiber.StatusBadRequest, "the share link must be between 1 byte and 64 KiB")
	case len(request.EncryptedLabel) == 0 || len(request.EncryptedLabel) > apitypes.ShareLinkMaxLabelSize:
		return createAPIError(fiber.StatusBadRequest, "the share link label must be between 1 byte and 4 KiB")
	case !slices.Contains(shareLinkExpirySeconds, request.ExpiresInSeconds):
		return createAPIError(fiber.StatusBadRequest, "a share link expires after 1 hour, 1 day, 7 days or 30 days")
	case request.MaxViews < 0 || request.MaxViews > apitypes.ShareLinkMaxViews:
		return createAPIError(fiber.StatusBadRequest, "a share link allows 1 to 100 views, or 0 for unlimited")
	}
	if request.Passphrase != nil {
		params := crypt.ShareLinkPassphraseParams{Salt: request.Passphrase.Salt, Iterations: request.Passphrase.Iterations}
		if params.Validate() != nil {
			return createAPIError(fiber.StatusBadRequest, "the passphrase needs a 16-byte salt and 600000 to 10000000 iterations")
		}
	}
	return nil
}

func convertToProtoShareLink(link *store.ShareLink) apitypes.ShareLink {
	return apitypes.ShareLink{
		ID: link.ID, EncryptedLabel: link.EncryptedLabel, CreatedAt: link.CreatedAt, ExpiresAt: link.ExpiresAt,
		MaxViews: link.MaxViews, ViewCount: link.ViewCount, HasPassphrase: link.PassphraseSalt != nil,
	}
}

func (s *Server) publishShareLinksChanged(accountID string) {
	s.events.publish([]string{accountID}, serverEvent{name: apitypes.EventShareLinksChanged, data: struct{}{}})
}

func (s *Server) handleCreateShareLink(c fiber.Ctx) error {
	currentSession := getSession(c)
	var request apitypes.CreateShareLinkRequest
	if err := decodeBodyWithLimit(c, &request, apitypes.ShareLinkMaxCiphertextSize*2); err != nil {
		return err
	}
	if err := validateShareLinkRequest(&request); err != nil {
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
	var err error
	for range 3 {
		link.ID = generateShareLinkID()
		if err = s.store.CreateShareLink(c.Context(), link, ttl, maxLiveShareLinkCount); !errors.Is(err, store.ErrExists) {
			break
		}
	}
	if errors.Is(err, store.ErrConflict) {
		return errTooManyLiveShareLink
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

func (s *Server) handleListShareLinks(c fiber.Ctx) error {
	links, err := s.store.ListShareLinks(c.Context(), getSession(c).AccountID)
	if err != nil {
		return err
	}
	responses := make([]apitypes.ShareLink, 0, len(links))
	for index := range links {
		responses = append(responses, convertToProtoShareLink(&links[index]))
	}
	return c.JSON(responses)
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
	response := apitypes.PublicShareLink{Ciphertext: link.Ciphertext, ExpiresAt: link.ExpiresAt, MaxViews: link.MaxViews, ViewCount: link.ViewCount}
	if link.PassphraseSalt != nil {
		response.Passphrase = &apitypes.ShareLinkPassphrase{Salt: link.PassphraseSalt, Iterations: link.PassphraseIterations}
	}
	c.Set("Referrer-Policy", "no-referrer")
	c.Set("X-Robots-Tag", "noindex, nofollow")
	return c.JSON(response)
}
