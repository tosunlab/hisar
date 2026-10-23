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
	if !isValidCollectionPath(path) {
		return apitypes.CollectionPage{}, ErrNotFound
	}
	page, err := s.store.ReadCollectionPage(ctx, actor.AccountID, actor.DeviceID, path, cursor)
	return page, convertCollectionError(err)
}

// ReadCollection hands visit every page of a collection from one snapshot, for a peer that cannot read pages itself.
// An error visit returns stops the walk and comes back unchanged.
func (s *Service) ReadCollection(ctx context.Context, actor Actor, path string, visit func(apitypes.CollectionPage) error) error {
	if !isValidCollectionPath(path) {
		return ErrNotFound
	}
	return convertCollectionError(s.store.ReadCollection(ctx, actor.AccountID, actor.DeviceID, path, visit))
}

func isValidCollectionPath(path string) bool {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 3 && parts[0] == "admin" && parts[1] == "accounts" && !crypt.IsValidAccountID(parts[2]) {
		return false
	}
	if len(parts) > 1 && (parts[0] == "vaults" || parts[0] == "organizations") && !IsValidUUID(parts[1]) {
		return false
	}
	return len(parts) != 5 || IsValidUUID(parts[3])
}

func convertCollectionError(err error) error {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, store.ErrActorNotAdmin):
		return ErrAdminOnly
	case errors.Is(err, store.ErrCollectionChanged):
		return newError(KindConflict, apitypes.CollectionChangedMessage)
	case errors.Is(err, store.ErrConflict):
		return newError(KindInvalid, "invalid collection cursor")
	case errors.Is(err, store.ErrCollectionTooLarge):
		return newError(KindConflict, "this record exceeds the collection page byte budget")
	default:
		return err
	}
}
