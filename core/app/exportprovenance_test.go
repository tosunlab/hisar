package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cache"
	"github.com/tryhisar/hisar/core/export"
)

func TestOfflineExportOmitsNeverSyncedVaultInsteadOfEmptySuccess(t *testing.T) {
	fixture := createUnlockedTestService(t)
	s := fixture.service
	c, err := cache.Open(filepath.Join(t.TempDir(), "copy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	s.unlockedSession.Cache = c
	if _, err := s.unlockedSession.ReadVaults(context.Background()); err != nil {
		t.Fatal(err)
	}
	originalURL := s.unlockedSession.Client.BaseURL
	s.unlockedSession.Client.BaseURL = "http://127.0.0.1:1"
	content, result, err := s.buildExport(ExportFormatPlain, "", s.sessionEpoch)
	if err != nil {
		t.Fatal(err)
	}
	document, err := export.Decode(content, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Provenance == nil || !result.Provenance.IsCachedCopy() || !result.Provenance.IsPartial() || result.VaultCount != 0 || len(document.Vaults) != 0 || len(document.Provenance.Omissions) != 2 || result.Warning == "" {
		t.Fatalf("never-synced vaults claimed complete: %+v %+v", result, document)
	}
	s.unlockedSession.Client.BaseURL = originalURL
	if _, err := s.ListVaults(); err != nil {
		t.Fatal(err)
	}
	_, retried, err := s.buildExport(ExportFormatPlain, "", s.sessionEpoch)
	if err != nil || retried.Provenance.IsCachedCopy() || retried.Provenance.IsPartial() || retried.VaultCount != 2 {
		t.Fatalf("online retry did not establish completed empty vaults: %+v %v", retried, err)
	}
}

func TestCompletedOfflineExportSurvivesCacheRestart(t *testing.T) {
	fixture := createUnlockedTestService(t)
	s := fixture.service
	path := filepath.Join(t.TempDir(), "copy.db")
	c, err := cache.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.unlockedSession.Cache = c
	if _, err := s.SaveItem(fixture.personalID, ItemDetail{Title: "Emergency exit", Password: "secret"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListVaults(); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	c, err = cache.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	s.unlockedSession.Cache = c
	s.unlockedSession.Client.BaseURL = "http://127.0.0.1:1"
	content, result, err := s.buildExport(ExportFormatPlain, "", s.sessionEpoch)
	if err != nil {
		t.Fatal(err)
	}
	document, err := export.Decode(content, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Provenance.IsCachedCopy() || result.Provenance.IsPartial() || result.VaultCount != 2 || result.ItemCount != 1 || document.CountItem() != 1 {
		t.Fatalf("completed rescue copy: %+v %+v", result, document)
	}
	for _, checkpoint := range document.Provenance.Vaults {
		if !checkpoint.HasCompletedSync || checkpoint.CompletedAt == "" {
			t.Fatalf("durable completion lost: %+v", checkpoint)
		}
	}
}

func TestExportReportsUnreadableItemsAndKeepsHealthyRecords(t *testing.T) {
	fixture := createUnlockedTestService(t)
	s := fixture.service
	if _, err := s.SaveItem(fixture.personalID, ItemDetail{Title: "Healthy", Password: "secret"}); err != nil {
		t.Fatal(err)
	}
	fixture.apiServer.mutex.Lock()
	fixture.apiServer.seq++
	fixture.apiServer.itemsByVaultID[fixture.personalID]["bad"] = apitypes.Item{ID: "bad", Revision: "bad-revision", Seq: fixture.apiServer.seq, Overview: []byte("bad"), Details: []byte("bad")}
	fixture.apiServer.mutex.Unlock()
	content, result, err := s.buildExport(ExportFormatPlain, "", s.sessionEpoch)
	if err != nil {
		t.Fatal(err)
	}
	document, err := export.Decode(content, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.ItemCount != 1 || !result.Provenance.IsPartial() || len(document.Provenance.Omissions) != 1 || document.Provenance.Omissions[0].ItemID != "bad" {
		t.Fatalf("partial export lost outcome: %+v %+v", result, document)
	}
}
