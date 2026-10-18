package app

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/itemicon"
)

func encodeTestIconPNG(t *testing.T, side int) []byte {
	t.Helper()
	picture := image.NewNRGBA(image.Rect(0, 0, side, side))
	for x := range side {
		for y := range side {
			picture.SetNRGBA(x, y, color.NRGBA{uint8(x), uint8(y), 90, 255})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, picture); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestSaveItemKeepsTheChosenIcon(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	saved, err := service.SaveItem(test.personalID, ItemDetail{Title: "Bank", Password: "bank-password", Icon: &ItemIcon{Preset: "bank", Color: "green"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(saved.Icon, &ItemIcon{Preset: "bank", Color: "green"}) {
		t.Errorf("preset icon = %+v", saved.Icon)
	}
	if listed := listItemsByTitle(t, service, test.personalID)["Bank"]; !reflect.DeepEqual(listed.Icon, saved.Icon) {
		t.Errorf("the list shows %+v", listed.Icon)
	}

	upload, err := service.PrepareItemIcon(encodeTestIconPNG(t, 600))
	if err != nil {
		t.Fatal(err)
	}
	saved.Icon = &ItemIcon{Preset: "bank", Image: upload}
	saved, err = service.SaveItem(test.personalID, saved)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Icon == nil || saved.Icon.Preset != "" || !bytes.Equal(saved.Icon.Image, upload) {
		t.Errorf("image icon = %+v", saved.Icon)
	}
	duplicate, err := service.DuplicateItem(test.personalID, saved.ID)
	if err != nil || duplicate.Icon == nil || !bytes.Equal(duplicate.Icon.Image, upload) {
		t.Errorf("duplicate icon = %+v, %v", duplicate.Icon, err)
	}
	moved, err := service.MoveItem(test.personalID, saved.ID, test.familyID)
	if err != nil || moved.Icon == nil || !bytes.Equal(moved.Icon.Image, upload) {
		t.Errorf("moved icon = %+v, %v", moved.Icon, err)
	}

	moved.Icon = nil
	if cleared, err := service.SaveItem(test.familyID, moved); err != nil || cleared.Icon != nil {
		t.Errorf("removing the icon: %+v, %v", cleared.Icon, err)
	}
	if _, err := service.SaveItem(test.personalID, ItemDetail{Title: "Bad", Icon: &ItemIcon{Image: encodeTestIconPNG(t, 600)}}); err == nil ||
		!strings.Contains(err.Error(), "choose a PNG, JPEG, GIF or WebP image") {
		t.Errorf("an image PrepareItemIcon did not make was saved: %v", err)
	}
}

type fakeWebsiteIconFetcher struct {
	mutex    sync.Mutex
	icons    map[string][]byte
	err      error
	calls    atomic.Int32
	isPaused chan struct{}
}

func (fetcher *fakeWebsiteIconFetcher) Fetch(ctx context.Context, rawURL string) ([]byte, error) {
	fetcher.calls.Add(1)
	if fetcher.isPaused != nil {
		<-fetcher.isPaused
	}
	fetcher.mutex.Lock()
	defer fetcher.mutex.Unlock()
	if fetcher.err != nil {
		return nil, fetcher.err
	}
	if icon, isFound := fetcher.icons[rawURL]; isFound {
		return icon, nil
	}
	return nil, itemicon.ErrNoIcon
}

func TestGetWebsiteIconCachesAndRefreshes(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	icon := encodeTestIconPNG(t, 64)
	fetcher := &fakeWebsiteIconFetcher{icons: map[string][]byte{"https://github.com/": icon}}
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	service.config.WebsiteIconFetcher = fetcher
	service.config.Now = func() time.Time { return now }

	for range 2 {
		if got, err := service.GetWebsiteIcon("https://github.com/login"); err != nil || !bytes.Equal(got, icon) {
			t.Fatalf("GetWebsiteIcon = %d bytes, %v", len(got), err)
		}
	}
	if missing, err := service.GetWebsiteIcon("https://no-icon.example.com"); err != nil || missing != nil {
		t.Errorf("a website without an icon: %v, %v", missing, err)
	}
	service.GetWebsiteIcon("https://no-icon.example.com")
	if calls := fetcher.calls.Load(); calls != 2 {
		t.Errorf("%d fetches, want one per website", calls)
	}
	for _, refused := range []string{"http://github.com", "https://192.168.1.1", "ssh://server", ""} {
		if got, _ := service.GetWebsiteIcon(refused); got != nil || fetcher.calls.Load() != 2 {
			t.Errorf("%q was fetched", refused)
		}
	}

	// The cache outlives the memory of a session, sealed: the file holds no domain.
	cacheFile, err := os.ReadFile(filepath.Join(filepath.Dir(service.config.StatePath), "desktop-cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(cacheFile, []byte("github.com")) || bytes.Contains(cacheFile, []byte("no-icon")) {
		t.Error("the cache file names a website")
	}
	service.mutex.Lock()
	service.forgetWebsiteIconKeys()
	service.mutex.Unlock()
	if got, _ := service.GetWebsiteIcon("https://github.com"); !bytes.Equal(got, icon) || fetcher.calls.Load() != 2 {
		t.Errorf("from the cache file: %d bytes, %d fetches", len(got), fetcher.calls.Load())
	}

	// Past the refresh time the website is asked again; while offline the old icon stays.
	now = now.Add(31 * 24 * time.Hour)
	fetcher.err = context.DeadlineExceeded
	if got, _ := service.GetWebsiteIcon("https://github.com"); !bytes.Equal(got, icon) || fetcher.calls.Load() != 3 {
		t.Errorf("offline refresh: %d bytes, %d fetches", len(got), fetcher.calls.Load())
	}
	service.GetWebsiteIcon("https://github.com")
	if fetcher.calls.Load() != 3 {
		t.Error("a failed fetch was repeated at once")
	}
	service.Lock()
	if got, _ := service.GetWebsiteIcon("https://github.com"); got != nil {
		t.Error("an icon was returned while locked")
	}
}

func TestGetWebsiteIconSharesAFetchAndFollowsTheSetting(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	icon := encodeTestIconPNG(t, 32)
	fetcher := &fakeWebsiteIconFetcher{icons: map[string][]byte{"https://github.com/": icon}, isPaused: make(chan struct{})}
	service.config.WebsiteIconFetcher = fetcher

	var waiting sync.WaitGroup
	results := make([][]byte, 5)
	for index := range results {
		waiting.Add(1)
		go func() {
			defer waiting.Done()
			results[index], _ = service.GetWebsiteIcon("https://github.com/" + strings.Repeat("x", index))
		}()
	}
	for fetcher.calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	close(fetcher.isPaused)
	waiting.Wait()
	for index, result := range results {
		if !bytes.Equal(result, icon) {
			t.Errorf("caller %d got %d bytes", index, len(result))
		}
	}
	if calls := fetcher.calls.Load(); calls != 1 {
		t.Errorf("%d fetches for one website", calls)
	}

	if !service.IsWebsiteIconsEnabled() {
		t.Fatal("website icons are off by default")
	}
	if err := service.SetWebsiteIconsEnabled(false); err != nil {
		t.Fatal(err)
	}
	if got, _ := service.GetWebsiteIcon("https://github.com"); got != nil {
		t.Error("an icon was returned with website icons off")
	}
	if err := service.SetWebsiteIconsEnabled(true); err != nil {
		t.Fatal(err)
	}
	if got, _ := service.GetWebsiteIcon("https://github.com"); !bytes.Equal(got, icon) || fetcher.calls.Load() != 2 {
		t.Errorf("turning icons off kept the cache: %d fetches", fetcher.calls.Load())
	}
}

func TestPrepareItemIconRefusesOtherFiles(t *testing.T) {
	test := createUnlockedTestService(t)
	if _, err := test.service.PrepareItemIcon([]byte("<svg></svg>")); err == nil {
		t.Error("an SVG became an icon")
	}
	prepared, err := test.service.PrepareItemIcon(encodeTestIconPNG(t, 300))
	if err != nil || !item.IsIconImage(prepared) {
		t.Errorf("prepared %d bytes, %v", len(prepared), err)
	}
}

func TestCommandLineEditKeepsTheChosenIcon(t *testing.T) {
	test := createCommandLineTest(t)
	service := test.testService.service
	vaultID := test.testService.personalID
	upload, err := service.PrepareItemIcon(encodeTestIconPNG(t, 200))
	if err != nil {
		t.Fatal(err)
	}
	for _, icon := range []*ItemIcon{{Preset: "bank", Color: "green"}, {Image: upload}} {
		saved, err := service.SaveItem(vaultID, ItemDetail{Title: "Bank", Password: "bank-password", Icon: icon})
		if err != nil {
			t.Fatal(err)
		}
		var edited cliprotocol.ItemResult
		test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeSaveItem, VaultID: vaultID, Item: &cliprotocol.ItemChange{
			ID: saved.ID, Revision: saved.Revision, Title: "Bank", Password: "new-bank-password",
		}}, &edited)
		if stored := getCachedItem(t, service, vaultID, saved.ID); !reflect.DeepEqual(stored.Overview.Icon, convertFromItemIcon(icon)) ||
			stored.Details.Password != "new-bank-password" {
			t.Errorf("after hisar edit: icon %+v, password %q", stored.Overview.Icon, stored.Details.Password)
		}
	}
}
