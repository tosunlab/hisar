// Package fakegithub is an in-memory GitHub Releases API, with asset downloads redirected to a separate storage host.
package fakegithub

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/project"
)

const DefaultRepository = project.GitHubRepository

type Asset struct {
	Name    string
	Content []byte
}

type Release struct {
	TagName      string
	IsDraft      bool
	IsPrerelease bool
	Assets       []Asset
}

type storedAsset struct {
	id int64
	Asset
}

type storedRelease struct {
	Release
	id     int64
	stored []storedAsset
}

type Request struct {
	Path          string
	Authorization string
	IfNoneMatch   string
	Status        int
}

type Server struct {
	// URL is the API's base URL once started.
	URL        string
	Repository string
	// Token, when set, is required as a bearer token; without it the repository answers 404 like a private one.
	Token string
	// StorageURL is where asset downloads redirect to.
	StorageURL string
	// BeforeDownload runs before storage sends an asset, e.g. to stream it slowly; false means it answered itself.
	BeforeDownload func(writer http.ResponseWriter, request *http.Request, assetName string) bool

	mutex                 sync.Mutex
	releases              []*storedRelease
	nextID                int64
	requests              []Request
	storageAuthorizations []string
	rateLimitStatus       int
	rateLimitDelay        int
	closeAll              func()
}

func New(repository, token string) *Server {
	return &Server{Repository: repository, Token: token, nextID: 1}
}

// Start serves the API and the storage on two test servers.
func Start(t testing.TB, repository, token string) *Server {
	fake := New(repository, token)
	api := httptest.NewServer(http.HandlerFunc(fake.ServeAPI))
	storage := httptest.NewServer(http.HandlerFunc(fake.ServeStorage))
	fake.closeAll = func() {
		api.Close()
		storage.Close()
	}
	t.Cleanup(fake.Close)
	fake.URL, fake.StorageURL = api.URL, storage.URL
	return fake
}

func (fake *Server) Close() {
	if fake.closeAll != nil {
		fake.closeAll()
	}
}

func (fake *Server) AddRelease(release Release) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	stored := &storedRelease{Release: release, id: fake.nextID}
	fake.nextID++
	for _, asset := range release.Assets {
		stored.stored = append(stored.stored, storedAsset{id: fake.nextID, Asset: asset})
		fake.nextID++
	}
	fake.releases = append(fake.releases, stored)
}

// PublishRelease does what the Publish workflow does: renames update-unpublished.json to update.json and ends the prerelease.
func (fake *Server) PublishRelease(tagName string) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	for _, stored := range fake.releases {
		if stored.TagName != tagName {
			continue
		}
		stored.IsPrerelease = false
		for index := range stored.stored {
			if stored.stored[index].Name == "update-unpublished.json" {
				stored.stored[index].Name = "update.json"
			}
		}
	}
}

// LimitRate answers every API request with status (403 or 429) until called with 0.
func (fake *Server) LimitRate(status, retryAfterSecond int) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.rateLimitStatus, fake.rateLimitDelay = status, retryAfterSecond
}

func (fake *Server) GetRequests() []Request {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return append([]Request(nil), fake.requests...)
}

// GetStorageAuthorizations returns the Authorization header of every storage request, in order.
func (fake *Server) GetStorageAuthorizations() []string {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return append([]string(nil), fake.storageAuthorizations...)
}

func (fake *Server) recordRequest(request *http.Request, status int) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.requests = append(fake.requests, Request{
		Path: request.URL.RequestURI(), Authorization: request.Header.Get("Authorization"), IfNoneMatch: request.Header.Get("If-None-Match"), Status: status,
	})
}

func (fake *Server) findAsset(assetID int64) (storedAsset, bool) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	for _, stored := range fake.releases {
		for _, asset := range stored.stored {
			if asset.id == assetID {
				return asset, true
			}
		}
	}
	return storedAsset{}, false
}

func (fake *Server) ServeAPI(writer http.ResponseWriter, request *http.Request) {
	status := fake.answerAPI(writer, request)
	fake.recordRequest(request, status)
}

func (fake *Server) answerAPI(writer http.ResponseWriter, request *http.Request) int {
	fake.mutex.Lock()
	rateLimitStatus, rateLimitDelay := fake.rateLimitStatus, fake.rateLimitDelay
	fake.mutex.Unlock()
	if rateLimitStatus != 0 {
		if rateLimitStatus == http.StatusTooManyRequests {
			writer.Header().Set("Retry-After", strconv.Itoa(rateLimitDelay))
		} else {
			writer.Header().Set("X-RateLimit-Remaining", "0")
			writer.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(time.Duration(rateLimitDelay)*time.Second).Unix(), 10))
		}
		http.Error(writer, `{"message":"API rate limit exceeded"}`, rateLimitStatus)
		return rateLimitStatus
	}
	path, found := strings.CutPrefix(request.URL.Path, "/repos/"+fake.Repository+"/releases")
	if !found || (fake.Token != "" && request.Header.Get("Authorization") != "Bearer "+fake.Token) {
		http.Error(writer, `{"message":"Not Found"}`, http.StatusNotFound)
		return http.StatusNotFound
	}
	if path == "" {
		return fake.listRelease(writer, request)
	}
	assetID, err := strconv.ParseInt(strings.TrimPrefix(path, "/assets/"), 10, 64)
	asset, found := fake.findAsset(assetID)
	if !strings.HasPrefix(path, "/assets/") || err != nil || !found {
		http.Error(writer, `{"message":"Not Found"}`, http.StatusNotFound)
		return http.StatusNotFound
	}
	if request.Header.Get("Accept") != "application/octet-stream" {
		writeJSON(writer, map[string]any{"id": asset.id, "name": asset.Name})
		return http.StatusOK
	}
	http.Redirect(writer, request, fake.StorageURL+"/assets/"+strconv.FormatInt(asset.id, 10)+"?signature=fake", http.StatusFound)
	return http.StatusFound
}

// listRelease mirrors GitHub: newest first, per_page up to 100, a Link header to the next page and ETag with If-None-Match.
func (fake *Server) listRelease(writer http.ResponseWriter, request *http.Request) int {
	query := request.URL.Query()
	pageSize, _ := strconv.Atoi(query.Get("per_page"))
	if pageSize <= 0 {
		pageSize = 30
	}
	pageSize = min(pageSize, 100)
	page, _ := strconv.Atoi(query.Get("page"))
	page = max(page, 1)
	fake.mutex.Lock()
	listed := []map[string]any{}
	for index := len(fake.releases) - 1; index >= 0; index-- {
		stored := fake.releases[index]
		if stored.IsDraft && fake.Token == "" {
			continue
		}
		assets := []map[string]any{}
		for _, asset := range stored.stored {
			assets = append(assets, map[string]any{
				"id": asset.id, "name": asset.Name, "size": len(asset.Content), "state": "uploaded",
				"url": "https://api.github.invalid/assets/" + strconv.FormatInt(asset.id, 10),
			})
		}
		listed = append(listed, map[string]any{
			"id": stored.id, "tag_name": stored.TagName, "draft": stored.IsDraft, "prerelease": stored.IsPrerelease, "assets": assets,
		})
	}
	fake.mutex.Unlock()
	start, end := min((page-1)*pageSize, len(listed)), min(page*pageSize, len(listed))
	content, _ := json.Marshal(listed[start:end])
	digest := sha256.Sum256(content)
	etag := `W/"` + hex.EncodeToString(digest[:]) + `"`
	writer.Header().Set("ETag", etag)
	if end < len(listed) {
		writer.Header().Set("Link", fmt.Sprintf(`<%s?per_page=%d&page=%d>; rel="next"`, request.URL.Path, pageSize, page+1))
	}
	if request.Header.Get("If-None-Match") == etag {
		writer.WriteHeader(http.StatusNotModified)
		return http.StatusNotModified
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.Write(content)
	return http.StatusOK
}

func (fake *Server) ServeStorage(writer http.ResponseWriter, request *http.Request) {
	fake.mutex.Lock()
	fake.storageAuthorizations = append(fake.storageAuthorizations, request.Header.Get("Authorization"))
	fake.mutex.Unlock()
	assetID, err := strconv.ParseInt(strings.TrimPrefix(request.URL.Path, "/assets/"), 10, 64)
	asset, found := fake.findAsset(assetID)
	if err != nil || !found {
		http.NotFound(writer, request)
		return
	}
	if fake.BeforeDownload != nil && !fake.BeforeDownload(writer, request, asset.Name) {
		return
	}
	writer.Header().Set("Content-Type", "application/octet-stream")
	writer.Header().Set("Content-Length", strconv.Itoa(len(asset.Content)))
	writer.Write(asset.Content)
}

func writeJSON(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(value)
}

type ReleaseConfig struct {
	Version            string
	BuildNumber        int64
	Archive            []byte
	ReleaseNotes       string
	ReleaseNotesFormat string
	PublishedAt        time.Time
	IsUnpublished      bool
}

// NewDesktopRelease returns the release the Release desktop workflow creates, with a fixed signature;
// published unless IsUnpublished, in which case it is a prerelease with update-unpublished.json.
func NewDesktopRelease(config ReleaseConfig) Release {
	archiveName := "Hisar-" + config.Version + ".zip"
	if config.ReleaseNotesFormat == "" {
		config.ReleaseNotesFormat = "markdown"
	}
	metadata, _ := json.Marshal(map[string]any{
		"version":              config.Version,
		"buildNumber":          config.BuildNumber,
		"archive":              archiveName,
		"edSignature":          base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 64)),
		"length":               len(config.Archive),
		"minimumSystemVersion": "12.0",
		"releaseNotes":         config.ReleaseNotes,
		"releaseNotesFormat":   config.ReleaseNotesFormat,
		"publishedAt":          config.PublishedAt.UTC().Format(time.RFC3339),
	})
	metadataName := "update.json"
	if config.IsUnpublished {
		metadataName = "update-unpublished.json"
	}
	return Release{
		TagName:      "desktop-" + config.Version,
		IsPrerelease: config.IsUnpublished,
		Assets: []Asset{
			{Name: "Hisar-" + config.Version + ".dmg", Content: []byte("dmg of " + config.Version)},
			{Name: archiveName, Content: config.Archive},
			{Name: metadataName, Content: metadata},
		},
	}
}
