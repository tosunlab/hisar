package app

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cache"
	"github.com/tryhisar/hisar/core/export"
)

func TestR08MixedProvenanceSurvivesActualPlainAndEncryptedArtifacts(t *testing.T) {
	f := createUnlockedTestService(t)
	s := f.service
	c, err := cache.Open(filepath.Join(t.TempDir(), "cache.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	s.unlockedSession.Cache = c
	if _, err := s.SaveItem(f.personalID, ItemDetail{Title: "Healthy cached secret", Password: "synthetic-mixed-secret"}); err != nil {
		t.Fatal(err)
	}
	f.apiServer.mutex.Lock()
	f.apiServer.seq++
	f.apiServer.itemsByVaultID[f.personalID]["bad"] = apitypes.Item{ID: "bad", Revision: "mixed-bad-revision", Seq: f.apiServer.seq, Overview: []byte("bad"), Details: []byte("bad")}
	f.apiServer.mutex.Unlock()
	if _, err := s.ListVaults(); err != nil {
		t.Fatal(err)
	}
	s.unlockedSession.Client.HTTP.Transport = r08AppTransport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/changes") {
			return &http.Response{StatusCode: 502, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"changes unavailable"}`))}, nil
		}
		return http.DefaultTransport.RoundTrip(r)
	})
	for _, format := range []string{ExportFormatPlain, ExportFormatEncrypted} {
		content, result, err := s.buildExport(format, "synthetic-export-passphrase", s.sessionEpoch)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := export.Decode(content, []byte("synthetic-export-passphrase"))
		if err != nil || doc.Provenance == nil || doc.Provenance.Source != "mixed" || !doc.Provenance.IsPartial() || doc.CountItem() != 1 || doc.Vaults[0].Items[0].Details.Password != "synthetic-mixed-secret" || result.Provenance.Source != "mixed" {
			t.Fatalf("mixed source or healthy record lost from actual artifact: %+v %v", doc, err)
		}
	}
}
