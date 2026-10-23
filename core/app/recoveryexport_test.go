package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/cache"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/export"
)

func TestR08NeverSyncedArtifactCannotClaimEmptyRecovery(t *testing.T) {
	f := createUnlockedTestService(t)
	s := f.service
	c, err := cache.Open(filepath.Join(t.TempDir(), "copy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	s.unlockedSession.Cache = c
	if _, err := s.unlockedSession.ListVaults(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.unlockedSession.Client.BaseURL = "http://127.0.0.1:1"
	content, _, err := s.buildExport(ExportFormatPlain, "", s.sessionEpoch)
	if err != nil {
		t.Fatal(err)
	}
	var artifact struct {
		Vaults     []json.RawMessage `json:"vaults"`
		Provenance struct {
			Source    string            `json:"source"`
			Omissions []json.RawMessage `json:"omissions"`
		} `json:"provenance"`
	}
	if err := json.Unmarshal(content, &artifact); err != nil {
		t.Fatal(err)
	}
	if len(artifact.Vaults) != 0 || artifact.Provenance.Source != "cached" || len(artifact.Provenance.Omissions) != 2 {
		t.Fatalf("consumer artifact represents unsynced account as empty success: %s", content)
	}
}

func TestR08UnreadableNameStillExportsHealthySecret(t *testing.T) {
	f := createUnlockedTestService(t)
	s := f.service
	if _, err := s.SaveItem(f.personalID, ItemDetail{Title: "Only rescue copy", Password: "synthetic-rescue-password"}); err != nil {
		t.Fatal(err)
	}
	f.apiServer.mutex.Lock()
	for index := range f.apiServer.vaults {
		if f.apiServer.vaults[index].ID == f.personalID {
			f.apiServer.vaults[index].EncName = []byte("damaged-name")
		}
	}
	f.apiServer.mutex.Unlock()
	content, result, err := s.buildExport(ExportFormatPlain, "", s.sessionEpoch)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := export.Decode(content, nil)
	if err != nil {
		t.Fatal(err)
	}
	if doc.CountItem() != 1 || result.ItemCount != 1 {
		t.Fatalf("readable healthy secret omitted because only name is damaged: result=%+v artifact=%s", result, content)
	}
}

func TestR08OldRequesterCannotCreateUnwarnedArtifact(t *testing.T) {
	fixture := createCommandLineTest(t)
	path := filepath.Join(t.TempDir(), "legacy-export.json")
	failure := fixture.send(t, cliprotocol.Request{Type: cliprotocol.TypeExport, Export: &cliprotocol.ExportChange{Format: ExportFormatPlain, Path: path}}, nil)
	if !strings.Contains(failure, cliprotocol.ErrorUpdateRequired) {
		t.Fatalf("old requester silently exported: %q", failure)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("legacy refusal wrote artifact: %v", err)
	}
}
