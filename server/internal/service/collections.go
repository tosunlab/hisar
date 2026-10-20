package service

import (
	"context"
	"errors"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/server/internal/store"
	"strings"
)

func (s *Service) ReadCollectionPage(ctx context.Context, actor Actor, path, cursor string) (apitypes.CollectionPage, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 3 && parts[0] == "admin" && parts[1] == "accounts" && !crypt.IsValidAccountID(parts[2]) {
		return apitypes.CollectionPage{}, ErrNotFound
	}
	if len(parts) > 1 && (parts[0] == "vaults" || parts[0] == "organizations") && !IsValidUUID(parts[1]) {
		return apitypes.CollectionPage{}, ErrNotFound
	}
	if len(parts) == 5 && !IsValidUUID(parts[3]) {
		return apitypes.CollectionPage{}, ErrNotFound
	}
	page, err := s.store.ReadCollectionPage(ctx, actor.AccountID, actor.DeviceID, path, cursor)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return page, ErrNotFound
	case errors.Is(err, store.ErrActorNotAdmin):
		return page, ErrAdminOnly
	case errors.Is(err, store.ErrCollectionChanged):
		return page, newError(KindConflict, apitypes.CollectionChangedMessage)
	case errors.Is(err, store.ErrConflict):
		return page, newError(KindInvalid, "invalid collection cursor")
	case errors.Is(err, store.ErrCollectionTooLarge):
		return page, newError(KindConflict, "this record exceeds the collection page byte budget")
	default:
		return page, err
	}
}
