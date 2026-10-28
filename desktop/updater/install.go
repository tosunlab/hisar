package updater

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/project"
)

// InstallKind is how this copy of Hisar installs an update: the package it came from.
type InstallKind string

const (
	// InstallNone is a copy that cannot install updates (a development build, or a binary run from anywhere); the
	// user downloads the new version from its release.
	InstallNone     InstallKind = ""
	InstallAppImage InstallKind = "appimage"
	InstallDeb      InstallKind = "deb"
	InstallRPM      InstallKind = "rpm"
	InstallFlatpak  InstallKind = "flatpak"
)

// updatePublicKey is SUPublicEDKey in build/darwin/Info.plist: the key the release signs the Mac's Sparkle updates
// and each Linux package with (hisar-update-key sign).
const updatePublicKey = "V2E8SipUAka3AgY4qXZEvH2nZ9tWAJfMqAK+fFxrk7c="

const (
	packageDownloadTimeout = 15 * time.Minute
	maximumPackageSize     = 512 << 20
	maximumSignatureSize   = 1 << 10
)

// Installation is this copy of Hisar as an update replaces it.
type Installation struct {
	Kind InstallKind
	// Architecture is the release's name for this machine (amd64, arm64).
	Architecture string
	// AppImagePath is the AppImage file an update replaces.
	AppImagePath string
	// DownloadDirectory keeps a downloaded .deb, .rpm or Flatpak bundle for the system's software installer.
	DownloadDirectory string
	// Open hands a downloaded package to the system's software installer.
	Open func(path string) error
	// Relaunch starts the replaced AppImage once this process has exited, and Quit ends this process.
	Relaunch func(appImagePath string) error
	Quit     func()
}

// getPackageName is the release file of the version for this installation, as scripts/package-desktop-linux.sh and
// scripts/package-desktop-flatpak.sh name it.
func (installation Installation) getPackageName(version string) string {
	extension := map[InstallKind]string{InstallAppImage: "AppImage", InstallDeb: "deb", InstallRPM: "rpm", InstallFlatpak: "flatpak"}[installation.Kind]
	return fmt.Sprintf("Hisar-%s-linux-%s.%s", version, installation.Architecture, extension)
}

// packageSource is where the releases' packages are, and the key their signatures must check out with.
type packageSource struct {
	client    *http.Client
	publicKey []byte
	// downloadURL holds a version's files under desktop-<version>/: GitHub's release downloads.
	downloadURL string
}

func newPackageSource(publicKey []byte) packageSource {
	return packageSource{client: newDownloadClient(), publicKey: publicKey, downloadURL: project.GitHubURL + "/releases/download/"}
}

func (source packageSource) buildURL(version, fileName string) string {
	return source.downloadURL + "desktop-" + version + "/" + fileName
}

// newDownloadClient follows GitHub's redirect to its download host, over https only.
func newDownloadClient() *http.Client {
	return &http.Client{Timeout: packageDownloadTimeout, CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		if request.URL.Scheme != "https" {
			return errors.New("the download left https")
		}
		return nil
	}}
}

// download writes url into path, at most limit bytes, and returns the file's SHA-256.
func download(client *http.Client, url, path string, limit int64) ([sha256.Size]byte, error) {
	var digest [sha256.Size]byte
	response, err := client.Get(url)
	if err != nil {
		return digest, fmt.Errorf("could not download the update: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return digest, fmt.Errorf("the update's download answered %s", response.Status)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return digest, err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, limit+1))
	closeErr := file.Close()
	switch {
	case copyErr != nil:
		return digest, fmt.Errorf("could not download the update: %w", copyErr)
	case closeErr != nil:
		return digest, closeErr
	case written > limit:
		return digest, errors.New("the update is larger than any Hisar release")
	}
	hash.Sum(digest[:0])
	return digest, nil
}

func readSignature(client *http.Client, url string) ([]byte, error) {
	response, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("could not download the update's signature: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the update's signature answered %s", response.Status)
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, maximumSignatureSize))
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(strings.TrimSpace(string(encoded)))
}

// fetchVerifiedPackage downloads the version's package into path and keeps it only if the release's signature over
// its name and content checks out with the update key this app was built with.
func fetchVerifiedPackage(source packageSource, version, fileName, path string) error {
	digest, err := download(source.client, source.buildURL(version, fileName), path, maximumPackageSize)
	if err == nil {
		var signature []byte
		if signature, err = readSignature(source.client, source.buildURL(version, fileName+".sig")); err == nil {
			if crypt.VerifyDesktopPackage(source.publicKey, fileName, digest, signature) != nil {
				err = errors.New("the downloaded update is not signed by Hisar; it was not installed")
			}
		}
	}
	if err != nil {
		os.Remove(path)
	}
	return err
}

// install downloads and verifies the version, then installs it the way this installation can: an AppImage replaces
// its file and starts again; a .deb, .rpm or Flatpak bundle goes to the system's software installer, which asks for
// the administrator's password where it needs one.
func (installation Installation) install(source packageSource, version string) error {
	fileName := installation.getPackageName(version)
	switch installation.Kind {
	case InstallAppImage:
		// Beside the AppImage, so the rename that replaces it stays on one file system; the running copy keeps its
		// mount of the old file.
		partialPath := filepath.Join(filepath.Dir(installation.AppImagePath), "."+filepath.Base(installation.AppImagePath)+".update")
		if err := fetchVerifiedPackage(source, version, fileName, partialPath); err != nil {
			return err
		}
		if err := os.Chmod(partialPath, 0o755); err != nil {
			os.Remove(partialPath)
			return err
		}
		if err := os.Rename(partialPath, installation.AppImagePath); err != nil {
			os.Remove(partialPath)
			return fmt.Errorf("could not replace %s: %w", installation.AppImagePath, err)
		}
		if err := installation.Relaunch(installation.AppImagePath); err != nil {
			return fmt.Errorf("Hisar %s is installed; open Hisar again to use it (%w)", version, err)
		}
		installation.Quit()
		return nil
	case InstallDeb, InstallRPM, InstallFlatpak:
		if err := os.MkdirAll(installation.DownloadDirectory, 0o700); err != nil {
			return err
		}
		path := filepath.Join(installation.DownloadDirectory, fileName)
		partialPath := path + ".partial"
		if err := fetchVerifiedPackage(source, version, fileName, partialPath); err != nil {
			return err
		}
		if err := os.Rename(partialPath, path); err != nil {
			os.Remove(partialPath)
			return err
		}
		if err := installation.Open(path); err != nil {
			return fmt.Errorf("Hisar %s is downloaded to %s; install it with your software installer (%w)", version, path, err)
		}
		return nil
	}
	return errors.New("this copy of Hisar cannot install updates; download the new version from its release")
}
