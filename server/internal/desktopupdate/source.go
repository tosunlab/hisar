package desktopupdate

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	appcastItemCount       = 5
	maximumInspectedCount  = 10
	defaultCacheDuration   = 10 * time.Minute
	defaultRetryAfterError = time.Minute
	refreshTimeout         = time.Minute
)

var ErrFileNotFound = errors.New("no such update file")

type SourceConfig struct {
	PublicURL     string
	CacheDuration time.Duration
	Now           func() time.Time
}

type snapshot struct {
	appcast        []byte
	releasesByName map[string]*Release
	fetchedAt      time.Time
}

// Source caches the appcast and the assets of the releases in it.
type Source struct {
	gitHub           *GitHub
	config           SourceConfig
	mutex            sync.Mutex
	cached           *snapshot
	lastFailureAt    time.Time
	rateLimitedUntil time.Time
	rateLimitErr     error
	// An asset's content never changes; a new upload gets a new asset ID.
	metadataByAssetID map[int64][]byte
}

func NewSource(gitHub *GitHub, config SourceConfig) *Source {
	if config.CacheDuration == 0 {
		config.CacheDuration = defaultCacheDuration
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &Source{gitHub: gitHub, config: config, metadataByAssetID: map[int64][]byte{}}
}

type candidateRelease struct {
	version      string
	buildNumber  int64
	assetsByName map[string]gitHubAsset
}

// selectCandidates keeps published desktop releases, newest first: a desktop-MAJOR.MINOR.PATCH tag, neither draft nor prerelease, with an update.json asset.
func selectCandidates(releases []gitHubRelease) []candidateRelease {
	candidates := []candidateRelease{}
	seenVersion := map[string]bool{}
	for _, listed := range releases {
		version, isDesktop := strings.CutPrefix(listed.TagName, releaseTagPrefix)
		if !isDesktop || listed.IsDraft || listed.IsPrerelease || seenVersion[version] {
			continue
		}
		buildNumber, err := GetBuildNumber(version)
		if err != nil {
			continue
		}
		assetsByName := map[string]gitHubAsset{}
		for _, asset := range listed.Assets {
			if IsValidFileName(asset.Name) && (asset.State == "" || asset.State == "uploaded") {
				assetsByName[asset.Name] = asset
			}
		}
		if _, found := assetsByName[metadataFileName]; !found {
			continue
		}
		seenVersion[version] = true
		candidates = append(candidates, candidateRelease{version: version, buildNumber: buildNumber, assetsByName: assetsByName})
	}
	slices.SortFunc(candidates, func(first, second candidateRelease) int { return cmp.Compare(second.buildNumber, first.buildNumber) })
	return candidates
}

func (source *Source) fetchSnapshot(ctx context.Context) (*snapshot, error) {
	listedReleases, err := source.gitHub.listReleases(ctx)
	if err != nil {
		return nil, err
	}
	candidates := selectCandidates(listedReleases)
	releases := []*Release{}
	metadataByAssetID := map[int64][]byte{}
	for _, candidate := range candidates[:min(len(candidates), maximumInspectedCount)] {
		if len(releases) == appcastItemCount {
			break
		}
		release, err := source.fetchRelease(ctx, candidate, metadataByAssetID)
		if err != nil {
			var limitError *rateLimitError
			if ctx.Err() != nil || errors.As(err, &limitError) {
				return nil, cmp.Or(ctx.Err(), err)
			}
			log.Printf("desktop update %s skipped: %v", candidate.version, err)
			continue
		}
		releases = append(releases, release)
	}
	source.metadataByAssetID = metadataByAssetID
	fetched := &snapshot{appcast: BuildAppcast(source.config.PublicURL, releases), releasesByName: map[string]*Release{}, fetchedAt: source.config.Now()}
	for _, release := range releases {
		fetched.releasesByName[release.Version] = release
	}
	return fetched, nil
}

func (source *Source) fetchRelease(ctx context.Context, candidate candidateRelease, metadataByAssetID map[int64][]byte) (*Release, error) {
	metadataAsset := candidate.assetsByName[metadataFileName]
	if metadataAsset.Size > maximumMetadataSize {
		return nil, errors.New("update.json is too large")
	}
	content, found := source.metadataByAssetID[metadataAsset.ID]
	if !found {
		var err error
		if content, err = source.gitHub.readAsset(ctx, metadataAsset.ID, maximumMetadataSize); err != nil {
			return nil, fmt.Errorf("read update.json: %w", err)
		}
	}
	metadataByAssetID[metadataAsset.ID] = content
	return parseRelease(content, candidate.version, candidate.assetsByName)
}

// getSnapshot refreshes an expired cache. When GitHub fails it keeps serving the previous appcast and retries after a minute,
// or once GitHub's rate limit allows.
func (source *Source) getSnapshot(ctx context.Context) (*snapshot, error) {
	source.mutex.Lock()
	defer source.mutex.Unlock()
	now := source.config.Now()
	if source.cached != nil && now.Sub(source.cached.fetchedAt) < source.config.CacheDuration {
		return source.cached, nil
	}
	if now.Before(source.rateLimitedUntil) {
		if source.cached != nil {
			return source.cached, nil
		}
		return nil, source.rateLimitErr
	}
	if source.cached != nil && now.Sub(source.lastFailureAt) < defaultRetryAfterError {
		return source.cached, nil
	}
	fetchContext, cancel := context.WithTimeout(ctx, refreshTimeout)
	defer cancel()
	fetched, err := source.fetchSnapshot(fetchContext)
	if err != nil {
		source.lastFailureAt = now
		var limitError *rateLimitError
		if errors.As(err, &limitError) {
			source.rateLimitedUntil = now.Add(max(limitError.retryDelay, defaultRetryAfterError))
			source.rateLimitErr = err
		}
		if source.cached != nil {
			log.Printf("desktop updates: serving the cached appcast: %v", err)
			return source.cached, nil
		}
		return nil, err
	}
	source.cached = fetched
	return fetched, nil
}

func (source *Source) GetAppcast(ctx context.Context) ([]byte, error) {
	current, err := source.getSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	return current.appcast, nil
}

// OpenFile streams a file of a release in the appcast; the caller closes the reader.
func (source *Source) OpenFile(ctx context.Context, version, fileName string) (io.ReadCloser, int64, error) {
	if !IsValidVersion(version) || !IsValidFileName(fileName) {
		return nil, 0, ErrFileNotFound
	}
	current, err := source.getSnapshot(ctx)
	if err != nil {
		return nil, 0, err
	}
	release, found := current.releasesByName[version]
	if !found {
		return nil, 0, ErrFileNotFound
	}
	asset, found := release.assetsByName[fileName]
	if !found {
		return nil, 0, ErrFileNotFound
	}
	response, err := source.gitHub.openAsset(ctx, asset.ID)
	if errors.Is(err, errNotFound) {
		return nil, 0, ErrFileNotFound
	}
	if err != nil {
		return nil, 0, err
	}
	size := response.ContentLength
	if size < 0 {
		size = asset.Size
	}
	return response.Body, size, nil
}
