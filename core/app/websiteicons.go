package app

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/itemicon"
)

const (
	// websiteIconRefreshAfter is how long a fetched icon is shown before the website is asked again.
	websiteIconRefreshAfter = 30 * 24 * time.Hour
	// websiteIconRetryAfter is how long a website without a usable icon is left alone.
	websiteIconRetryAfter = 7 * 24 * time.Hour
	// websiteIconFailureRetryAfter is how long a fetch that failed (offline, a timeout) is not repeated in this session.
	websiteIconFailureRetryAfter = time.Hour
)

// websiteIconRecord is what the device caches for a domain, sealed: the icon, or none for a website without one.
type websiteIconRecord struct {
	Image     []byte    `json:"image,omitempty"`
	FetchedAt time.Time `json:"fetchedAt"`
}

// websiteIcons holds the website icons of the unlocked session, with its own mutex so fetches never hold s.mutex.
type websiteIcons struct {
	mutex sync.Mutex
	// keys is nil while locked; a fetch that started under other keys drops its result.
	keys            *crypt.WebsiteIconKeys
	recordsByLookup map[string]websiteIconRecord
	failedAt        map[string]time.Time
	fetches         map[string]*websiteIconFetch
}

type websiteIconFetch struct {
	done  chan struct{}
	image []byte
}

// IsWebsiteIconsEnabled reports whether logins show their website's icon, which the app fetches from the website itself.
func (s *Service) IsWebsiteIconsEnabled() bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return !s.state.IsWebsiteIconsDisabled
}

// SetWebsiteIconsEnabled turns website icons on or off for this device; turning them off forgets the cached icons.
func (s *Service) SetWebsiteIconsEnabled(isEnabled bool) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.state.IsWebsiteIconsDisabled = !isEnabled
	if !isEnabled {
		s.websiteIcons.mutex.Lock()
		s.websiteIcons.recordsByLookup, s.websiteIcons.failedAt = map[string]websiteIconRecord{}, map[string]time.Time{}
		s.websiteIcons.mutex.Unlock()
		s.getCache()
		if s.localCache != nil {
			s.localCache.ClearWebsiteIcons()
		}
	}
	return s.state.Save()
}

// PrepareItemIcon turns an image the user chose into the PNG an item's icon keeps: cropped to a square and scaled down
// to at most 128 pixels. SaveItem refuses an icon image that did not come from here.
func (s *Service) PrepareItemIcon(image []byte) ([]byte, error) {
	return itemicon.NormalizeImage(image)
}

// GetWebsiteIcon returns the icon of the website rawURL names, as a PNG, from this device's sealed cache or fetched
// from the website itself. It returns nil without an error when the website has none, when website icons are off,
// while locked, and for anything but a public https website.
func (s *Service) GetWebsiteIcon(rawURL string) ([]byte, error) {
	domain, err := itemicon.Domain(rawURL)
	if err != nil {
		return nil, nil
	}
	s.mutex.Lock()
	if s.unlockedSession == nil || s.state.IsWebsiteIconsDisabled {
		s.mutex.Unlock()
		return nil, nil
	}
	keys, err := s.getWebsiteIconKeys()
	if err != nil {
		s.mutex.Unlock()
		return nil, err
	}
	lookup := keys.Lookup(domain)
	now := s.config.Now()
	record, isCached := s.loadWebsiteIconRecord(keys, lookup)
	s.mutex.Unlock()
	if isCached {
		age := now.Sub(record.FetchedAt)
		if (record.Image != nil && age < websiteIconRefreshAfter) || (record.Image == nil && age < websiteIconRetryAfter) {
			return record.Image, nil
		}
	}

	icons := &s.websiteIcons
	icons.mutex.Lock()
	if failedAt, hasFailed := icons.failedAt[lookup]; hasFailed && now.Sub(failedAt) < websiteIconFailureRetryAfter {
		icons.mutex.Unlock()
		return record.Image, nil
	}
	if fetch, isRunning := icons.fetches[lookup]; isRunning {
		icons.mutex.Unlock()
		<-fetch.done
		return fetch.image, nil
	}
	fetch := &websiteIconFetch{done: make(chan struct{})}
	icons.fetches[lookup] = fetch
	icons.mutex.Unlock()

	image, fetchErr := s.getWebsiteIconFetcher().Fetch(context.Background(), "https://"+domain+"/")
	isMissing := errors.Is(fetchErr, itemicon.ErrNoIcon) || errors.Is(fetchErr, itemicon.ErrRefusedURL)
	if fetchErr != nil && !isMissing {
		// Offline or slow: keep showing what the cache had and try again later.
		image = record.Image
	}
	fetch.image = image
	s.mutex.Lock()
	icons.mutex.Lock()
	delete(icons.fetches, lookup)
	isCurrent := icons.keys == keys && s.unlockedSession != nil && !s.state.IsWebsiteIconsDisabled
	if isCurrent && fetchErr != nil && !isMissing {
		icons.failedAt[lookup] = s.config.Now()
	}
	icons.mutex.Unlock()
	if isCurrent && (fetchErr == nil || isMissing) {
		s.saveWebsiteIconRecord(keys, lookup, websiteIconRecord{Image: image, FetchedAt: s.config.Now()})
	}
	s.mutex.Unlock()
	close(fetch.done)
	return image, nil
}

// getWebsiteIconKeys derives the cache keys once per unlock. It needs s.mutex and an unlocked session.
func (s *Service) getWebsiteIconKeys() (*crypt.WebsiteIconKeys, error) {
	icons := &s.websiteIcons
	icons.mutex.Lock()
	defer icons.mutex.Unlock()
	if icons.keys == nil {
		keys, err := crypt.DeriveWebsiteIconKeys(s.unlockedSession.Keyset.UserKey, s.unlockedSession.AccountID)
		if err != nil {
			return nil, err
		}
		icons.keys = keys
		icons.recordsByLookup, icons.failedAt, icons.fetches = map[string]websiteIconRecord{}, map[string]time.Time{}, map[string]*websiteIconFetch{}
	}
	return icons.keys, nil
}

// forgetWebsiteIconKeys runs when the session locks; the cached icons stay sealed on disk.
func (s *Service) forgetWebsiteIconKeys() {
	icons := &s.websiteIcons
	icons.mutex.Lock()
	defer icons.mutex.Unlock()
	if icons.keys != nil {
		clear(icons.keys.SealKey)
		clear(icons.keys.LookupKey)
	}
	icons.keys, icons.recordsByLookup, icons.failedAt = nil, nil, nil
}

// loadWebsiteIconRecord reads a record from memory, then from the offline cache. It needs s.mutex.
func (s *Service) loadWebsiteIconRecord(keys *crypt.WebsiteIconKeys, lookup string) (websiteIconRecord, bool) {
	icons := &s.websiteIcons
	icons.mutex.Lock()
	record, isFound := icons.recordsByLookup[lookup]
	icons.mutex.Unlock()
	if isFound {
		return record, true
	}
	s.getCache()
	if s.localCache == nil {
		return websiteIconRecord{}, false
	}
	sealed, err := s.localCache.LoadWebsiteIcon(lookup)
	if err != nil || sealed == nil {
		return websiteIconRecord{}, false
	}
	plaintext, err := keys.Open(lookup, sealed)
	if err != nil || json.Unmarshal(plaintext, &record) != nil {
		return websiteIconRecord{}, false
	}
	icons.mutex.Lock()
	icons.recordsByLookup[lookup] = record
	icons.mutex.Unlock()
	return record, true
}

// saveWebsiteIconRecord keeps a record in memory and, sealed, in the offline cache. It needs s.mutex.
func (s *Service) saveWebsiteIconRecord(keys *crypt.WebsiteIconKeys, lookup string, record websiteIconRecord) {
	icons := &s.websiteIcons
	icons.mutex.Lock()
	icons.recordsByLookup[lookup] = record
	icons.mutex.Unlock()
	s.getCache()
	if s.localCache == nil {
		return
	}
	plaintext, err := json.Marshal(record)
	if err != nil {
		return
	}
	if sealed, err := keys.Seal(lookup, plaintext); err == nil {
		s.localCache.SaveWebsiteIcon(lookup, sealed)
	}
}

func (s *Service) getWebsiteIconFetcher() WebsiteIconFetcher {
	if s.config.WebsiteIconFetcher != nil {
		return s.config.WebsiteIconFetcher
	}
	s.websiteIcons.mutex.Lock()
	defer s.websiteIcons.mutex.Unlock()
	if s.defaultWebsiteIconFetcher == nil {
		s.defaultWebsiteIconFetcher = itemicon.NewFetcher(nil)
	}
	return s.defaultWebsiteIconFetcher
}
