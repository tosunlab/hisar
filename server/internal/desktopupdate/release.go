package desktopupdate

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/appversion"
)

const (
	metadataFileName    = "update.json"
	maximumMetadataSize = 1 << 20
)

var (
	fileNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	systemPattern   = regexp.MustCompile(`^[0-9]{1,2}(\.[0-9]{1,2}){0,2}$`)
)

type Release struct {
	Version              string    `json:"version"`
	BuildNumber          int64     `json:"buildNumber"`
	Archive              string    `json:"archive"`
	EdSignature          string    `json:"edSignature"`
	Length               int64     `json:"length"`
	MinimumSystemVersion string    `json:"minimumSystemVersion"`
	ReleaseNotes         string    `json:"releaseNotes"`
	ReleaseNotesFormat   string    `json:"releaseNotesFormat"`
	PublishedAt          time.Time `json:"publishedAt"`

	allAssetByName map[string]gitHubAsset
}

// GetBuildNumber maps MAJOR.MINOR.PATCH to MAJOR*1000000 + MINOR*1000 + PATCH, the app's CFBundleVersion.
func GetBuildNumber(version string) (int64, error) {
	parsed, err := appversion.Parse(version)
	if err != nil || parsed.IsPreRelease() || parsed.String() != version || max(parsed.Major, parsed.Minor, parsed.Patch) > 999 {
		return 0, fmt.Errorf("version %q is not MAJOR.MINOR.PATCH with parts up to 999", version)
	}
	return int64(parsed.Major*1_000_000 + parsed.Minor*1_000 + parsed.Patch), nil
}

func IsValidVersion(version string) bool {
	_, err := GetBuildNumber(version)
	return err == nil
}

func IsValidFileName(fileName string) bool {
	return fileNamePattern.MatchString(fileName) && !strings.Contains(fileName, "..")
}

func parseRelease(content []byte, version string, allAssetByName map[string]gitHubAsset) (*Release, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	release := &Release{}
	if err := decoder.Decode(release); err != nil {
		return nil, fmt.Errorf("update.json: %w", err)
	}
	buildNumber, err := GetBuildNumber(version)
	if err != nil {
		return nil, err
	}
	switch {
	case release.Version != version:
		return nil, fmt.Errorf("update.json is for version %q", release.Version)
	case release.BuildNumber != buildNumber:
		return nil, fmt.Errorf("build number %d, expected %d", release.BuildNumber, buildNumber)
	case !IsValidFileName(release.Archive) || !strings.HasSuffix(release.Archive, ".zip"):
		return nil, fmt.Errorf("archive %q is not a zip file name", release.Archive)
	case release.Length <= 0:
		return nil, errors.New("the archive length is missing")
	case !systemPattern.MatchString(release.MinimumSystemVersion):
		return nil, fmt.Errorf("minimum macOS version %q is not valid", release.MinimumSystemVersion)
	case release.ReleaseNotesFormat != "markdown" && release.ReleaseNotesFormat != "plain-text":
		return nil, fmt.Errorf("release notes format %q is neither markdown nor plain-text", release.ReleaseNotesFormat)
	case release.PublishedAt.IsZero():
		return nil, errors.New("the publication date is missing")
	}
	if signature, err := base64.StdEncoding.DecodeString(release.EdSignature); err != nil || len(signature) != 64 {
		return nil, errors.New("the EdDSA signature is not 64 bytes of base64")
	}
	if asset, found := allAssetByName[release.Archive]; !found {
		return nil, fmt.Errorf("archive %s is not in the release", release.Archive)
	} else if asset.Size != release.Length {
		return nil, fmt.Errorf("archive %s has %d bytes, update.json says %d", release.Archive, asset.Size, release.Length)
	}
	release.allAssetByName = allAssetByName
	return release, nil
}
