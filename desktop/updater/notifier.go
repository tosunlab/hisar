package updater

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/app"
	"github.com/tryhisar/hisar/core/appversion"
	"github.com/tryhisar/hisar/core/project"
)

const (
	automaticCheckInterval = 24 * time.Hour
	firstAutomaticDelay    = time.Minute
	feedRequestTimeout     = 30 * time.Second
	maximumFeedSize        = 4 << 20
)

var errSignedOut = errors.New("sign in to check for updates")

// stampedVersion is the release version the Linux and Windows packaging scripts set with -X.
var stampedVersion string

// GetStampedVersion is the version a Linux or Windows build was packaged as; empty in a development build.
func GetStampedVersion() string { return stampedVersion }

// Notifier checks the signed-in server's update feed, the same appcast Sparkle reads on the Mac, and finds a newer
// version for the user to download from its GitHub release; it never installs anything.
type Notifier struct {
	currentVersion string
	client         *http.Client
	mutex          sync.Mutex
	feedURL        string
	authorization  string
	available      app.AvailableUpdate
	stopAutomatic  context.CancelFunc
}

func NewNotifier(currentVersion string) *Notifier {
	return &Notifier{currentVersion: currentVersion, client: &http.Client{Timeout: feedRequestTimeout, CheckRedirect: apiclient.CheckRedirect}}
}

// IsAvailable is false in a development build, which has no release version to compare.
func (notifier *Notifier) IsAvailable() bool {
	_, err := appversion.Parse(notifier.currentVersion)
	return err == nil
}

func (notifier *Notifier) GetVersion() (string, string) {
	if notifier.currentVersion == "" {
		return developmentVersion, ""
	}
	return notifier.currentVersion, ""
}

func (notifier *Notifier) SetSession(feedURL, authorization string) {
	notifier.mutex.Lock()
	defer notifier.mutex.Unlock()
	notifier.feedURL, notifier.authorization = feedURL, authorization
}

// SetAutomaticCheck checks a minute after it is turned on, then once a day.
func (notifier *Notifier) SetAutomaticCheck(isEnabled bool) {
	notifier.mutex.Lock()
	defer notifier.mutex.Unlock()
	if notifier.stopAutomatic != nil {
		notifier.stopAutomatic()
		notifier.stopAutomatic = nil
	}
	if !isEnabled || !notifier.IsAvailable() {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	notifier.stopAutomatic = cancel
	go func() {
		timer := time.NewTimer(firstAutomaticDelay)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				notifier.CheckForUpdates()
				timer.Reset(automaticCheckInterval)
			}
		}
	}()
}

func (notifier *Notifier) GetAvailableUpdate() app.AvailableUpdate {
	notifier.mutex.Lock()
	defer notifier.mutex.Unlock()
	return notifier.available
}

// CheckForUpdates reads the feed now; the result is in GetAvailableUpdate.
func (notifier *Notifier) CheckForUpdates() error {
	notifier.mutex.Lock()
	feedURL, authorization := notifier.feedURL, notifier.authorization
	notifier.mutex.Unlock()
	if feedURL == "" {
		return errSignedOut
	}
	feed, err := notifier.readFeed(feedURL, authorization)
	if err != nil {
		return err
	}
	newest, err := findNewerVersion(feed, notifier.currentVersion)
	if err != nil {
		return err
	}
	available := app.AvailableUpdate{HasChecked: true}
	if newest != "" {
		available.Version, available.DownloadURL = newest, buildReleaseURL(newest)
	}
	notifier.mutex.Lock()
	notifier.available = available
	notifier.mutex.Unlock()
	return nil
}

func (notifier *Notifier) readFeed(feedURL, authorization string) ([]byte, error) {
	request, err := http.NewRequest(http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", authorization)
	response, err := notifier.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("could not reach the update feed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the update feed answered %s", response.Status)
	}
	return io.ReadAll(io.LimitReader(response.Body, maximumFeedSize))
}

// buildReleaseURL is the GitHub release page that holds every platform's download of a desktop version.
func buildReleaseURL(version string) string {
	return project.GitHubURL + "/releases/tag/desktop-" + version
}

type appcast struct {
	Items []struct {
		ShortVersion string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle shortVersionString"`
	} `xml:"channel>item"`
}

// findNewerVersion returns the newest release in the appcast above currentVersion, or "" when there is none.
func findNewerVersion(feed []byte, currentVersion string) (string, error) {
	current, err := appversion.Parse(currentVersion)
	if err != nil {
		return "", fmt.Errorf("this build has no release version: %w", err)
	}
	var parsed appcast
	if err := xml.Unmarshal(feed, &parsed); err != nil {
		return "", fmt.Errorf("could not read the update feed: %w", err)
	}
	newest, newestText := current, ""
	for _, item := range parsed.Items {
		version, err := appversion.Parse(item.ShortVersion)
		if err != nil || version.IsPreRelease() {
			continue
		}
		if appversion.Compare(version, newest) > 0 {
			newest, newestText = version, item.ShortVersion
		}
	}
	return newestText, nil
}
