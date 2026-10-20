package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/cache"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/itemicon"
	"github.com/tryhisar/hisar/core/session"
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
	service := createLockedUnlockableService(t).service
	service.config.IsEventStreamDisabled = true
	if _, err := service.Unlock(testMasterPassword); err != nil {
		t.Fatal(err)
	}
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

// A session whose cache failed to attach must not write icons into that cache: a cache with icons and no account is
// refused at every later launch, so offline unlock would never work again.
func TestWebsiteIconsStayOutOfACacheTheSessionDidNotAttach(t *testing.T) {
	fixture := createLockedUnlockableService(t)
	s := fixture.service
	s.config.IsEventStreamDisabled = true
	icon := encodeTestIconPNG(t, 32)
	s.config.WebsiteIconFetcher = &fakeWebsiteIconFetcher{icons: map[string][]byte{"https://github.com/": icon}}
	client := s.createClient(s.state.Server)
	client.Token = s.state.Token
	secretKey, err := s.config.SecretKeyStore.LoadSecretKey(fixture.accountID)
	if err != nil {
		t.Fatal(err)
	}
	unlocked, err := session.Unlock(context.Background(), client, nil, []byte(testMasterPassword), secretKey)
	if err != nil {
		t.Fatal(err)
	}
	client.HTTP = &http.Client{Transport: failedAccountReadTransport{}}
	s.mutex.Lock()
	s.attachCache(context.Background(), unlocked)
	s.openSession(unlocked)
	isAttached := s.unlockedSession.Cache != nil
	s.mutex.Unlock()
	if isAttached {
		t.Fatal("fixture: the cache was attached although the account read failed")
	}
	if got, err := s.GetWebsiteIcon("https://github.com"); err != nil || !bytes.Equal(got, icon) {
		t.Errorf("GetWebsiteIcon without an attached cache = %d bytes, %v", len(got), err)
	}
	s.Lock()
	s.mutex.Lock()
	if s.localCache != nil {
		s.localCache.Close()
		s.localCache = nil
	}
	s.mutex.Unlock()
	reopened, err := cache.Open(s.config.CachePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.CheckAccount(fixture.accountID); err != nil {
		t.Errorf("the icon left a cache later launches refuse: %v", err)
	}
	reopened.Close()
	if _, err := s.Unlock(testMasterPassword); err != nil {
		t.Fatal(err)
	}
	s.mutex.Lock()
	isAttached = s.unlockedSession.Cache != nil
	s.mutex.Unlock()
	if !isAttached {
		t.Error("the next unlock could not attach the offline copy")
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

type blockingWebsiteIconFetcher struct {
	started chan struct{}
	ended   chan error
}

func (fetcher *blockingWebsiteIconFetcher) Fetch(ctx context.Context, _ string) ([]byte, error) {
	close(fetcher.started)
	select {
	case <-ctx.Done():
		fetcher.ended <- ctx.Err()
	case <-time.After(5 * time.Second):
		fetcher.ended <- errors.New("the fetch was not canceled")
	}
	return nil, context.Canceled
}

func TestGetWebsiteIconCancelsAFetchOnLockAndWhenTurnedOff(t *testing.T) {
	for name, stop := range map[string]func(*Service){
		"lock":     func(service *Service) { service.Lock() },
		"turn off": func(service *Service) { service.SetWebsiteIconsEnabled(false) },
	} {
		t.Run(name, func(t *testing.T) {
			service := createUnlockedTestService(t).service
			fetcher := &blockingWebsiteIconFetcher{started: make(chan struct{}), ended: make(chan error, 1)}
			service.config.WebsiteIconFetcher = fetcher
			go service.GetWebsiteIcon("https://github.com")
			<-fetcher.started
			stop(service)
			if err := <-fetcher.ended; !errors.Is(err, context.Canceled) {
				t.Errorf("fetch after %s: %v", name, err)
			}
		})
	}
}

func TestGetWebsiteIconDoesNotFetchAfterItsAuthorizationIsRevoked(t *testing.T) {
	for _, reenable := range []bool{false, true} {
		t.Run(fmt.Sprintf("reenable=%v", reenable), func(t *testing.T) {
			service := createUnlockedTestService(t).service
			fetcher := &fakeWebsiteIconFetcher{}
			service.config.WebsiteIconFetcher = fetcher
			authorized, resume := make(chan struct{}), make(chan struct{})
			resumeClock := sync.OnceFunc(func() { close(resume) })
			defer resumeClock()
			var once sync.Once
			service.config.Now = func() time.Time {
				once.Do(func() {
					close(authorized)
					<-resume
				})
				return time.Now()
			}
			done := make(chan []byte, 1)
			go func() {
				image, _ := service.GetWebsiteIcon("https://github.com")
				done <- image
			}()
			select {
			case <-authorized:
			case <-time.After(5 * time.Second):
				t.Fatal("website icon request did not reach the authorization barrier")
			}
			settingsChanged := make(chan error, 1)
			go func() {
				err := service.SetWebsiteIconsEnabled(false)
				if err == nil && reenable {
					err = service.SetWebsiteIconsEnabled(true)
				}
				settingsChanged <- err
			}()
			select {
			case err := <-settingsChanged:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("changing website icon settings blocked while the authorized request was paused")
			}
			resumeClock()
			select {
			case image := <-done:
				if image != nil {
					t.Errorf("revoked request returned %d bytes", len(image))
				}
			case <-time.After(5 * time.Second):
				t.Fatal("revoked website icon request did not finish after resuming")
			}
			if calls := fetcher.calls.Load(); calls != 0 {
				t.Errorf("revoked request made %d fetches", calls)
			}
		})
	}
}

func TestGetWebsiteIconDoesNotCacheOrJoinARevokedFetch(t *testing.T) {
	service := createUnlockedTestService(t).service
	icon := encodeTestIconPNG(t, 32)
	fetcher := &fakeWebsiteIconFetcher{icons: map[string][]byte{"https://github.com/": icon}, isPaused: make(chan struct{})}
	service.config.WebsiteIconFetcher = fetcher
	done := make(chan []byte, 1)
	go func() {
		image, _ := service.GetWebsiteIcon("https://github.com")
		done <- image
	}()
	for fetcher.calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	if err := service.SetWebsiteIconsEnabled(false); err != nil {
		t.Fatal(err)
	}
	if err := service.SetWebsiteIconsEnabled(true); err != nil {
		t.Fatal(err)
	}
	current := make(chan []byte, 1)
	go func() {
		image, _ := service.GetWebsiteIcon("https://github.com")
		current <- image
	}()
	deadline := time.After(5 * time.Second)
	for fetcher.calls.Load() < 2 {
		select {
		case <-deadline:
			close(fetcher.isPaused)
			t.Fatal("new request joined the revoked fetch")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	close(fetcher.isPaused)
	if image := <-done; image != nil {
		t.Errorf("revoked fetch returned %d bytes", len(image))
	}
	if image := <-current; !bytes.Equal(image, icon) {
		t.Errorf("current fetch returned %d bytes", len(image))
	}
	if image, err := service.GetWebsiteIcon("https://github.com"); err != nil || !bytes.Equal(image, icon) || fetcher.calls.Load() != 2 {
		t.Errorf("current cache returned %d bytes, %v, after %d fetches", len(image), err, fetcher.calls.Load())
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
