package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"regexp"
	"slices"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/server/internal/realtime"
	"github.com/tryhisar/hisar/server/internal/store"
)

const (
	MaxLiveShareLinkCount = 500
	shareLinkIDSize       = 16
)

var (
	shareLinkExpirySeconds = []int64{
		int64(apitypes.ShareLinkExpiryHour / time.Second), int64(apitypes.ShareLinkExpiryDay / time.Second),
		int64(apitypes.ShareLinkExpiryWeek / time.Second), int64(apitypes.ShareLinkExpiryMonth / time.Second),
	}
	shareLinkIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{22}$`)

	errTooManyShareLink     = newError(KindTooManyRequests, "too many share links today; try again tomorrow")
	errTooManyLiveShareLink = newError(KindTooManyRequests, "you have too many live share links; revoke some first")
)

func generateShareLinkID() string {
	randomBytes := make([]byte, shareLinkIDSize)
	rand.Read(randomBytes)
	return base64.RawURLEncoding.EncodeToString(randomBytes)
}

func validateShareLinkRequest(request *apitypes.CreateShareLinkRequest) error {
	switch {
	case len(request.Ciphertext) == 0 || len(request.Ciphertext) > apitypes.ShareLinkMaxCiphertextSize:
		return newError(KindInvalid, "the share link must be between 1 byte and 64 KiB")
	case len(request.EncryptedLabel) == 0 || len(request.EncryptedLabel) > apitypes.ShareLinkMaxLabelSize:
		return newError(KindInvalid, "the share link label must be between 1 byte and 4 KiB")
	case !slices.Contains(shareLinkExpirySeconds, request.ExpiresInSeconds):
		return newError(KindInvalid, "a share link expires after 1 hour, 1 day, 7 days or 30 days")
	case request.MaxViews < 0 || request.MaxViews > apitypes.ShareLinkMaxViews:
		return newError(KindInvalid, "a share link allows 1 to 100 views, or 0 for unlimited")
	}
	if request.Passphrase != nil {
		params := crypt.ShareLinkPassphraseParams{Salt: request.Passphrase.Salt, Iterations: request.Passphrase.Iterations}
		if params.Validate() != nil {
			return newError(KindInvalid, "the passphrase needs a 16-byte salt and 600000 to 10000000 iterations")
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

func (s *Service) publishShareLinksChanged(accountID string) {
	s.events.Publish([]string{accountID}, realtime.Event{Name: apitypes.EventShareLinksChanged, Data: struct{}{}})
}

// CreateShareLink stores a link's ciphertext; its key is in the link's fragment and never reaches the server.
func (s *Service) CreateShareLink(ctx context.Context, actor Actor, request *apitypes.CreateShareLinkRequest) (apitypes.ShareLink, error) {
	if err := validateShareLinkRequest(request); err != nil {
		return apitypes.ShareLink{}, err
	}
	if !s.Limiters.ShareLinkAccount.Allow(actor.AccountID) {
		return apitypes.ShareLink{}, errTooManyShareLink
	}
	link := &store.ShareLink{AccountID: actor.AccountID, Ciphertext: request.Ciphertext, EncryptedLabel: request.EncryptedLabel, MaxViews: request.MaxViews}
	if request.Passphrase != nil {
		link.PassphraseSalt, link.PassphraseIterations = request.Passphrase.Salt, request.Passphrase.Iterations
	}
	ttl := time.Duration(request.ExpiresInSeconds) * time.Second
	var err error
	for range 3 {
		link.ID = generateShareLinkID()
		if err = s.store.CreateShareLink(ctx, link, ttl, MaxLiveShareLinkCount); !errors.Is(err, store.ErrExists) {
			break
		}
	}
	if errors.Is(err, store.ErrConflict) {
		return apitypes.ShareLink{}, errTooManyLiveShareLink
	}
	if err != nil {
		return apitypes.ShareLink{}, err
	}
	s.recordAudit(ctx, actor, "share-link-create", map[string]string{"shareLinkID": link.ID})
	s.publishShareLinksChanged(actor.AccountID)
	response := convertToProtoShareLink(link)
	response.EncryptedLabel = nil
	return response, nil
}

func (s *Service) ListShareLinks(ctx context.Context, actor Actor) ([]apitypes.ShareLink, error) {
	links, err := s.store.ListShareLinks(ctx, actor.AccountID)
	if err != nil {
		return nil, err
	}
	responses := make([]apitypes.ShareLink, 0, len(links))
	for index := range links {
		responses = append(responses, convertToProtoShareLink(&links[index]))
	}
	return responses, nil
}

func (s *Service) RevokeShareLink(ctx context.Context, actor Actor, linkID string) error {
	if !shareLinkIDPattern.MatchString(linkID) {
		return ErrNotFound
	}
	if err := s.store.DeleteShareLink(ctx, actor.AccountID, linkID); errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	s.recordAudit(ctx, actor, "share-link-revoke", map[string]string{"shareLinkID": linkID})
	s.publishShareLinksChanged(actor.AccountID)
	return nil
}

// ViewShareLink counts a view; a malformed, unknown, expired, used-up and revoked ID are all ErrNotFound.
func (s *Service) ViewShareLink(ctx context.Context, linkID string) (apitypes.PublicShareLink, error) {
	if !shareLinkIDPattern.MatchString(linkID) {
		return apitypes.PublicShareLink{}, ErrNotFound
	}
	link, err := s.store.ViewShareLink(ctx, linkID)
	if errors.Is(err, store.ErrNotFound) {
		return apitypes.PublicShareLink{}, ErrNotFound
	}
	if err != nil {
		return apitypes.PublicShareLink{}, err
	}
	s.publishShareLinksChanged(link.AccountID)
	response := apitypes.PublicShareLink{Ciphertext: link.Ciphertext, ExpiresAt: link.ExpiresAt, MaxViews: link.MaxViews, ViewCount: link.ViewCount}
	if link.PassphraseSalt != nil {
		response.Passphrase = &apitypes.ShareLinkPassphrase{Salt: link.PassphraseSalt, Iterations: link.PassphraseIterations}
	}
	return response, nil
}
