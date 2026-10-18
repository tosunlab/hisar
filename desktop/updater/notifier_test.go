package updater

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

const testAppcast = `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle">
<channel>
<title>Hisar for macOS</title>
<item><title>Hisar 0.4.0</title><sparkle:version>4000</sparkle:version><sparkle:shortVersionString>0.4.0</sparkle:shortVersionString></item>
<item><title>Hisar 0.10.1</title><sparkle:version>10001</sparkle:version><sparkle:shortVersionString>0.10.1</sparkle:shortVersionString></item>
<item><title>Hisar 0.3.0</title><sparkle:version>3000</sparkle:version><sparkle:shortVersionString>0.3.0</sparkle:shortVersionString></item>
</channel>
</rss>
`

func TestFindNewerVersionComparesNumerically(t *testing.T) {
	if newest, err := findNewerVersion([]byte(testAppcast), "0.4.0"); err != nil || newest != "0.10.1" {
		t.Fatalf("findNewerVersion = %q, %v", newest, err)
	}
	if newest, err := findNewerVersion([]byte(testAppcast), "0.10.1"); err != nil || newest != "" {
		t.Fatalf("findNewerVersion on the newest = %q, %v", newest, err)
	}
	// A custom build 0.10.1-test is older than the release 0.10.1 it leads up to.
	if newest, _ := findNewerVersion([]byte(testAppcast), "0.10.1-test"); newest != "0.10.1" {
		t.Fatalf("findNewerVersion on a custom build = %q", newest)
	}
	if _, err := findNewerVersion([]byte(testAppcast), "development"); err == nil {
		t.Fatal("a development build compared versions")
	}
}

func TestNotifierReadsTheSignedInFeed(t *testing.T) {
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		authorization = request.Header.Get("Authorization")
		writer.Write([]byte(testAppcast))
	}))
	defer server.Close()
	notifier := NewNotifier("0.3.0")
	if !notifier.IsAvailable() {
		t.Fatal("a release build is not available")
	}
	if err := notifier.CheckForUpdates(); err == nil {
		t.Fatal("checked while signed out")
	}
	notifier.SetSession(server.URL+"/api/v1/desktop/updates/appcast.xml", "Bearer token")
	if err := notifier.CheckForUpdates(); err != nil {
		t.Fatal(err)
	}
	available := notifier.GetAvailableUpdate()
	if authorization != "Bearer token" || !available.HasChecked || available.Version != "0.10.1" || available.DownloadURL != "https://github.com/tosunlab/hisar/releases/tag/desktop-0.10.1" {
		t.Fatalf("available = %+v, authorization %q", available, authorization)
	}
	notifier.SetAutomaticCheck(true)
	notifier.SetAutomaticCheck(false)
}

func TestNotifierInADevelopmentBuild(t *testing.T) {
	notifier := NewNotifier("")
	if notifier.IsAvailable() {
		t.Error("a development build is available")
	}
	if version, _ := notifier.GetVersion(); version != developmentVersion {
		t.Errorf("version = %q", version)
	}
}
