package desktopupdate

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const metadataFileName = "update.json"

var (
	versionPattern  = regexp.MustCompile(`^(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,2})$`)
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

	allFileSizeByName map[string]int64
}

// GetBuildNumber maps MAJOR.MINOR.PATCH to MAJOR*1000000 + MINOR*1000 + PATCH, the app's CFBundleVersion.
func GetBuildNumber(version string) (int64, error) {
	allPart := versionPattern.FindStringSubmatch(version)
	if allPart == nil {
		return 0, fmt.Errorf("version %q is not MAJOR.MINOR.PATCH with parts up to 999", version)
	}
	var buildNumber int64
	for _, part := range allPart[1:] {
		number, _ := strconv.ParseInt(part, 10, 64)
		buildNumber = buildNumber*1000 + number
	}
	return buildNumber, nil
}

func IsValidVersion(version string) bool {
	return versionPattern.MatchString(version)
}

func IsValidFileName(fileName string) bool {
	return fileNamePattern.MatchString(fileName) && !strings.Contains(fileName, "..")
}

func parseRelease(content []byte, version string, allFileSizeByName map[string]int64) (*Release, error) {
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
	if size, found := allFileSizeByName[release.Archive]; !found {
		return nil, fmt.Errorf("archive %s is not in the package", release.Archive)
	} else if size != release.Length {
		return nil, fmt.Errorf("archive %s has %d bytes, update.json says %d", release.Archive, size, release.Length)
	}
	release.allFileSizeByName = allFileSizeByName
	return release, nil
}
