package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/cache"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
)

// A vault whose items were never synced to the end, read from the cache while its changes route is unreachable, holds
// no evidence that an item is absent: strict consumers refuse it and the read view says it is not complete.
func TestStrictConsumersRefuseAVaultThatNeverFinishedSyncing(t *testing.T) {
	f := createUnlockedTestService(t)
	s := f.service
	if _, err := s.SaveItem(f.personalID, ItemDetail{Title: "Remote only", Username: "alice", Password: "remote-secret", URLs: []string{"https://remote.example"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveItem(f.familyID, ItemDetail{Title: "Family synced", Password: "family-secret"}); err != nil {
		t.Fatal(err)
	}
	c, err := cache.Open(filepath.Join(t.TempDir(), "cache.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	s.unlockedSession.Cache = c
	s.unlockedSession.Client.HTTP.Transport = r08AppTransport(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/api/v1/vaults/"+f.personalID+"/changes" {
			return &http.Response{StatusCode: http.StatusBadGateway, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"unavailable"}`))}, nil
		}
		return http.DefaultTransport.RoundTrip(request)
	})
	if _, err := s.ListVaults(); err != nil {
		t.Fatal(err)
	}

	bridge := NewCommandLineBridge(s, clisocket.NewSessionApprovals(0, nil, nil))
	ctx := bridge.BindSession(context.Background())
	epoch := ctx.Value(sessionEpochKey{}).(uint64)
	if listed, err := bridge.answer(ctx, epoch, &cliprotocol.Request{Type: cliprotocol.TypeListItem, VaultID: f.personalID}); !errors.Is(err, session.ErrIncompleteRead) {
		t.Errorf("a strict item list claimed the never-synced vault is complete: %+v %v", listed, err)
	}
	read, err := bridge.answer(ctx, epoch, &cliprotocol.Request{Type: cliprotocol.TypeReadItem, VaultID: f.personalID})
	if err != nil {
		t.Fatal(err)
	}
	if provenance := read.(cliprotocol.ItemListResult).Provenance; provenance == nil || provenance.HasCompletedSync {
		t.Errorf("the read view did not say the never-synced vault is incomplete: %+v", read)
	}
	if read, err := bridge.answer(ctx, epoch, &cliprotocol.Request{Type: cliprotocol.TypeReadItem, VaultID: f.familyID}); err != nil || read.(cliprotocol.ItemListResult).Provenance == nil || !read.(cliprotocol.ItemListResult).Provenance.HasCompletedSync {
		t.Errorf("the read view of a synced vault: %+v %v", read, err)
	}
	if summaries, err := s.listBrowserItems(ctx, "", "", false); !errors.Is(err, session.ErrIncompleteRead) {
		t.Errorf("the extension's item list claimed every item: %+v %v", summaries, err)
	}
	if check, err := s.checkLogin(ctx, "https://remote.example", "alice", "remote-secret"); !errors.Is(err, session.ErrIncompleteRead) {
		t.Errorf("the save offer concluded %+v from a vault that never finished syncing: %v", check, err)
	}
	if imported, err := s.saveCommandLineImports(f.personalID, []item.Item{{Overview: item.Overview{Title: "Remote only"}, Details: item.Details{Password: "remote-secret"}}}); !errors.Is(err, session.ErrIncompleteRead) {
		t.Errorf("an import went into a vault whose items are not known: %+v %v", imported, err)
	}
	if resolved, err := s.resolveSecrets([]string{"hisar://Family/Family%20synced/password"}); err != nil || len(resolved.Values) != 1 || resolved.Values[0] != "family-secret" {
		t.Errorf("a synced vault's reference: %+v %v", resolved, err)
	}
}

// Saving into a vault the app has not read yet must not leave a view holding only the saved item.
func TestSavingIntoAnUnreadVaultKeepsItsOtherItemsVisible(t *testing.T) {
	f := createUnlockedTestService(t)
	s := f.service
	ctx := context.Background()
	vaults, err := s.unlockedSession.ListVaults(ctx)
	if err != nil {
		t.Fatal(err)
	}
	index := slices.IndexFunc(vaults, func(vault session.Vault) bool { return vault.ID == f.personalID })
	other := &item.Item{Overview: item.Overview{Title: "Saved elsewhere", Username: "alice", URLs: []string{"https://elsewhere.example"}}, Details: item.Details{Password: "elsewhere-secret"}}
	if _, err := s.unlockedSession.SaveItem(ctx, &vaults[index], other); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveItem(f.personalID, ItemDetail{Title: "Saved here", Password: "here-secret"}); err != nil {
		t.Fatal(err)
	}
	logins, err := ListLoginsForURL(s, "https://elsewhere.example")
	if err != nil || len(logins) != 1 || logins[0].ItemID != other.ID {
		t.Fatalf("AutoFill lost an item saved on another device: %+v %v", logins, err)
	}
	if check, err := s.checkLogin(ctx, "https://elsewhere.example", "alice", "elsewhere-secret"); err != nil || check.Status != "unchanged" {
		t.Fatalf("the save offer did not know the saved login: %+v %v", check, err)
	}
}
