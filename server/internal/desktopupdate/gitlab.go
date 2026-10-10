// Package desktopupdate serves the macOS app's Sparkle appcast and release files from a GitLab generic package.
package desktopupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	PackageName          = "hisar-desktop"
	maximumListedPackage = 100
	maximumMetadataSize  = 1 << 20
)

type GitLabConfig struct {
	BaseURL   string
	ProjectID string
	Token     string
}

type GitLab struct {
	config     GitLabConfig
	httpClient *http.Client
}

type gitLabPackage struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	PackageType string `json:"package_type"`
	Status      string `json:"status"`
}

type gitLabPackageFile struct {
	FileName string `json:"file_name"`
	Size     int64  `json:"size"`
}

var errNotFound = errors.New("not found on GitLab")

func CreateGitLab(config GitLabConfig) (*GitLab, error) {
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	parsedURL, err := url.Parse(config.BaseURL)
	if err != nil || (parsedURL.Scheme != "https" && parsedURL.Scheme != "http") || parsedURL.Host == "" {
		return nil, errors.New("the GitLab URL must be an http(s) address such as https://git.example.com")
	}
	if strings.TrimSpace(config.ProjectID) == "" || strings.ContainsAny(config.ProjectID, "/?#%") {
		return nil, errors.New("the GitLab project ID must be the project's numeric ID")
	}
	if strings.TrimSpace(config.Token) == "" {
		return nil, errors.New("a GitLab token is required")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 30 * time.Second
	gitLab := &GitLab{config: config}
	gitLab.httpClient = &http.Client{
		Transport: transport,
		// Generic package downloads may redirect to object storage; the token must stay with GitLab.
		CheckRedirect: func(request *http.Request, allPreviousRequest []*http.Request) error {
			if len(allPreviousRequest) >= 10 {
				return errors.New("too many redirects")
			}
			if request.URL.Host != parsedURL.Host || request.URL.Scheme != parsedURL.Scheme {
				request.Header.Del("PRIVATE-TOKEN")
			}
			return nil
		},
	}
	return gitLab, nil
}

func (gitLab *GitLab) buildURL(path string, query url.Values) string {
	address := gitLab.config.BaseURL + "/api/v4/projects/" + url.PathEscape(gitLab.config.ProjectID) + path
	if len(query) > 0 {
		address += "?" + query.Encode()
	}
	return address
}

func (gitLab *GitLab) get(ctx context.Context, address string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("PRIVATE-TOKEN", gitLab.config.Token)
	response, err := gitLab.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode == http.StatusNotFound {
		response.Body.Close()
		return nil, errNotFound
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("GitLab answered %s", response.Status)
	}
	return response, nil
}

func (gitLab *GitLab) getJSON(ctx context.Context, address string, target any) error {
	response, err := gitLab.get(ctx, address)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(target)
}

func (gitLab *GitLab) listAllPackage(ctx context.Context) ([]gitLabPackage, error) {
	query := url.Values{
		"package_type": {"generic"},
		"package_name": {PackageName},
		"order_by":     {"created_at"},
		"sort":         {"desc"},
		"per_page":     {fmt.Sprint(maximumListedPackage)},
	}
	var allPackage []gitLabPackage
	if err := gitLab.getJSON(ctx, gitLab.buildURL("/packages", query), &allPackage); err != nil {
		return nil, fmt.Errorf("list packages: %w", err)
	}
	return allPackage, nil
}

func (gitLab *GitLab) listAllPackageFile(ctx context.Context, packageID int64) ([]gitLabPackageFile, error) {
	var allFile []gitLabPackageFile
	address := gitLab.buildURL(fmt.Sprintf("/packages/%d/package_files", packageID), url.Values{"per_page": {"100"}})
	if err := gitLab.getJSON(ctx, address, &allFile); err != nil {
		return nil, fmt.Errorf("list package files: %w", err)
	}
	return allFile, nil
}

// openFile returns the body unread; the caller closes it.
func (gitLab *GitLab) openFile(ctx context.Context, version, fileName string) (*http.Response, error) {
	path := "/packages/generic/" + PackageName + "/" + url.PathEscape(version) + "/" + url.PathEscape(fileName)
	return gitLab.get(ctx, gitLab.buildURL(path, nil))
}

func (gitLab *GitLab) readMetadata(ctx context.Context, version string) ([]byte, error) {
	response, err := gitLab.openFile(ctx, version, metadataFileName)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	content, err := io.ReadAll(io.LimitReader(response.Body, maximumMetadataSize+1))
	if err != nil {
		return nil, err
	}
	if len(content) > maximumMetadataSize {
		return nil, errors.New("update.json is too large")
	}
	return content, nil
}
