package desktopupdate_test

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tryhisar/hisar/server/internal/desktopupdate"
	"github.com/tryhisar/hisar/server/internal/desktopupdate/fakegithub"
)

const testToken = "github_pat_test"

type testClock struct {
	mutex sync.Mutex
	now   time.Time
}

func (clock *testClock) getNow() time.Time {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	return clock.now
}

func (clock *testClock) advance(duration time.Duration) {
	clock.mutex.Lock()
	clock.now = clock.now.Add(duration)
	clock.mutex.Unlock()
}

func createRelease(version, releaseNotes string, isUnpublished bool) fakegithub.Release {
	buildNumber, err := desktopupdate.GetBuildNumber(version)
	if err != nil {
		panic(err)
	}
	return fakegithub.NewDesktopRelease(fakegithub.ReleaseConfig{
		Version: version, BuildNumber: buildNumber, Archive: []byte("zip of " + version), ReleaseNotes: releaseNotes, IsUnpublished: isUnpublished,
		PublishedAt: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC).Add(time.Duration(buildNumber) * time.Second),
	})
}

func addRelease(fake *fakegithub.Server, version, releaseNotes string) fakegithub.Release {
	release := createRelease(version, releaseNotes, false)
	fake.AddRelease(release)
	return release
}

func startFake(t *testing.T) *fakegithub.Server {
	return fakegithub.Start(t, fakegithub.DefaultRepository, testToken)
}

func createSource(t *testing.T, apiURL, token string, clock *testClock) *desktopupdate.Source {
	t.Helper()
	gitHub, err := desktopupdate.NewGitHub(desktopupdate.GitHubConfig{APIURL: apiURL + "/", Repository: fakegithub.DefaultRepository, Token: token})
	if err != nil {
		t.Fatal(err)
	}
	return desktopupdate.NewSource(gitHub, desktopupdate.SourceConfig{PublicURL: "https://hisar.test/", Now: clock.getNow})
}

type appcastDocument struct {
	XMLName xml.Name `xml:"rss"`
	Items   []struct {
		Title                string `xml:"title"`
		PublishedAt          string `xml:"pubDate"`
		Version              string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle version"`
		ShortVersion         string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle shortVersionString"`
		MinimumSystemVersion string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle minimumSystemVersion"`
		Description          *struct {
			Format string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle format,attr"`
			Text   string `xml:",chardata"`
		} `xml:"description"`
		Enclosure struct {
			URL         string `xml:"url,attr"`
			Length      string `xml:"length,attr"`
			EdSignature string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle edSignature,attr"`
		} `xml:"enclosure"`
	} `xml:"channel>item"`
}

func parseAppcast(t *testing.T, appcast []byte) appcastDocument {
	t.Helper()
	var document appcastDocument
	if err := xml.Unmarshal(appcast, &document); err != nil {
		t.Fatalf("appcast is not valid XML: %v\n%s", err, appcast)
	}
	return document
}

func listShortVersions(t *testing.T, appcast []byte) string {
	t.Helper()
	shortVersions := []string{}
	for _, item := range parseAppcast(t, appcast).Items {
		shortVersions = append(shortVersions, item.ShortVersion)
	}
	return strings.Join(shortVersions, " ")
}

func countAPIRequest(fake *fakegithub.Server, test func(fakegithub.Request) bool) int {
	count := 0
	for _, request := range fake.GetRequests() {
		if test(request) {
			count++
		}
	}
	return count
}

func isAssetRequest(request fakegithub.Request) bool {
	return strings.Contains(request.Path, "/releases/assets/")
}

func TestGetBuildNumber(t *testing.T) {
	expectedValues := map[string]int64{"0.0.1": 1, "0.1.0": 1000, "0.2.1": 2001, "1.0.0": 1000000, "12.345.999": 12345999}
	for version, expected := range expectedValues {
		if buildNumber, err := desktopupdate.GetBuildNumber(version); err != nil || buildNumber != expected {
			t.Errorf("%s: %d %v", version, buildNumber, err)
		}
	}
	for _, version := range []string{"", "1", "1.2", "1.2.3.4", "01.2.3", "1.2.1000", "1.2.3-beta", "v1.2.3", "1.2.-3", " 1.2.3"} {
		if _, err := desktopupdate.GetBuildNumber(version); err == nil {
			t.Errorf("%q accepted", version)
		}
	}
}

func TestPackagingScriptUsesSameBuildNumber(t *testing.T) {
	for _, version := range []string{"0.0.1", "0.1.0", "0.2.1", "1.0.0", "12.345.999", "1.2", "01.2.3", "1.2.1000"} {
		output, scriptErr := exec.Command("bash", "../../../scripts/get-build-number.sh", version).Output()
		buildNumber, err := desktopupdate.GetBuildNumber(version)
		if (scriptErr == nil) != (err == nil) || (err == nil && strings.TrimSpace(string(output)) != fmt.Sprint(buildNumber)) {
			t.Errorf("%s: script %q %v, server %d %v", version, output, scriptErr, buildNumber, err)
		}
	}
}

func TestIsValidFileName(t *testing.T) {
	for _, fileName := range []string{"Hisar-0.2.0.zip", "update.json", "Hisar_1.dmg"} {
		if !desktopupdate.IsValidFileName(fileName) {
			t.Errorf("%q refused", fileName)
		}
	}
	for _, fileName := range []string{"", ".", "..", "../update.json", "a/b", `a\b`, ".hidden", "x..zip", "%2e%2e", "a b", "a\x00", strings.Repeat("a", 129)} {
		if desktopupdate.IsValidFileName(fileName) {
			t.Errorf("%q accepted", fileName)
		}
	}
}

func TestCreateGitHubValidatesConfig(t *testing.T) {
	configs := []desktopupdate.GitHubConfig{
		{APIURL: "api.github.com", Repository: fakegithub.DefaultRepository},
		{APIURL: "ftp://api.github.com", Repository: fakegithub.DefaultRepository},
		{Repository: "tosunlab"},
		{Repository: fakegithub.DefaultRepository + "/releases"},
		{Repository: "tosunlab/.."},
		{Repository: "../core"},
		{Repository: fakegithub.DefaultRepository + "?x=1"},
		{Repository: ""},
	}
	for _, config := range configs {
		if _, err := desktopupdate.NewGitHub(config); err == nil {
			t.Errorf("%+v accepted", config)
		}
	}
	for _, config := range []desktopupdate.GitHubConfig{{Repository: fakegithub.DefaultRepository}, {APIURL: "https://github.example.com/api/v3/", Repository: " Owner-1/repo.name_2 ", Token: "t"}} {
		if _, err := desktopupdate.NewGitHub(config); err != nil {
			t.Errorf("%+v refused: %v", config, err)
		}
	}
}

func TestBuildAppcastKeepsItsFormat(t *testing.T) {
	release := &desktopupdate.Release{
		Version: "0.2.1", BuildNumber: 2001, Archive: "Hisar-0.2.1.zip", EdSignature: "c2lnbmF0dXJl", Length: 1234, MinimumSystemVersion: "12.0",
		ReleaseNotes: "Fixes <b> & \"quotes\"", ReleaseNotesFormat: "markdown", PublishedAt: time.Date(2026, 10, 2, 9, 30, 0, 0, time.FixedZone("CEST", 2*3600)),
	}
	withoutNotes := *release
	withoutNotes.Version, withoutNotes.BuildNumber, withoutNotes.Archive, withoutNotes.ReleaseNotes = "0.2.0", 2000, "Hisar-0.2.0.zip", ""
	expected := `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle">
<channel>
<title>Hisar for macOS</title>
<item>
<title>Hisar 0.2.1</title>
<pubDate>Fri, 02 Oct 2026 07:30:00 +0000</pubDate>
<sparkle:version>2001</sparkle:version>
<sparkle:shortVersionString>0.2.1</sparkle:shortVersionString>
<sparkle:minimumSystemVersion>12.0</sparkle:minimumSystemVersion>
<description sparkle:format="markdown">Fixes &lt;b&gt; &amp; &#34;quotes&#34;</description>
<enclosure url="https://hisar.test/api/v1/desktop/updates/0.2.1/Hisar-0.2.1.zip" length="1234" type="application/octet-stream" sparkle:edSignature="c2lnbmF0dXJl"/>
</item>
<item>
<title>Hisar 0.2.0</title>
<pubDate>Fri, 02 Oct 2026 07:30:00 +0000</pubDate>
<sparkle:version>2000</sparkle:version>
<sparkle:shortVersionString>0.2.0</sparkle:shortVersionString>
<sparkle:minimumSystemVersion>12.0</sparkle:minimumSystemVersion>
<enclosure url="https://hisar.test/api/v1/desktop/updates/0.2.0/Hisar-0.2.0.zip" length="1234" type="application/octet-stream" sparkle:edSignature="c2lnbmF0dXJl"/>
</item>
</channel>
</rss>
`
	if appcast := desktopupdate.BuildAppcast("https://hisar.test/", []*desktopupdate.Release{release, &withoutNotes}); string(appcast) != expected {
		t.Errorf("appcast:\n%s", appcast)
	}
}

func TestAppcastListsNewestPublishedDesktopReleases(t *testing.T) {
	fake := startFake(t)
	for _, version := range []string{"0.1.0", "0.2.0", "0.3.0", "0.9.0", "0.10.0", "1.0.0"} {
		addRelease(fake, version, "")
	}
	addRelease(fake, "0.11.0", "Fixes <b>bold</b> & \"quotes\"\n\n- second line ]]>")
	fake.AddRelease(createRelease("1.1.0", "", true))
	publishedPrerelease := createRelease("1.2.0", "", false)
	publishedPrerelease.IsPrerelease = true
	fake.AddRelease(publishedPrerelease)
	labelled := createRelease("1.3.0", "", false)
	labelled.TagName = "desktop-1.3.0-beta"
	fake.AddRelease(labelled)
	draft := createRelease("1.4.0", "", false)
	draft.IsDraft = true
	fake.AddRelease(draft)
	notDesktop := createRelease("1.5.0", "", false)
	notDesktop.TagName = "cli-1.5.0"
	fake.AddRelease(notDesktop)
	withoutVersion := createRelease("1.6.0", "", false)
	withoutVersion.TagName = "desktop-latest"
	fake.AddRelease(withoutVersion)
	wrongBuildNumber := fakegithub.NewDesktopRelease(fakegithub.ReleaseConfig{Version: "2.0.0", BuildNumber: 1, Archive: []byte("x"), PublishedAt: time.Now()})
	fake.AddRelease(wrongBuildNumber)
	// The other apps' releases push the desktop ones onto the second page.
	for index := range 120 {
		fake.AddRelease(fakegithub.Release{TagName: fmt.Sprintf("cli-0.0.%d", index), Assets: []fakegithub.Asset{{Name: "SHA256SUMS", Content: []byte("sums")}}})
	}

	appcast, err := createSource(t, fake.URL, testToken, &testClock{now: time.Now()}).GetAppcast(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if shortVersions := listShortVersions(t, appcast); shortVersions != "1.0.0 0.11.0 0.10.0 0.9.0 0.3.0" {
		t.Fatalf("items: %v\n%s", shortVersions, appcast)
	}
	document := parseAppcast(t, appcast)
	newest, withNotes := document.Items[0], document.Items[1]
	if newest.Version != "1000000" || newest.Title != "Hisar 1.0.0" || newest.MinimumSystemVersion != "12.0" || newest.Description != nil {
		t.Errorf("newest item: %+v", newest)
	}
	if newest.Enclosure.URL != "https://hisar.test/api/v1/desktop/updates/1.0.0/Hisar-1.0.0.zip" || newest.Enclosure.Length != fmt.Sprint(len("zip of 1.0.0")) {
		t.Errorf("enclosure: %+v", newest.Enclosure)
	}
	if len(newest.Enclosure.EdSignature) != 88 {
		t.Errorf("signature: %q", newest.Enclosure.EdSignature)
	}
	if _, err := time.Parse(time.RFC1123Z, newest.PublishedAt); err != nil {
		t.Errorf("pubDate %q: %v", newest.PublishedAt, err)
	}
	if withNotes.Description == nil || withNotes.Description.Format != "markdown" || withNotes.Description.Text != "Fixes <b>bold</b> & \"quotes\"\n\n- second line ]]>" {
		t.Errorf("release notes: %+v", withNotes.Description)
	}
	if bytes.Contains(appcast, []byte("<b>")) {
		t.Error("release notes not escaped")
	}
	if pageCount := countAPIRequest(fake, func(request fakegithub.Request) bool { return strings.Contains(request.Path, "/releases?") }); pageCount != 2 {
		t.Errorf("listed %d pages", pageCount)
	}
	// 2.0.0 (refused) and the five in the appcast; never the older 0.2.0 and 0.1.0 or the skipped releases.
	if assetCount := countAPIRequest(fake, isAssetRequest); assetCount != 6 {
		t.Errorf("read %d update.json files: %+v", assetCount, fake.GetRequests())
	}
}

func TestPublishingOffersTheRelease(t *testing.T) {
	fake := startFake(t)
	addRelease(fake, "0.2.0", "")
	fake.AddRelease(createRelease("0.3.0", "", true))
	clock := &testClock{now: time.Now()}
	source := createSource(t, fake.URL, testToken, clock)
	if appcast, _ := source.GetAppcast(context.Background()); listShortVersions(t, appcast) != "0.2.0" {
		t.Fatalf("unpublished release offered:\n%s", appcast)
	}
	if _, _, err := source.OpenFile(context.Background(), "0.3.0", "Hisar-0.3.0.zip"); !errors.Is(err, desktopupdate.ErrFileNotFound) {
		t.Errorf("unpublished file served: %v", err)
	}
	fake.PublishRelease("desktop-0.3.0")
	clock.advance(11 * time.Minute)
	if appcast, _ := source.GetAppcast(context.Background()); listShortVersions(t, appcast) != "0.3.0 0.2.0" {
		t.Errorf("published release not offered:\n%s", appcast)
	}
}

func TestAppcastIsCachedForTenMinutes(t *testing.T) {
	fake := startFake(t)
	addRelease(fake, "0.2.0", "")
	clock := &testClock{now: time.Now()}
	source := createSource(t, fake.URL, testToken, clock)
	if _, err := source.GetAppcast(context.Background()); err != nil {
		t.Fatal(err)
	}
	requestCount := len(fake.GetRequests())
	clock.advance(9 * time.Minute)
	source.GetAppcast(context.Background())
	if len(fake.GetRequests()) != requestCount {
		t.Error("appcast refreshed before ten minutes")
	}

	clock.advance(2 * time.Minute)
	if appcast, _ := source.GetAppcast(context.Background()); listShortVersions(t, appcast) != "0.2.0" {
		t.Errorf("appcast after an unchanged refresh:\n%s", appcast)
	}
	refreshRequests := fake.GetRequests()[requestCount:]
	if len(refreshRequests) != 1 || refreshRequests[0].IfNoneMatch == "" || refreshRequests[0].Status != http.StatusNotModified {
		t.Errorf("unchanged refresh: %+v", refreshRequests)
	}

	addRelease(fake, "0.3.0", "")
	clock.advance(11 * time.Minute)
	if appcast, _ := source.GetAppcast(context.Background()); listShortVersions(t, appcast) != "0.3.0 0.2.0" {
		t.Errorf("appcast not refreshed after ten minutes:\n%s", appcast)
	}
	if assetCount := countAPIRequest(fake, isAssetRequest); assetCount != 2 {
		t.Errorf("update.json read %d times for two releases", assetCount)
	}
}

func TestAppcastSurvivesGitHubOutage(t *testing.T) {
	fake := startFake(t)
	addRelease(fake, "0.2.0", "")
	clock := &testClock{now: time.Now()}
	source := createSource(t, fake.URL, testToken, clock)
	if _, err := source.GetAppcast(context.Background()); err != nil {
		t.Fatal(err)
	}
	fake.Token = "rotated"
	clock.advance(11 * time.Minute)
	appcast, err := source.GetAppcast(context.Background())
	if err != nil || !bytes.Contains(appcast, []byte("0.2.0")) {
		t.Errorf("cached appcast not served during an outage: %v", err)
	}
	requestCount := len(fake.GetRequests())
	source.GetAppcast(context.Background())
	if len(fake.GetRequests()) != requestCount {
		t.Error("retried GitHub right after a failure")
	}
	clock.advance(time.Minute)
	source.GetAppcast(context.Background())
	if len(fake.GetRequests()) == requestCount {
		t.Error("did not retry GitHub a minute after a failure")
	}

	_, err = createSource(t, fake.URL, "", clock).GetAppcast(context.Background())
	if err == nil || !strings.Contains(err.Error(), "no repository "+fakegithub.DefaultRepository+" (a private repository needs a token)") {
		t.Errorf("private repository without a token: %v", err)
	}
}

func TestAppcastWaitsOutTheRateLimit(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusTooManyRequests} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			fake := startFake(t)
			addRelease(fake, "0.2.0", "")
			clock := &testClock{now: time.Now()}
			source := createSource(t, fake.URL, testToken, clock)
			if _, err := source.GetAppcast(context.Background()); err != nil {
				t.Fatal(err)
			}
			fake.LimitRate(status, 600)
			clock.advance(11 * time.Minute)
			appcast, err := source.GetAppcast(context.Background())
			if err != nil || listShortVersions(t, appcast) != "0.2.0" {
				t.Fatalf("cached appcast not served while rate limited: %v", err)
			}
			requestCount := len(fake.GetRequests())
			clock.advance(5 * time.Minute)
			source.GetAppcast(context.Background())
			if len(fake.GetRequests()) != requestCount {
				t.Error("asked GitHub again before the rate limit reset")
			}

			uncached := createSource(t, fake.URL, testToken, clock)
			if _, err := uncached.GetAppcast(context.Background()); err == nil || !strings.Contains(err.Error(), "rate limit") {
				t.Errorf("no cached appcast: %v", err)
			}
			requestCount = len(fake.GetRequests())
			if _, err := uncached.GetAppcast(context.Background()); err == nil || len(fake.GetRequests()) != requestCount {
				t.Errorf("asked GitHub again without a cached appcast: %v", err)
			}

			fake.LimitRate(0, 0)
			clock.advance(6 * time.Minute)
			addRelease(fake, "0.3.0", "")
			if appcast, _ := source.GetAppcast(context.Background()); listShortVersions(t, appcast) != "0.3.0 0.2.0" {
				t.Errorf("appcast not refreshed after the rate limit reset:\n%s", appcast)
			}
		})
	}
}

func TestOpenFileStreamsTheArchiveThroughTheAPI(t *testing.T) {
	fake := startFake(t)
	addRelease(fake, "0.2.0", "")
	firstHalf, secondHalf := bytes.Repeat([]byte("a"), 64<<10), bytes.Repeat([]byte("b"), 64<<10)
	releaseSecondHalf := make(chan struct{})
	fake.BeforeDownload = func(writer http.ResponseWriter, request *http.Request, assetName string) bool {
		if assetName != "Hisar-0.2.0.zip" {
			return true
		}
		writer.Header().Set("Content-Length", fmt.Sprint(len(firstHalf)+len(secondHalf)))
		writer.Write(firstHalf)
		writer.(http.Flusher).Flush()
		<-releaseSecondHalf
		writer.Write(secondHalf)
		return false
	}
	source := createSource(t, fake.URL, testToken, &testClock{now: time.Now()})

	reader, size, err := source.OpenFile(context.Background(), "0.2.0", "Hisar-0.2.0.zip")
	if err != nil {
		t.Fatal(err)
	}
	if size != int64(len(firstHalf)+len(secondHalf)) {
		t.Errorf("size = %d", size)
	}
	close(releaseSecondHalf)
	content, _ := io.ReadAll(reader)
	reader.Close()
	if !bytes.Equal(content, append(firstHalf, secondHalf...)) {
		t.Errorf("streamed %d bytes", len(content))
	}

	for _, request := range fake.GetRequests() {
		if request.Authorization != "Bearer "+testToken {
			t.Errorf("%s sent without the token: %q", request.Path, request.Authorization)
		}
		if isAssetRequest(request) && request.Status != http.StatusFound {
			t.Errorf("%s answered %d", request.Path, request.Status)
		}
	}
	storageAuthorizations := fake.GetStorageAuthorizations()
	if len(storageAuthorizations) != 2 {
		t.Errorf("storage asked %d times", len(storageAuthorizations))
	}
	for _, authorization := range storageAuthorizations {
		if authorization != "" {
			t.Errorf("token forwarded to storage: %q", authorization)
		}
	}
}

func TestOpenFileWithoutToken(t *testing.T) {
	fake := fakegithub.Start(t, fakegithub.DefaultRepository, "")
	addRelease(fake, "0.2.0", "")
	reader, _, err := createSource(t, fake.URL, "", &testClock{now: time.Now()}).OpenFile(context.Background(), "0.2.0", "Hisar-0.2.0.zip")
	if err != nil {
		t.Fatal(err)
	}
	content, _ := io.ReadAll(reader)
	reader.Close()
	if string(content) != "zip of 0.2.0" {
		t.Errorf("content %q", content)
	}
	for _, request := range fake.GetRequests() {
		if request.Authorization != "" {
			t.Errorf("%s sent a token: %q", request.Path, request.Authorization)
		}
	}
}

func TestOpenFileServesOnlyTheUpdateArchive(t *testing.T) {
	fake := startFake(t)
	addRelease(fake, "0.2.0", "")
	fake.AddRelease(createRelease("0.1.0", "", true))
	source := createSource(t, fake.URL, testToken, &testClock{now: time.Now()})
	cases := [][2]string{
		{"0.2.0", "Hisar-0.2.0.dmg"},
		{"0.2.0", "update.json"},
		{"0.2.0", "Hisar-0.3.0.zip"},
		{"0.1.0", "Hisar-0.1.0.dmg"},
		{"0.3.0", "Hisar-0.3.0.zip"},
		{"0.2.0", "../0.1.0/Hisar-0.1.0.dmg"},
		{"0.2.0", ".."},
		{"../0.2.0", "update.json"},
		{"0.2", "update.json"},
		{"0.2.0", "secret%2fkey"},
	}
	for _, testCase := range cases {
		if _, _, err := source.OpenFile(context.Background(), testCase[0], testCase[1]); !errors.Is(err, desktopupdate.ErrFileNotFound) {
			t.Errorf("%s/%s: %v", testCase[0], testCase[1], err)
		}
	}
}
