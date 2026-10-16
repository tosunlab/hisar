package service

import (
	"context"
	"errors"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/server/internal/realtime"
	"github.com/tryhisar/hisar/server/internal/store"
)

var errPreferencesConflict = newError(KindConflict, "the preferences were changed on another device")

// GetAccountPreferences returns the caller's sealed record; the server cannot read it.
func (s *Service) GetAccountPreferences(ctx context.Context, actor Actor) (apitypes.AccountPreferences, error) {
	preferences, err := s.store.GetAccountPreferences(ctx, actor.AccountID)
	if err != nil {
		return apitypes.AccountPreferences{}, err
	}
	return apitypes.AccountPreferences{Revision: preferences.Revision, Ciphertext: preferences.Ciphertext}, nil
}

// PutAccountPreferences stores a new sealed record when request.BaseRevision is current, and tells the account's other devices.
func (s *Service) PutAccountPreferences(ctx context.Context, actor Actor, request *apitypes.PutAccountPreferencesRequest) (apitypes.PutAccountPreferencesResponse, error) {
	if request.BaseRevision < 0 || len(request.Ciphertext) == 0 || len(request.Ciphertext) > apitypes.AccountPreferencesMaxSize {
		return apitypes.PutAccountPreferencesResponse{}, newError(KindInvalid, "invalid preferences")
	}
	revision, err := s.store.PutAccountPreferences(ctx, actor.AccountID, request.BaseRevision, request.Ciphertext)
	if errors.Is(err, store.ErrConflict) {
		return apitypes.PutAccountPreferencesResponse{}, errPreferencesConflict
	}
	if err != nil {
		return apitypes.PutAccountPreferencesResponse{}, err
	}
	s.events.Publish([]string{actor.AccountID}, realtime.Event{Name: apitypes.EventAccountPreferencesChanged, Data: struct{}{}})
	return apitypes.PutAccountPreferencesResponse{Revision: revision}, nil
}
