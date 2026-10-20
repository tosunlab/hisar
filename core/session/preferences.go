package session

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
)

const (
	favoriteItemIDsKey     = "favoriteItemIds"
	maxPreferencesAttempts = 5
)

var ErrPreferencesConflict = errors.New("your favorites were changed on another device at the same time; try again")

// Preferences is the account's own record, sealed with K_user: the server keeps only its ciphertext and revision.
// Fields a newer app added are kept as they are when this one saves.
type Preferences struct {
	Revision        int64
	FavoriteItemIDs []string
	// IsFromCache: the server could not be reached, so this is the copy cached on this device and may be old.
	IsFromCache bool
	otherFields map[string]json.RawMessage
}

// FavoriteChange stars and unstars items; it is applied to the newest record, so another device's changes stay.
type FavoriteChange struct {
	AddItemIDs    []string
	RemoveItemIDs []string
}

func (change FavoriteChange) apply(allFavoriteItemID []string) []string {
	result := []string{}
	for _, itemID := range allFavoriteItemID {
		if !slices.Contains(change.RemoveItemIDs, itemID) && !slices.Contains(result, itemID) {
			result = append(result, itemID)
		}
	}
	for _, itemID := range change.AddItemIDs {
		if itemID != "" && !slices.Contains(change.RemoveItemIDs, itemID) && !slices.Contains(result, itemID) {
			result = append(result, itemID)
		}
	}
	return result
}

// IsFavorite reports whether itemID is starred; a nil record has no favorites.
func (preferences *Preferences) IsFavorite(itemID string) bool {
	return preferences != nil && slices.Contains(preferences.FavoriteItemIDs, itemID)
}

func (s *Session) openPreferences(record *apitypes.AccountPreferences) (*Preferences, error) {
	preferences := &Preferences{Revision: record.Revision, FavoriteItemIDs: []string{}, otherFields: map[string]json.RawMessage{}}
	if len(record.Ciphertext) == 0 {
		return preferences, nil
	}
	plaintext, err := crypt.OpenAccountPreferences(s.Keyset.UserKey, s.AccountID, record.Ciphertext)
	if errors.Is(err, crypt.ErrDecrypt) || errors.Is(err, crypt.ErrShortCipher) || errors.Is(err, crypt.ErrBadVersion) {
		return nil, &unsealedPreferencesError{revision: record.Revision, err: err}
	}
	if err != nil {
		return nil, err
	}
	defer clear(plaintext)
	if err := json.Unmarshal(plaintext, &preferences.otherFields); err != nil {
		return nil, err
	}
	if encoded, isFound := preferences.otherFields[favoriteItemIDsKey]; isFound {
		if err := json.Unmarshal(encoded, &preferences.FavoriteItemIDs); err != nil {
			return nil, err
		}
		delete(preferences.otherFields, favoriteItemIDsKey)
	}
	if preferences.FavoriteItemIDs == nil {
		preferences.FavoriteItemIDs = []string{}
	}
	return preferences, nil
}

// unsealedPreferencesError marks an envelope this app cannot authenticate, which a session token alone can store.
// Writers keep this record on v1 and add new data in plaintext JSON fields.
type unsealedPreferencesError struct {
	revision int64
	err      error
}

func (err *unsealedPreferencesError) Error() string { return err.err.Error() }

func (err *unsealedPreferencesError) Unwrap() error { return err.err }

func (s *Session) sealPreferences(preferences *Preferences) ([]byte, error) {
	fields := map[string]any{}
	for key, value := range preferences.otherFields {
		fields[key] = value
	}
	fields[favoriteItemIDsKey] = preferences.FavoriteItemIDs
	plaintext, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	defer clear(plaintext)
	return crypt.SealAccountPreferences(s.Keyset.UserKey, s.AccountID, plaintext)
}

func (s *Session) fetchPreferences(ctx context.Context) (*Preferences, error) {
	record, err := s.Client.GetAccountPreferences(ctx)
	if err != nil {
		return nil, err
	}
	preferences, err := s.openPreferences(record)
	if err != nil {
		return nil, err
	}
	if s.Cache != nil {
		s.Cache.SaveAccountPreferences(s.AccountID, record)
	}
	return preferences, nil
}

// LoadPreferences reads the record from the server, or from the cache while the server is unreachable; with nothing
// cached it returns the unreachable error, so callers read it again once the server is back.
func (s *Session) LoadPreferences(ctx context.Context) (*Preferences, error) {
	preferences, err := s.fetchPreferences(ctx)
	if err == nil || s.Cache == nil || !apiclient.IsUnreachable(err) {
		return preferences, err
	}
	record, cacheErr := s.Cache.LoadAccountPreferences(s.AccountID)
	if cacheErr != nil || record == nil {
		return nil, err
	}
	preferences, openErr := s.openPreferences(record)
	if openErr != nil {
		return nil, openErr
	}
	preferences.IsFromCache = true
	return preferences, nil
}

// UpdateFavorites saves change on top of known, the record this device last read (nil to read it first). When another
// device saved in between, the server answers 409 and the change is applied again to the newer record. A record no key
// of the account sealed is replaced by known with the change, so it cannot stop favorites from being saved.
func (s *Session) UpdateFavorites(ctx context.Context, known *Preferences, change FavoriteChange) (*Preferences, error) {
	current := known
	for range maxPreferencesAttempts {
		isReplacing := false
		if current == nil {
			var err error
			current, err = s.fetchPreferences(ctx)
			var unsealed *unsealedPreferencesError
			switch {
			case errors.As(err, &unsealed):
				current, isReplacing = &Preferences{Revision: unsealed.revision, FavoriteItemIDs: []string{}, otherFields: map[string]json.RawMessage{}}, true
				if known != nil {
					current.FavoriteItemIDs, current.otherFields = known.FavoriteItemIDs, known.otherFields
				}
			case err != nil:
				return nil, err
			}
		}
		next := &Preferences{Revision: current.Revision, FavoriteItemIDs: change.apply(current.FavoriteItemIDs), otherFields: current.otherFields}
		if !isReplacing && slices.Equal(next.FavoriteItemIDs, current.FavoriteItemIDs) {
			if current != known {
				return current, nil
			}
			current = nil
			continue
		}
		ciphertext, err := s.sealPreferences(next)
		if err != nil {
			return nil, err
		}
		revision, err := s.Client.PutAccountPreferences(ctx, &apitypes.PutAccountPreferencesRequest{BaseRevision: current.Revision, Ciphertext: ciphertext})
		if apiclient.IsStatus(err, http.StatusConflict) {
			current = nil
			continue
		}
		if err != nil {
			return nil, err
		}
		next.Revision = revision
		if s.Cache != nil {
			s.Cache.SaveAccountPreferences(s.AccountID, &apitypes.AccountPreferences{Revision: revision, Ciphertext: ciphertext})
		}
		return next, nil
	}
	return nil, ErrPreferencesConflict
}
