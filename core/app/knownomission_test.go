package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cache"
	"github.com/tryhisar/hisar/core/export"
)

func TestR08ObservedUnreadableVaultRemainsAKnownOfflineOmission(t *testing.T) {
	f := createUnlockedTestService(t)
	s := f.service
	path := filepath.Join(t.TempDir(), "cache.sqlite")
	c, err := cache.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.unlockedSession.Cache = c
	if _, err := s.ListVaults(); err != nil {
		t.Fatal(err)
	}
	f.apiServer.mutex.Lock()
	f.apiServer.vaults = append(f.apiServer.vaults, apitypes.Vault{ID: "observed-but-unreadable", Type: apitypes.VaultShared, Role: apitypes.VaultOwner, WrappedKey: []byte("bad"), EncName: []byte("bad")})
	f.apiServer.mutex.Unlock()
	online, err := s.unlockedSession.ReadVaults(context.Background())
	if err != nil || len(online.Issues) != 1 || online.Issues[0].VaultID != "observed-but-unreadable" {
		t.Fatalf("setup did not observe missing data: %+v %v", online, err)
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
	doc, err := export.Decode(content, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	if doc.Provenance != nil {
		for _, issue := range doc.Provenance.Omissions {
			if issue.VaultID == "observed-but-unreadable" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("known omitted vault vanished from offline artifact after cache reopen: result=%+v artifact=%s", result, content)
	}
}
