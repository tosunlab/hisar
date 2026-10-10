package desktopupdate_test

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tryhisar/hisar/server/internal/desktopupdate"
	"github.com/tryhisar/hisar/server/internal/desktopupdate/fakegitlab"
)

const testToken = "glpat-test"

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

func addRelease(fake *fakegitlab.Server, version, releaseNotes string) []fakegitlab.File {
	buildNumber, err := desktopupdate.GetBuildNumber(version)
	if err != nil {
		panic(err)
	}
	allFile := fakegitlab.CreateAllReleaseFile(fakegitlab.ReleaseConfig{
		Version: version, BuildNumber: buildNumber, Archive: []byte("zip of " + version), ReleaseNotes: releaseNotes,
		PublishedAt: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC).Add(time.Duration(buildNumber) * time.Second),
	})
	allFile = append(allFile, fakegitlab.File{Name: "Hisar-" + version + ".dmg", Content: []byte("dmg of " + version)})
	fake.AddPackage(desktopupdate.PackageName, version, allFile...)
	return allFile
}

func createSource(t *testing.T, fake *fakegitlab.Server, clock *testClock) *desktopupdate.Source {
	t.Helper()
	gitLab, err := desktopupdate.CreateGitLab(desktopupdate.GitLabConfig{BaseURL: fake.URL + "/", ProjectID: fakegitlab.ProjectID, Token: testToken})
	if err != nil {
		t.Fatal(err)
	}
	return desktopupdate.CreateSource(gitLab, desktopupdate.SourceConfig{PublicURL: "https://hisar.test/", Now: clock.getNow})
}

type appcastDocument struct {
	XMLName xml.Name `xml:"rss"`
	AllItem []struct {
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

func TestGetBuildNumber(t *testing.T) {
	allExpected := map[string]int64{"0.0.1": 1, "0.1.0": 1000, "0.2.1": 2001, "1.0.0": 1000000, "12.345.999": 12345999}
	for version, expected := range allExpected {
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

func TestCreateGitLabValidatesConfig(t *testing.T) {
	allConfig := []desktopupdate.GitLabConfig{
		{BaseURL: "git.example.com", ProjectID: "2", Token: "t"},
		{BaseURL: "ftp://git.example.com", ProjectID: "2", Token: "t"},
		{BaseURL: "https://git.example.com", ProjectID: "hisar/core", Token: "t"},
		{BaseURL: "https://git.example.com", ProjectID: "2", Token: ""},
	}
	for _, config := range allConfig {
		if _, err := desktopupdate.CreateGitLab(config); err == nil {
			t.Errorf("%+v accepted", config)
		}
	}
}

func TestAppcastListsNewestValidReleases(t *testing.T) {
	fake := fakegitlab.Start(t, testToken)
	fake.AddPackage(desktopupdate.PackageName, "0.1.0", fakegitlab.File{Name: "Hisar-0.1.0.dmg", Content: []byte("dmg")})
	for _, version := range []string{"0.2.0", "0.3.0", "0.9.0", "0.10.0", "1.0.0"} {
		addRelease(fake, version, "")
	}
	addRelease(fake, "0.11.0", "Fixes <b>bold</b> & \"quotes\"\n\n- second line ]]>")
	brokenFile := fakegitlab.CreateAllReleaseFile(fakegitlab.ReleaseConfig{Version: "2.0.0", BuildNumber: 1, Archive: []byte("x"), PublishedAt: time.Now()})
	fake.AddPackage(desktopupdate.PackageName, "2.0.0", brokenFile...)
	fake.AddPackage(desktopupdate.PackageName, "not-a-version", brokenFile...)
	otherPackageFile := fakegitlab.CreateAllReleaseFile(fakegitlab.ReleaseConfig{Version: "3.0.0", BuildNumber: 3000000, Archive: []byte("x"), PublishedAt: time.Now()})
	fake.AddPackage(desktopupdate.PackageName+"-beta", "3.0.0", otherPackageFile...)

	clock := &testClock{now: time.Now()}
	source := createSource(t, fake, clock)
	appcast, err := source.GetAppcast(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	document := parseAppcast(t, appcast)
	allShortVersion := []string{}
	for _, item := range document.AllItem {
		allShortVersion = append(allShortVersion, item.ShortVersion)
	}
	if strings.Join(allShortVersion, " ") != "1.0.0 0.11.0 0.10.0 0.9.0 0.3.0" {
		t.Fatalf("items: %v\n%s", allShortVersion, appcast)
	}
	newest, withNotes := document.AllItem[0], document.AllItem[1]
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
	for _, path := range fake.GetAllRequestPath() {
		if strings.Contains(path, "/0.1.0/") || strings.Contains(path, "/0.2.0/") {
			t.Errorf("fetched beyond the newest valid releases: %s", path)
		}
	}
}

func TestAppcastIsCachedForTenMinutes(t *testing.T) {
	fake := fakegitlab.Start(t, testToken)
	addRelease(fake, "0.2.0", "")
	clock := &testClock{now: time.Now()}
	source := createSource(t, fake, clock)
	if _, err := source.GetAppcast(context.Background()); err != nil {
		t.Fatal(err)
	}
	requestCount := len(fake.GetAllRequestPath())
	addRelease(fake, "0.3.0", "")
	clock.advance(9 * time.Minute)
	appcast, _ := source.GetAppcast(context.Background())
	if len(fake.GetAllRequestPath()) != requestCount || bytes.Contains(appcast, []byte("0.3.0")) {
		t.Error("appcast refreshed before ten minutes")
	}
	clock.advance(2 * time.Minute)
	appcast, _ = source.GetAppcast(context.Background())
	if !bytes.Contains(appcast, []byte("<sparkle:shortVersionString>0.3.0<")) {
		t.Errorf("appcast not refreshed after ten minutes:\n%s", appcast)
	}
}

func TestAppcastSurvivesGitLabOutage(t *testing.T) {
	fake := fakegitlab.Start(t, testToken)
	addRelease(fake, "0.2.0", "")
	clock := &testClock{now: time.Now()}
	source := createSource(t, fake, clock)
	if _, err := source.GetAppcast(context.Background()); err != nil {
		t.Fatal(err)
	}
	fake.Token = "rotated"
	clock.advance(11 * time.Minute)
	appcast, err := source.GetAppcast(context.Background())
	if err != nil || !bytes.Contains(appcast, []byte("0.2.0")) {
		t.Errorf("cached appcast not served during an outage: %v", err)
	}
	requestCount := len(fake.GetAllRequestPath())
	source.GetAppcast(context.Background())
	if len(fake.GetAllRequestPath()) != requestCount {
		t.Error("retried GitLab right after a failure")
	}

	unreachable := createSource(t, fake, clock)
	if _, err := unreachable.GetAppcast(context.Background()); err == nil {
		t.Error("no error without any appcast")
	}
}

func TestOpenFileStreamsListedFiles(t *testing.T) {
	fake := fakegitlab.Start(t, testToken)
	allFile := addRelease(fake, "0.2.0", "")
	firstHalf, secondHalf := bytes.Repeat([]byte("a"), 64<<10), bytes.Repeat([]byte("b"), 64<<10)
	releaseSecondHalf := make(chan struct{})
	fake.BeforeDownload = func(writer http.ResponseWriter, request *http.Request, fileName string) bool {
		if fileName != "Hisar-0.2.0.dmg" {
			return true
		}
		writer.Header().Set("Content-Length", fmt.Sprint(len(firstHalf)+len(secondHalf)))
		writer.Write(firstHalf)
		writer.(http.Flusher).Flush()
		<-releaseSecondHalf
		writer.Write(secondHalf)
		return false
	}
	source := createSource(t, fake, &testClock{now: time.Now()})

	reader, size, err := source.OpenFile(context.Background(), "0.2.0", "Hisar-0.2.0.zip")
	if err != nil {
		t.Fatal(err)
	}
	content, _ := io.ReadAll(reader)
	reader.Close()
	if size != int64(len(allFile[0].Content)) || !bytes.Equal(content, allFile[0].Content) {
		t.Errorf("zip: %d %q", size, content)
	}

	reader, size, err = source.OpenFile(context.Background(), "0.2.0", "Hisar-0.2.0.dmg")
	if err != nil {
		t.Fatal(err)
	}
	if size != int64(len(firstHalf)+len(secondHalf)) {
		t.Errorf("size = %d", size)
	}
	close(releaseSecondHalf)
	content, _ = io.ReadAll(reader)
	reader.Close()
	if !bytes.Equal(content, append(firstHalf, secondHalf...)) {
		t.Errorf("streamed %d bytes", len(content))
	}
}

func TestOpenFileRefusesUnlistedFiles(t *testing.T) {
	fake := fakegitlab.Start(t, testToken)
	addRelease(fake, "0.2.0", "")
	fake.AddPackage(desktopupdate.PackageName, "0.1.0", fakegitlab.File{Name: "Hisar-0.1.0.dmg", Content: []byte("dmg")})
	source := createSource(t, fake, &testClock{now: time.Now()})
	allCase := [][2]string{
		{"0.2.0", "Hisar-0.3.0.zip"},
		{"0.1.0", "Hisar-0.1.0.dmg"},
		{"0.3.0", "Hisar-0.3.0.zip"},
		{"0.2.0", "../0.1.0/Hisar-0.1.0.dmg"},
		{"0.2.0", ".."},
		{"../0.2.0", "update.json"},
		{"0.2", "update.json"},
		{"0.2.0", "secret%2fkey"},
	}
	for _, testCase := range allCase {
		if _, _, err := source.OpenFile(context.Background(), testCase[0], testCase[1]); !errors.Is(err, desktopupdate.ErrFileNotFound) {
			t.Errorf("%s/%s: %v", testCase[0], testCase[1], err)
		}
	}
}

func TestRedirectDoesNotForwardToken(t *testing.T) {
	var storageToken string
	storage := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		storageToken = request.Header.Get("PRIVATE-TOKEN")
		writer.Write([]byte("from storage"))
	}))
	t.Cleanup(storage.Close)
	fake := fakegitlab.Start(t, testToken)
	addRelease(fake, "0.2.0", "")
	fake.BeforeDownload = func(writer http.ResponseWriter, request *http.Request, fileName string) bool {
		if fileName == "update.json" {
			return true
		}
		http.Redirect(writer, request, storage.URL+"/bucket/object", http.StatusFound)
		return false
	}
	source := createSource(t, fake, &testClock{now: time.Now()})
	reader, _, err := source.OpenFile(context.Background(), "0.2.0", "Hisar-0.2.0.zip")
	if err != nil {
		t.Fatal(err)
	}
	content, _ := io.ReadAll(reader)
	reader.Close()
	if string(content) != "from storage" || storageToken != "" {
		t.Errorf("content %q, token sent to storage %q", content, storageToken)
	}
}
