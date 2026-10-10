package desktopupdate

import (
	"cmp"
	"context"
	"errors"
	"io"
	"log"
	"slices"
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
	appcast          []byte
	allReleaseByName map[string]*Release
	fetchedAt        time.Time
}

// Source caches the appcast and the files of the releases in it.
type Source struct {
	gitLab        *GitLab
	config        SourceConfig
	mutex         sync.Mutex
	cached        *snapshot
	lastFailureAt time.Time
}

func CreateSource(gitLab *GitLab, config SourceConfig) *Source {
	if config.CacheDuration == 0 {
		config.CacheDuration = defaultCacheDuration
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &Source{gitLab: gitLab, config: config}
}

type versionedPackage struct {
	gitLabPackage
	buildNumber int64
}

func (source *Source) fetchSnapshot(ctx context.Context) (*snapshot, error) {
	allPackage, err := source.gitLab.listAllPackage(ctx)
	if err != nil {
		return nil, err
	}
	allCandidate := []versionedPackage{}
	seenVersion := map[string]bool{}
	for _, listedPackage := range allPackage {
		if listedPackage.Name != PackageName || listedPackage.PackageType != "generic" || (listedPackage.Status != "" && listedPackage.Status != "default") {
			continue
		}
		buildNumber, err := GetBuildNumber(listedPackage.Version)
		if err != nil || seenVersion[listedPackage.Version] {
			continue
		}
		seenVersion[listedPackage.Version] = true
		allCandidate = append(allCandidate, versionedPackage{gitLabPackage: listedPackage, buildNumber: buildNumber})
	}
	slices.SortFunc(allCandidate, func(first, second versionedPackage) int { return cmp.Compare(second.buildNumber, first.buildNumber) })

	allRelease := []*Release{}
	for _, candidate := range allCandidate[:min(len(allCandidate), maximumInspectedCount)] {
		if len(allRelease) == appcastItemCount {
			break
		}
		release, err := source.fetchRelease(ctx, candidate.gitLabPackage)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			log.Printf("desktop update %s skipped: %v", candidate.Version, err)
			continue
		}
		allRelease = append(allRelease, release)
	}
	fetched := &snapshot{appcast: BuildAppcast(source.config.PublicURL, allRelease), allReleaseByName: map[string]*Release{}, fetchedAt: source.config.Now()}
	for _, release := range allRelease {
		fetched.allReleaseByName[release.Version] = release
	}
	return fetched, nil
}

func (source *Source) fetchRelease(ctx context.Context, listedPackage gitLabPackage) (*Release, error) {
	allFile, err := source.gitLab.listAllPackageFile(ctx, listedPackage.ID)
	if err != nil {
		return nil, err
	}
	allFileSizeByName := map[string]int64{}
	for _, file := range allFile {
		// A re-uploaded file is listed again; the newest upload is served, and it comes last.
		if IsValidFileName(file.FileName) {
			allFileSizeByName[file.FileName] = file.Size
		}
	}
	if _, found := allFileSizeByName[metadataFileName]; !found {
		return nil, errors.New("no update.json")
	}
	content, err := source.gitLab.readMetadata(ctx, listedPackage.Version)
	if err != nil {
		return nil, err
	}
	return parseRelease(content, listedPackage.Version, allFileSizeByName)
}

// getSnapshot refreshes an expired cache; when GitLab fails it keeps serving the previous appcast and retries after a minute.
func (source *Source) getSnapshot(ctx context.Context) (*snapshot, error) {
	source.mutex.Lock()
	defer source.mutex.Unlock()
	now := source.config.Now()
	if source.cached != nil && now.Sub(source.cached.fetchedAt) < source.config.CacheDuration {
		return source.cached, nil
	}
	if source.cached != nil && now.Sub(source.lastFailureAt) < defaultRetryAfterError {
		return source.cached, nil
	}
	fetchContext, cancel := context.WithTimeout(ctx, refreshTimeout)
	defer cancel()
	fetched, err := source.fetchSnapshot(fetchContext)
	if err != nil {
		source.lastFailureAt = now
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
	release, found := current.allReleaseByName[version]
	if !found {
		return nil, 0, ErrFileNotFound
	}
	listedSize, found := release.allFileSizeByName[fileName]
	if !found {
		return nil, 0, ErrFileNotFound
	}
	response, err := source.gitLab.openFile(ctx, version, fileName)
	if errors.Is(err, errNotFound) {
		return nil, 0, ErrFileNotFound
	}
	if err != nil {
		return nil, 0, err
	}
	size := response.ContentLength
	if size < 0 {
		size = listedSize
	}
	return response.Body, size, nil
}
