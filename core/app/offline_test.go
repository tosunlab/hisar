package app

import (
	"bytes"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cache"
)

func TestOfflineShowsCachedVaultsAndRefusesChanges(t *testing.T) {
	testService := createUnlockedTestService(t)
	service := testService.service
	cachePath := filepath.Join(t.TempDir(), "desktop-cache.db")
	service.mutex.Lock()
	service.config.CachePath = cachePath
	service.unlockedSession.Cache = service.getCache()
	service.mutex.Unlock()

	saved, err := service.SaveItem(testService.familyID, ItemDetail{Title: "Netflix", Password: "hunter2"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ListVaults(); err != nil {
		t.Fatal(err)
	}
	if service.GetStatus().IsOffline {
		t.Fatal("offline while the server is reachable")
	}

	testService.apiServer.setUnreachable(true)
	vaults, err := service.ListVaults()
	if err != nil || len(vaults) != 2 {
		t.Fatalf("vaults offline: %+v %v", vaults, err)
	}
	if !service.GetStatus().IsOffline {
		t.Error("status does not say offline")
	}
	items, err := service.ListItems(testService.familyID)
	if err != nil || len(items) != 1 || items[0].Title != "Netflix" {
		t.Fatalf("items offline: %+v %v", items, err)
	}
	if detail, err := service.GetItem(testService.familyID, saved.ID); err != nil || detail.Password != "hunter2" {
		t.Errorf("item offline: %+v %v", detail, err)
	}
	if _, err := service.SaveItem(testService.familyID, ItemDetail{Title: "Bank"}); !errors.Is(err, ErrOffline) {
		t.Errorf("saving offline: %v", err)
	}

	testService.apiServer.setUnreachable(false)
	if _, err := service.ListVaults(); err != nil {
		t.Fatal(err)
	}
	if service.GetStatus().IsOffline {
		t.Error("still offline after the server came back")
	}

	if _, err := service.SignOut(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Errorf("cache kept after signing out: %v", err)
	}
}

func TestBrokenCacheFileIsStartedOver(t *testing.T) {
	testService := createUnlockedTestService(t)
	service := testService.service
	cachePath := filepath.Join(t.TempDir(), "desktop-cache.db")
	if err := os.WriteFile(cachePath, []byte("this is not a SQLite database, but it is long enough to have a header"), 0o600); err != nil {
		t.Fatal(err)
	}
	service.mutex.Lock()
	defer service.mutex.Unlock()
	service.config.CachePath = cachePath
	if service.getCache() == nil {
		t.Fatal("a broken cache file was not replaced")
	}
}

func TestCacheOpenPermissionErrorKeepsTheOfflineCopy(t *testing.T) {
	service, _, _ := createTestService(t, filepath.Join(t.TempDir(), "desktop.json"), createMemorySecretKeyStore())
	path := service.config.CachePath
	original := []byte("an offline snapshot that must not be discarded on an access error")
	if err := os.WriteFile(path, original, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0o600) })
	service.mutex.Lock()
	opened := service.getCache()
	service.mutex.Unlock()
	if opened != nil {
		t.Error("opened a cache whose access was refused")
	}
	kept, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(kept, original) {
		t.Errorf("permission error replaced the offline copy: %v", err)
	}
}

func TestCacheOpenBusyErrorKeepsTheOfflineCopy(t *testing.T) {
	service, _, _ := createTestService(t, filepath.Join(t.TempDir(), "desktop.json"), createMemorySecretKeyStore())
	service.state.AccountID = "ABC123"
	path := service.config.CachePath
	snapshot, err := cache.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshot.SaveAccount(&apitypes.Account{AccountID: "ABC123", Email: "kept@example.com"}); err != nil {
		t.Fatal(err)
	}
	if err := snapshot.Close(); err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec("BEGIN EXCLUSIVE"); err != nil {
		t.Fatal(err)
	}
	defer database.Exec("ROLLBACK")
	service.mutex.Lock()
	opened := service.getCache()
	service.mutex.Unlock()
	if opened != nil {
		t.Error("a temporary SQLite lock replaced the cache")
	}
	if _, err := database.Exec("ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	service.mutex.Lock()
	reopened := service.getCache()
	service.mutex.Unlock()
	if reopened == nil {
		t.Fatal("the cache was not usable after its lock ended")
	}
	account, err := reopened.LoadAccount("ABC123")
	if err != nil || account == nil || account.Email != "kept@example.com" {
		t.Errorf("the temporary lock discarded the cached account: %+v %v", account, err)
	}
}
