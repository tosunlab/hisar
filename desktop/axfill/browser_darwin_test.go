//go:build darwin && !ios && cgo

package axfill

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/signin"
	"github.com/tryhisar/hisar/desktop/appinfo"
)

// browsersVariable lists the browser apps the integration tests drive, comma-separated, such as
// "/Applications/Brave Browser.app,/tmp/Google Chrome.app". Each runs as its own instance with a throwaway profile.
const browsersVariable = "HISAR_TEST_SIGN_IN_BROWSERS"

type site struct {
	Main    string `json:"main"`
	Other   string `json:"other"`
	logPath string
}

type post struct {
	Origin string            `json:"origin"`
	Path   string            `json:"path"`
	Form   map[string]string `json:"form"`
	JSON   json.RawMessage   `json:"json"`
}

func startSite(t *testing.T, mainAddress, otherAddress string) *site {
	t.Helper()
	directory := t.TempDir()
	binary := filepath.Join(directory, "hisar-sign-in-site-fixture")
	build := exec.Command("go", "build", "-o", binary, "./cmd/hisar-sign-in-site-fixture")
	build.Dir = filepath.Join("..", "..", "server")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the site: %v\n%s", err, output)
	}
	logPath := filepath.Join(directory, "posts.jsonl")
	command := exec.Command(binary, "-steer", "-log", logPath, "-main", mainAddress, "-other", otherAddress)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		command.Process.Signal(syscall.SIGTERM)
		command.Wait()
	})
	served := &site{logPath: logPath}
	if err := json.NewDecoder(stdout).Decode(served); err != nil {
		t.Fatalf("reading the site's origins: %v", err)
	}
	return served
}

func (served *site) steer(t *testing.T, pageURL string) {
	t.Helper()
	response, err := http.PostForm(served.Main+"/steer", url.Values{"url": {pageURL}})
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
}

func (served *site) readPosts(t *testing.T) []post {
	t.Helper()
	file, err := os.Open(served.logPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	posts := []post{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var line post
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			t.Fatal(err)
		}
		posts = append(posts, line)
	}
	return posts
}

type browser struct {
	app     string
	profile string
	target  signin.Target
}

func listBrowserApps(t *testing.T) []string {
	t.Helper()
	apps := []string{}
	for _, app := range strings.Split(os.Getenv(browsersVariable), ",") {
		if app = strings.TrimSpace(app); app != "" {
			apps = append(apps, app)
		}
	}
	if len(apps) == 0 {
		t.Skipf("set %s to the browser apps to drive", browsersVariable)
	}
	if !New(appinfo.NewSystemResolver()).HasAccess() {
		t.Skip("this process has no Accessibility permission")
	}
	return apps
}

func launchBrowser(t *testing.T, app, pageURL string, arguments ...string) *browser {
	t.Helper()
	profile, err := os.MkdirTemp("", "hisar-sign-in-profile-")
	if err != nil {
		t.Fatal(err)
	}
	launched := &browser{app: app, profile: profile}
	open := append([]string{"-n", "-g", "-a", app, "--args", "--user-data-dir=" + profile, "--no-first-run",
		"--no-default-browser-check", "--use-mock-keychain"}, arguments...)
	if output, err := exec.Command("open", append(open, pageURL)...).CombinedOutput(); err != nil {
		t.Fatalf("open %s: %v\n%s", app, err, output)
	}
	t.Cleanup(launched.quit)
	deadline := time.Now().Add(30 * time.Second)
	for launched.target.PID == 0 || launched.target.WindowID == 0 {
		if time.Now().After(deadline) {
			t.Fatalf("%s did not open a window", app)
		}
		time.Sleep(200 * time.Millisecond)
		if launched.target.PID == 0 {
			launched.target.PID = findMainProcess(t, profile)
			continue
		}
		if windows, err := listWindowIDs(launched.target.PID); err == nil && len(windows) > 0 {
			launched.target.WindowID = windows[0]
		}
	}
	return launched
}

func findMainProcess(t *testing.T, profile string) int {
	t.Helper()
	output, err := exec.Command("ps", "-axww", "-o", "pid=,args=").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(output), "\n") {
		if strings.Contains(line, "--user-data-dir="+profile) && !strings.Contains(line, "--type=") {
			pid, _ := strconv.Atoi(strings.Fields(line)[0])
			return pid
		}
	}
	return 0
}

func (launched *browser) quit() {
	if launched.target.PID != 0 {
		syscall.Kill(launched.target.PID, syscall.SIGTERM)
		for attempt := 0; attempt < 50 && syscall.Kill(launched.target.PID, 0) == nil; attempt++ {
			time.Sleep(100 * time.Millisecond)
		}
	}
	os.RemoveAll(launched.profile)
}

// waitForPage observes target until its top document is pageURL and isReady accepts the page. Time the window spends
// off the screen, as while the user works in a full-screen app's Space, does not count against the 20 s it may take.
func waitForPage(t *testing.T, driver signin.Driver, target signin.Target, pageURL string, isReady func(signin.Page) bool) signin.Page {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	giveUp := time.Now().Add(5 * time.Minute)
	var last signin.Page
	for time.Now().Before(deadline) && time.Now().Before(giveUp) {
		snapshot, err := driver.Observe(context.Background(), target)
		switch {
		case errors.Is(err, signin.ErrNotVisible):
			deadline = deadline.Add(100 * time.Millisecond)
		case err == nil:
			last = snapshot.Page()
			snapshot.Close()
			if len(last.Frames) > 0 && last.Frames[0].URL == pageURL && isReady(last) {
				return last
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("the window did not show %s: last %+v", pageURL, last.Frames)
	return last
}
