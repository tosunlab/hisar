package app

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/cache"
)

type r08AppTransport func(*http.Request) (*http.Response, error)

func (f r08AppTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestR08FavoriteWriteNeverPrunesFromMixedUnknownCoverage(t *testing.T) {
	f := createUnlockedTestService(t)
	s := f.service
	missing := saveFavoriteTestItem(t, s, f.personalID, "Remote favorite not cached")
	healthy := saveFavoriteTestItem(t, s, f.familyID, "Healthy favorite")
	if err := s.SetItemFavorite(f.personalID, missing.ID, true); err != nil {
		t.Fatal(err)
	}
	c, err := cache.Open(filepath.Join(t.TempDir(), "new-cache.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	s.unlockedSession.Cache = c
	if _, err := s.unlockedSession.ReadVaults(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.unlockedSession.Client.HTTP.Transport = r08AppTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/v1/vaults/"+f.personalID+"/changes" {
			return &http.Response{StatusCode: 502, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"vault route unavailable"}`))}, nil
		}
		return http.DefaultTransport.RoundTrip(r)
	})
	for range 32 {
		if err := s.SetItemFavorite(f.familyID, healthy.ID, true); err != nil {
			t.Fatal(err)
		}
		ids := readFavoriteItemIDs(t, s)
		if !slices.Contains(ids, missing.ID) {
			t.Fatalf("ordinary favorite write pruned a live remote favorite from an unsynced mixed inventory: missing=%s saved=%v", missing.ID, ids)
		}
	}
}
