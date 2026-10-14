// Package desktopupdate serves the macOS app's Sparkle appcast and release files from the GitHub Releases of the desktop app.
package desktopupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultGitHubAPIURL     = "https://api.github.com"
	DefaultGitHubRepository = "hisarapp/core"
	releaseTagPrefix        = "desktop-"
	releasePageSize         = 100
	maximumReleasePageCount = 10
	maximumReleasePageSize  = 16 << 20
	maximumRateLimitDelay   = time.Hour
)

var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})/[A-Za-z0-9._-]{1,100}$`)

type GitHubConfig struct {
	APIURL     string
	Repository string
	Token      string
}

type cachedPage struct {
	etag    string
	content []byte
	hasNext bool
}

type GitHub struct {
	config     GitHubConfig
	apiHost    string
	httpClient *http.Client

	mutex            sync.Mutex
	cachedPagesByURL map[string]cachedPage
}

type gitHubAsset struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	State string `json:"state"`
}

type gitHubRelease struct {
	TagName      string        `json:"tag_name"`
	IsDraft      bool          `json:"draft"`
	IsPrerelease bool          `json:"prerelease"`
	Assets       []gitHubAsset `json:"assets"`
}

var errNotFound = errors.New("not found on GitHub")

type rateLimitError struct {
	retryDelay time.Duration
}

func (limitError *rateLimitError) Error() string {
	return fmt.Sprintf("GitHub rate limit reached, retrying in %s", limitError.retryDelay.Round(time.Second))
}

func NewGitHub(config GitHubConfig) (*GitHub, error) {
	config.APIURL = strings.TrimRight(strings.TrimSpace(config.APIURL), "/")
	if config.APIURL == "" {
		config.APIURL = DefaultGitHubAPIURL
	}
	parsedURL, err := url.Parse(config.APIURL)
	if err != nil || (parsedURL.Scheme != "https" && parsedURL.Scheme != "http") || parsedURL.Host == "" {
		return nil, errors.New("the GitHub API URL must be an http(s) address such as https://api.github.com")
	}
	config.Repository = strings.TrimSpace(config.Repository)
	if !repositoryPattern.MatchString(config.Repository) || strings.Contains(config.Repository, "..") {
		return nil, fmt.Errorf("the GitHub repository %q is not owner/name", config.Repository)
	}
	config.Token = strings.TrimSpace(config.Token)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 30 * time.Second
	gitHub := &GitHub{config: config, apiHost: parsedURL.Host, cachedPagesByURL: map[string]cachedPage{}}
	gitHub.httpClient = &http.Client{
		Transport: transport,
		// Asset downloads redirect to storage on another host; the token must stay with the API.
		CheckRedirect: func(request *http.Request, previousRequests []*http.Request) error {
			if len(previousRequests) >= 10 {
				return errors.New("too many redirects")
			}
			if request.URL.Host != parsedURL.Host || request.URL.Scheme != parsedURL.Scheme {
				request.Header.Del("Authorization")
			}
			return nil
		},
	}
	return gitHub, nil
}

func (gitHub *GitHub) buildURL(path string, query url.Values) string {
	address := gitHub.config.APIURL + "/repos/" + gitHub.config.Repository + path
	if len(query) > 0 {
		address += "?" + query.Encode()
	}
	return address
}

func readRateLimitDelay(response *http.Response) (time.Duration, bool) {
	if response.StatusCode != http.StatusForbidden && response.StatusCode != http.StatusTooManyRequests {
		return 0, false
	}
	if seconds, err := strconv.Atoi(response.Header.Get("Retry-After")); err == nil && seconds >= 0 {
		return min(time.Duration(seconds)*time.Second, maximumRateLimitDelay), true
	}
	if response.Header.Get("X-RateLimit-Remaining") == "0" {
		if resetUnix, err := strconv.ParseInt(response.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			return min(max(time.Until(time.Unix(resetUnix, 0)), 0), maximumRateLimitDelay), true
		}
		return time.Minute, true
	}
	return time.Minute, response.StatusCode == http.StatusTooManyRequests
}

func (gitHub *GitHub) get(ctx context.Context, address, accept, etag string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", accept)
	request.Header.Set("User-Agent", "hisar-server")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if gitHub.config.Token != "" {
		request.Header.Set("Authorization", "Bearer "+gitHub.config.Token)
	}
	if etag != "" {
		request.Header.Set("If-None-Match", etag)
	}
	response, err := gitHub.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	switch {
	case response.StatusCode == http.StatusOK || (etag != "" && response.StatusCode == http.StatusNotModified):
		return response, nil
	case response.StatusCode == http.StatusNotFound:
		response.Body.Close()
		return nil, errNotFound
	}
	response.Body.Close()
	if delay, isRateLimited := readRateLimitDelay(response); isRateLimited {
		return nil, &rateLimitError{retryDelay: delay}
	}
	return nil, fmt.Errorf("GitHub answered %s", response.Status)
}

func hasNextPage(linkHeader string) bool {
	for _, link := range strings.Split(linkHeader, ",") {
		if strings.Contains(link, `rel="next"`) {
			return true
		}
	}
	return false
}

// getReleasePage sends the page's ETag, so an unchanged page costs no rate limit.
func (gitHub *GitHub) getReleasePage(ctx context.Context, page int) ([]gitHubRelease, bool, error) {
	address := gitHub.buildURL("/releases", url.Values{"per_page": {strconv.Itoa(releasePageSize)}, "page": {strconv.Itoa(page)}})
	gitHub.mutex.Lock()
	cached, isCached := gitHub.cachedPagesByURL[address]
	gitHub.mutex.Unlock()
	response, err := gitHub.get(ctx, address, "application/vnd.github+json", cached.etag)
	if err != nil {
		return nil, false, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotModified && isCached {
		io.Copy(io.Discard, response.Body)
	} else {
		content, err := io.ReadAll(io.LimitReader(response.Body, maximumReleasePageSize+1))
		if err != nil {
			return nil, false, err
		}
		if len(content) > maximumReleasePageSize {
			return nil, false, errors.New("the release list is too large")
		}
		cached = cachedPage{etag: response.Header.Get("ETag"), content: content, hasNext: hasNextPage(response.Header.Get("Link"))}
		gitHub.mutex.Lock()
		if cached.etag != "" {
			gitHub.cachedPagesByURL[address] = cached
		} else {
			delete(gitHub.cachedPagesByURL, address)
		}
		gitHub.mutex.Unlock()
	}
	var releases []gitHubRelease
	if err := json.Unmarshal(cached.content, &releases); err != nil {
		return nil, false, fmt.Errorf("release list: %w", err)
	}
	return releases, cached.hasNext, nil
}

func (gitHub *GitHub) listReleases(ctx context.Context) ([]gitHubRelease, error) {
	var releases []gitHubRelease
	for page := 1; page <= maximumReleasePageCount; page++ {
		pageReleases, hasNext, err := gitHub.getReleasePage(ctx, page)
		if errors.Is(err, errNotFound) {
			return nil, fmt.Errorf("list releases: no repository %s (a private repository needs a token)", gitHub.config.Repository)
		}
		if err != nil {
			return nil, fmt.Errorf("list releases: %w", err)
		}
		releases = append(releases, pageReleases...)
		if !hasNext || len(pageReleases) == 0 {
			break
		}
	}
	return releases, nil
}

// openAsset returns the asset's body unread; the caller closes it.
func (gitHub *GitHub) openAsset(ctx context.Context, assetID int64) (*http.Response, error) {
	return gitHub.get(ctx, gitHub.buildURL("/releases/assets/"+strconv.FormatInt(assetID, 10), nil), "application/octet-stream", "")
}

func (gitHub *GitHub) readAsset(ctx context.Context, assetID int64, maximumSize int64) ([]byte, error) {
	response, err := gitHub.openAsset(ctx, assetID)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	content, err := io.ReadAll(io.LimitReader(response.Body, maximumSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > maximumSize {
		return nil, errors.New("the asset is too large")
	}
	return content, nil
}
