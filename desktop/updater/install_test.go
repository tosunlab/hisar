package updater

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/tryhisar/hisar/core/app"
	"github.com/tryhisar/hisar/core/crypt"
)

type fakeRelease struct {
	privateKey ed25519.PrivateKey
	files      map[string][]byte
	server     *httptest.Server
}

// startFakeRelease serves desktop-0.10.1's files as GitHub's release downloads do, each package with its .sig.
func startFakeRelease(t *testing.T) *fakeRelease {
	t.Helper()
	_, privateKey, _ := ed25519.GenerateKey(nil)
	release := &fakeRelease{privateKey: privateKey, files: map[string][]byte{}}
	release.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		content, isFound := release.files[request.URL.Path]
		if !isFound {
			http.NotFound(writer, request)
			return
		}
		writer.Write(content)
	}))
	t.Cleanup(release.server.Close)
	return release
}

func (release *fakeRelease) publish(fileName string, content []byte) {
	signature := crypt.SignDesktopPackage(release.privateKey, fileName, sha256.Sum256(content))
	release.files["/desktop-0.10.1/"+fileName] = content
	release.files["/desktop-0.10.1/"+fileName+".sig"] = []byte(base64.StdEncoding.EncodeToString(signature) + "\n")
}

// createNotifier is a 0.3.0 build that found 0.10.1 and downloads from the fake release.
func (release *fakeRelease) createNotifier(installation Installation) *Notifier {
	notifier := NewNotifier("0.3.0", installation)
	notifier.packages = packageSource{
		client:      release.server.Client(),
		publicKey:   release.privateKey.Public().(ed25519.PublicKey),
		downloadURL: release.server.URL + "/",
	}
	notifier.available = app.AvailableUpdate{Version: "0.10.1", HasChecked: true}
	return notifier
}

// An AppImage update replaces the file the user runs, executable, and starts it once this copy quits.
func TestAnAppImageUpdateReplacesTheAppImageAndStartsIt(t *testing.T) {
	release := startFakeRelease(t)
	release.publish("Hisar-0.10.1-linux-amd64.AppImage", []byte("new AppImage"))
	appImagePath := filepath.Join(t.TempDir(), "Hisar.AppImage")
	os.WriteFile(appImagePath, []byte("old AppImage"), 0o755)
	var relaunchedPath string
	isQuit := false
	notifier := release.createNotifier(Installation{
		Kind: InstallAppImage, Architecture: "amd64", AppImagePath: appImagePath,
		Relaunch: func(path string) error { relaunchedPath = path; return nil },
		Quit:     func() { isQuit = true },
	})
	if kind := notifier.GetInstallKind(); kind != "appimage" {
		t.Fatalf("install kind = %q", kind)
	}
	if err := notifier.InstallUpdate(); err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(appImagePath)
	fileInfo, _ := os.Stat(appImagePath)
	if string(content) != "new AppImage" || fileInfo.Mode().Perm() != 0o755 || relaunchedPath != appImagePath || !isQuit {
		t.Fatalf("AppImage %q mode %o, relaunched %q, quit %v", content, fileInfo.Mode().Perm(), relaunchedPath, isQuit)
	}
	if entries, _ := os.ReadDir(filepath.Dir(appImagePath)); len(entries) != 1 {
		t.Errorf("left beside the AppImage: %v", entries)
	}
}

// A package whose content or signature is not the release's is never installed, and nothing of it stays behind.
func TestAnUpdateWithoutHisarsSignatureIsNotInstalled(t *testing.T) {
	release := startFakeRelease(t)
	release.publish("Hisar-0.10.1-linux-amd64.AppImage", []byte("new AppImage"))
	release.files["/desktop-0.10.1/Hisar-0.10.1-linux-amd64.AppImage"] = []byte("tampered AppImage")
	release.publish("Hisar-0.10.1-linux-amd64.deb", []byte("a package"))
	release.files["/desktop-0.10.1/Hisar-0.10.1-linux-amd64.deb.sig"] = release.files["/desktop-0.10.1/Hisar-0.10.1-linux-amd64.AppImage.sig"]
	directory := t.TempDir()
	appImagePath := filepath.Join(directory, "Hisar.AppImage")
	os.WriteFile(appImagePath, []byte("old AppImage"), 0o755)
	isStarted := false
	for name, installation := range map[string]Installation{
		"tampered content": {Kind: InstallAppImage, Architecture: "amd64", AppImagePath: appImagePath,
			Relaunch: func(string) error { isStarted = true; return nil }, Quit: func() { isStarted = true }},
		"another file's signature": {Kind: InstallDeb, Architecture: "amd64", DownloadDirectory: filepath.Join(directory, "updates"),
			Open: func(string) error { isStarted = true; return nil }},
		"no release for this architecture": {Kind: InstallRPM, Architecture: "arm64", DownloadDirectory: filepath.Join(directory, "updates"),
			Open: func(string) error { isStarted = true; return nil }},
	} {
		if err := release.createNotifier(installation).InstallUpdate(); err == nil {
			t.Errorf("%s: installed", name)
		}
	}
	if content, _ := os.ReadFile(appImagePath); string(content) != "old AppImage" || isStarted {
		t.Fatalf("AppImage %q, started %v", content, isStarted)
	}
	if entries, _ := os.ReadDir(directory); len(entries) > 2 {
		t.Errorf("left behind: %v", entries)
	}
	if entries, _ := os.ReadDir(filepath.Join(directory, "updates")); len(entries) != 0 {
		t.Errorf("downloads kept: %v", entries)
	}
}

// A .deb, .rpm or Flatpak bundle is kept in the download folder and handed to the system's software installer.
func TestAPackageUpdateOpensInTheSoftwareInstaller(t *testing.T) {
	release := startFakeRelease(t)
	for _, test := range []struct {
		kind     InstallKind
		fileName string
	}{{InstallDeb, "Hisar-0.10.1-linux-arm64.deb"}, {InstallRPM, "Hisar-0.10.1-linux-arm64.rpm"}, {InstallFlatpak, "Hisar-0.10.1-linux-arm64.flatpak"}} {
		release.publish(test.fileName, []byte(test.fileName))
		downloadDirectory := filepath.Join(t.TempDir(), "updates")
		var openedPath string
		notifier := release.createNotifier(Installation{Kind: test.kind, Architecture: "arm64", DownloadDirectory: downloadDirectory,
			Open: func(path string) error { openedPath = path; return nil }})
		if err := notifier.InstallUpdate(); err != nil {
			t.Fatalf("%s: %v", test.kind, err)
		}
		if content, _ := os.ReadFile(openedPath); openedPath != filepath.Join(downloadDirectory, test.fileName) || string(content) != test.fileName {
			t.Fatalf("%s: opened %q holding %q", test.kind, openedPath, content)
		}
	}
}

func TestOnlyAFoundUpdateFromAPackageInstalls(t *testing.T) {
	release := startFakeRelease(t)
	notifier := release.createNotifier(Installation{})
	if err := notifier.InstallUpdate(); err == nil {
		t.Error("a copy outside a package installed an update")
	}
	notifier = release.createNotifier(Installation{Kind: InstallDeb, Architecture: "amd64"})
	notifier.available = app.AvailableUpdate{HasChecked: true}
	if err := notifier.InstallUpdate(); err == nil {
		t.Error("installed without a newer version")
	}
}

// The Linux app checks packages with the key the Mac's Sparkle updates are signed with.
func TestUpdatePublicKeyIsSparklesKey(t *testing.T) {
	infoPlist, err := os.ReadFile(filepath.Join("..", "build", "darwin", "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`<key>SUPublicEDKey</key>\s*<string>([^<]+)</string>`).FindSubmatch(infoPlist)
	if match == nil || string(match[1]) != updatePublicKey {
		t.Fatalf("SUPublicEDKey %q, updatePublicKey %q", match, updatePublicKey)
	}
}
