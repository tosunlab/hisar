package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
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
	if _, err := service.ListAllVault(); err != nil {
		t.Fatal(err)
	}
	if service.GetStatus().IsOffline {
		t.Fatal("offline while the server is reachable")
	}

	testService.apiServer.setUnreachable(true)
	allVault, err := service.ListAllVault()
	if err != nil || len(allVault) != 2 {
		t.Fatalf("vaults offline: %+v %v", allVault, err)
	}
	if !service.GetStatus().IsOffline {
		t.Error("status does not say offline")
	}
	allItem, err := service.ListAllItem(testService.familyID)
	if err != nil || len(allItem) != 1 || allItem[0].Title != "Netflix" {
		t.Fatalf("items offline: %+v %v", allItem, err)
	}
	if detail, err := service.GetItem(testService.familyID, saved.ID); err != nil || detail.Password != "hunter2" {
		t.Errorf("item offline: %+v %v", detail, err)
	}
	if _, err := service.SaveItem(testService.familyID, ItemDetail{Title: "Bank"}); !errors.Is(err, ErrOffline) {
		t.Errorf("saving offline: %v", err)
	}

	testService.apiServer.setUnreachable(false)
	if _, err := service.ListAllVault(); err != nil {
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
