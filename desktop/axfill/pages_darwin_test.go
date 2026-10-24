//go:build darwin && !ios && cgo

package axfill

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/tryhisar/hisar/core/agentharness"
	"github.com/tryhisar/hisar/core/signin"
	"github.com/tryhisar/hisar/desktop/appinfo"
)

var update = flag.Bool("update", false, "write the recorded pages to core/signin/testdata/pages")

// recordingAddresses are fixed so the recorded pages' URLs do not change between recordings.
const recordingMainAddress, recordingOtherAddress = "127.0.0.1:47811", "127.0.0.1:47812"

var browserIDs = map[string]string{
	"com.brave.Browser":     "brave",
	"com.google.Chrome":     "chrome",
	"com.microsoft.edgemac": "edge",
}

type recordedPage struct {
	name      string
	path      string
	isOther   bool
	minFields int
	minFrames int
}

var recordedPages = []recordedPage{
	{name: "plain", path: "/plain", minFields: 2},
	{name: "controlled", path: "/controlled", minFields: 2},
	{name: "iframe", path: "/iframe", minFields: 2, minFrames: 2},
	{name: "srcdoc", path: "/srcdoc", minFields: 2, minFrames: 2},
	{name: "step1", path: "/step1", minFields: 1},
	{name: "step2", path: "/step2?username=user%40example.com", minFields: 1},
	{name: "spa-steps", path: "/spa-steps", minFields: 1},
	{name: "totp", path: "/totp", minFields: 2},
	{name: "totp-code", path: "/totp-code", minFields: 1},
	{name: "split-totp", path: "/split-totp", minFields: 8},
	{name: "spoof", path: "/spoof", minFields: 2},
	{name: "signup", path: "/signup", minFields: 3},
	{name: "newsletter", path: "/newsletter", minFields: 1},
	{name: "wrong", path: "/wrong", minFields: 2},
	{name: "invisible", path: "/invisible", minFields: 3},
	{name: "phish", path: "/phish", isOther: true, minFields: 2},
	{name: "return", path: "/return", minFields: 2},
	{name: "signed-in", path: "/signed-in"},
}

// TestIntegrationRecordPages reads every fixture page in each browser and compares it with
// core/signin/testdata/pages/<browser>/<page>.json; with -update it writes them instead.
func TestIntegrationRecordPages(t *testing.T) {
	apps := listBrowserApps(t)
	served := startSite(t, recordingMainAddress, recordingOtherAddress)
	driver := New(appinfo.NewSystemResolver())
	for _, app := range apps {
		launched := launchBrowser(t, app, served.Main+"/signed-in")
		waitForPage(t, driver, launched.target, served.Main+"/signed-in", func(signin.Page) bool { return true })
		var browserID string
		for _, page := range recordedPages {
			pageURL := served.Main + page.path
			if page.isOther {
				pageURL = served.Other + page.path
			}
			served.steer(t, pageURL)
			recorded := waitForPage(t, driver, launched.target, pageURL, func(read signin.Page) bool {
				return len(read.Fields) >= page.minFields && len(read.Frames) >= max(page.minFrames, 1)
			})
			browserID = browserIDs[recorded.Process.Signature.Identifier]
			if browserID == "" {
				t.Fatalf("%s is signed as %+v, which is not a known browser", app, recorded.Process.Signature)
			}
			encoded, err := json.MarshalIndent(normalizePage(recorded), "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join("..", "..", "core", "signin", "testdata", "pages", browserID, page.name+".json")
			if *update {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
					t.Fatal(err)
				}
				continue
			}
			stored, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(bytes.TrimSpace(stored), encoded) {
				t.Errorf("%s in %s differs from %s (%v); rerun with -update to record it:\n%s", page.name, browserID, path, err, encoded)
			}
		}
		t.Logf("recorded %d pages in %s", len(recordedPages), browserID)
	}
}

// normalizePage keeps what core/signin reads from a page and numbers its documents and elements from 1, so a recording
// changes only when the browser shows the page differently.
func normalizePage(page signin.Page) signin.Page {
	normalized := signin.Page{Process: agentharness.Process{Signature: page.Process.Signature}}
	next := uint64(0)
	number := func() uint64 {
		next++
		return next
	}
	for _, frame := range page.Frames {
		frame.Document = signin.DocumentID(number())
		normalized.Frames = append(normalized.Frames, frame)
	}
	for _, field := range page.Fields {
		field.ID = signin.ElementID(number())
		normalized.Fields = append(normalized.Fields, field)
	}
	for _, button := range page.Buttons {
		button.ID = signin.ElementID(number())
		normalized.Buttons = append(normalized.Buttons, button)
	}
	return normalized
}
