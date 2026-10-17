// Package serverupdate finds the newest hisar-server release among the GitHub Releases of the update repository, so the
// admin dashboard can say when a newer server exists.
package serverupdate

import (
	"context"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tryhisar/hisar/core/appversion"
	"github.com/tryhisar/hisar/server/internal/desktopupdate"
)

const (
	releaseTagPrefix = "server-"
	refreshInterval  = time.Hour
	refreshTimeout   = time.Minute
)

// Source holds the newest server release it read from GitHub.
type Source struct {
	gitHub        *desktopupdate.GitHub
	latestVersion atomic.Pointer[appversion.Version]
}

// New returns nil when config.Repository is desktopupdate.Off.
func New(config desktopupdate.GitHubConfig) (*Source, error) {
	if config.Repository == desktopupdate.Off {
		return nil, nil
	}
	gitHub, err := desktopupdate.NewGitHub(config)
	if err != nil {
		return nil, err
	}
	return &Source{gitHub: gitHub}, nil
}

// Run reads the releases now and then once an hour until ctx ends. A failed read keeps the version read before.
func (source *Source) Run(ctx context.Context) {
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()
	for {
		if err := source.refresh(ctx); err != nil && ctx.Err() == nil {
			log.Printf("server updates: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (source *Source) refresh(ctx context.Context) error {
	refreshContext, cancel := context.WithTimeout(ctx, refreshTimeout)
	defer cancel()
	tags, err := source.gitHub.ListPublishedTags(refreshContext)
	if err != nil {
		return err
	}
	source.latestVersion.Store(selectLatestVersion(tags))
	return nil
}

// selectLatestVersion picks the highest server-MAJOR.MINOR.PATCH tag, or nil; custom builds (server-1.2.3-label) are skipped.
func selectLatestVersion(tags []string) *appversion.Version {
	var latest *appversion.Version
	for _, tag := range tags {
		text, isServer := strings.CutPrefix(tag, releaseTagPrefix)
		if !isServer {
			continue
		}
		version, err := appversion.Parse(text)
		if err != nil || version.IsPreRelease() {
			continue
		}
		if latest == nil || appversion.Compare(version, *latest) > 0 {
			latest = &version
		}
	}
	return latest
}

// GetNewerVersion returns the newest release when it is newer than current, and "" otherwise, before the first read, or
// when current is not a version (a development build).
func (source *Source) GetNewerVersion(current string) string {
	latest := source.latestVersion.Load()
	if latest == nil {
		return ""
	}
	currentVersion, err := appversion.Parse(current)
	if err != nil || appversion.Compare(currentVersion, *latest) >= 0 {
		return ""
	}
	return latest.String()
}
