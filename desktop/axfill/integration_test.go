//go:build (darwin && !ios && cgo) || linux

package axfill

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/signin"
	"github.com/tryhisar/hisar/desktop/appinfo"
)

const (
	integrationPassword        = "HISAR-CANARY-3b8e1f0a9d27c645"
	integrationOneTimePassword = "otpauth://totp/Fixture:user?secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP&digits=8"
)

type promptCounter struct {
	signin.Driver
	mutex     sync.Mutex
	dismissed int
}

func (counter *promptCounter) DismissPrompt(ctx context.Context, process signin.ProcessInstance, prompt signin.ElementID) error {
	err := counter.Driver.DismissPrompt(ctx, process, prompt)
	if err == nil {
		counter.mutex.Lock()
		counter.dismissed++
		counter.mutex.Unlock()
	}
	return err
}

type signInScenario struct {
	page       string
	isOther    bool
	reason     signin.Reason
	outcome    signin.Outcome
	cleanup    signin.Cleanup
	steps      int
	finalURL   string
	checkPosts func(t *testing.T, posts []post)
	checkPage  func(t *testing.T, served *site, driver signin.Driver, target signin.Target)
}

func checkForm(path string, expected map[string]string) func(t *testing.T, posts []post) {
	return func(t *testing.T, posts []post) {
		t.Helper()
		index := slices.IndexFunc(posts, func(sent post) bool { return sent.Path == path })
		if index < 0 {
			t.Errorf("the site got no %s: %+v", path, posts)
			return
		}
		for key, value := range expected {
			if posts[index].Form[key] != value {
				t.Errorf("%s %s = %q, want %q", path, key, posts[index].Form[key], value)
			}
		}
	}
}

var signInScenarios = []signInScenario{
	{
		page: "plain", outcome: signin.Submitted, cleanup: signin.CleanupPageLeft, steps: 1, finalURL: "/signed-in",
		checkPosts: checkForm("/submit/plain", map[string]string{"username": "user@example.com", "password": integrationPassword}),
	},
	{
		page: "controlled", outcome: signin.Submitted, cleanup: signin.CleanupFieldsRemoved, steps: 1, finalURL: "/controlled",
		checkPosts: func(t *testing.T, posts []post) {
			var sent struct {
				State, DOM struct{ Username, Password string }
			}
			if len(posts) != 1 || json.Unmarshal(posts[0].JSON, &sent) != nil || sent.State.Password != integrationPassword ||
				sent.DOM.Password != integrationPassword || sent.State.Username != "user@example.com" {
				t.Errorf("the controlled form sent %+v", posts)
			}
		},
	},
	{
		page: "step1", outcome: signin.Submitted, cleanup: signin.CleanupPageLeft, steps: 2, finalURL: "/signed-in",
		checkPosts: checkForm("/submit/step2", map[string]string{"username": "user@example.com", "password": integrationPassword}),
	},
	{
		page: "return", outcome: signin.Submitted, cleanup: signin.CleanupPageLeft, steps: 1, finalURL: "/signed-in",
		checkPosts: checkForm("/submit/return", map[string]string{"username": "user@example.com", "password": integrationPassword}),
	},
	{page: "spa-steps", outcome: signin.Submitted, cleanup: signin.CleanupFieldsRemoved, steps: 2, finalURL: "/spa-steps"},
	{
		page: "totp", outcome: signin.Submitted, cleanup: signin.CleanupPageLeft, steps: 2, finalURL: "/signed-in",
		checkPosts: checkForm("/submit/totp", map[string]string{"password": integrationPassword}),
	},
	{page: "split-totp", outcome: signin.Submitted, cleanup: signin.CleanupPageLeft, steps: 1, finalURL: "/signed-in"},
	{
		page: "wrong", outcome: signin.NotAccepted, cleanup: signin.CleanupFieldsCleared, steps: 1, finalURL: "/wrong",
		checkPosts: func(t *testing.T, posts []post) {
			if len(posts) != 1 || !strings.Contains(string(posts[0].JSON), integrationPassword) {
				t.Errorf("the wrong-password page got %+v, want one post with the password", posts)
			}
		},
		checkPage: checkFieldsWereCleared,
	},
	{
		page: "hidden", outcome: signin.Submitted, cleanup: signin.CleanupFieldsRemoved, steps: 1, finalURL: "/hidden",
		checkPage: checkHiddenFieldsKeptTheirValues,
	},
	{page: "iframe", reason: signin.ReasonCrossOriginFrame},
	{page: "spoof", reason: signin.ReasonNotSecureField},
	{page: "signup", reason: signin.ReasonSignUpForm},
	{page: "phish", isOther: true},
}

// TestIntegrationSignIn signs in to the fixture site through axfill and signin.Signer in each browser of
// HISAR_TEST_SIGN_IN_BROWSERS, each a new instance with a throwaway profile in the background.
func TestIntegrationSignIn(t *testing.T) {
	apps := listBrowsers(t)
	served := startSite(t, "127.0.0.1:0", "127.0.0.1:0")
	login := signin.Login{
		Username: "user@example.com", Password: signin.NewSecret(integrationPassword),
		OneTimePassword: signin.NewSecret(integrationOneTimePassword),
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, app := range apps {
		counter := &promptCounter{Driver: New(appinfo.NewSystemResolver())}
		signer := signin.NewSigner(counter, home, time.Now)
		launched := launchBrowser(t, app, served.Main+"/signed-in")
		waitForPage(t, counter, launched.target, served.Main+"/signed-in", func(signin.Page) bool { return true })
		for _, scenario := range signInScenarios {
			origin := served.Main
			if scenario.isOther {
				origin = served.Other
			}
			if scenario.isOther {
				served.steer(t, origin+"/"+scenario.page)
				waitForPage(t, counter, launched.target, origin+"/"+scenario.page, func(page signin.Page) bool { return len(page.Fields) > 0 })
				destination, err := signer.Inspect(context.Background(), launched.target)
				if err != nil || destination.Origin.String() != served.Other {
					t.Errorf("%s: %+v, %v; core/app matches logins against the origin Inspect read", scenario.page, destination, err)
				}
				t.Logf("%s %s: Inspect read %s", launched.name(), scenario.page, destination.Origin)
				continue
			}
			var run scenarioRun
			for attempt := 1; ; attempt++ {
				run = runScenario(t, served, counter, signer, launched, origin+"/"+scenario.page, login)
				if !run.watch.isDisturbed() || attempt == 5 {
					break
				}
				t.Logf("%s %s: the user switched windows during the sign-in (%s, %s); again", launched.name(), scenario.page, run.result.Outcome, run.watch)
			}
			encoded, _ := json.Marshal(run.result)
			if run.err != nil && strings.Contains(run.err.Error(), integrationPassword) || strings.Contains(string(encoded), integrationPassword) {
				t.Fatalf("%s: the password reached the result", scenario.page)
			}
			t.Logf("%s %s: %s, err %v, %d posts, %v, %s (browser %d)", launched.name(), scenario.page, encoded, run.err, len(run.posts),
				run.elapsed.Round(time.Millisecond), run.watch, launched.target.PID)
			run.watch.check(t, scenario.page, launched)
			if scenario.reason != "" {
				var refusal *signin.Refusal
				if !errors.As(run.err, &refusal) || refusal.Reason != scenario.reason || len(run.posts) != 0 {
					t.Errorf("%s: %v with %d posts, want a %s refusal", scenario.page, run.err, len(run.posts), scenario.reason)
				}
				continue
			}
			if run.err != nil || run.result.Outcome != scenario.outcome || run.result.Cleanup != scenario.cleanup || len(run.result.Steps) != scenario.steps {
				t.Errorf("%s: %+v, %v", scenario.page, run.result, run.err)
			}
			if scenario.checkPosts != nil {
				scenario.checkPosts(t, run.posts)
			}
			waitForPage(t, counter, launched.target, origin+scenario.finalURL, func(signin.Page) bool { return true })
			checkNoPasswordPrompt(t, counter, launched.target)
			if scenario.checkPage != nil {
				scenario.checkPage(t, served, counter, launched.target)
			}
		}
		counter.mutex.Lock()
		t.Logf("%s: pressed the close control of a save-password prompt %d times", launched.name(), counter.dismissed)
		counter.mutex.Unlock()
		checkAHungBrowserCannotHoldTheDriver(t, served, counter, launched)
		launched.quit()
	}
	for _, app := range apps {
		debuggable := launchBrowser(t, app, served.Main+"/plain", "--remote-debugging-port=0")
		driver := New(appinfo.NewSystemResolver())
		waitForPage(t, driver, debuggable.target, served.Main+"/plain", func(page signin.Page) bool { return len(page.Fields) > 0 })
		_, err := signin.NewSigner(driver, home, time.Now).Inspect(context.Background(), debuggable.target)
		var refusal *signin.Refusal
		if !errors.As(err, &refusal) || refusal.Reason != signin.ReasonDebuggable {
			t.Errorf("%s with remote debugging: %v", debuggable.name(), err)
		}
		t.Logf("%s with --remote-debugging-port=0: %v", debuggable.name(), err)
	}
}

type scenarioRun struct {
	result  signin.Result
	err     error
	posts   []post
	elapsed time.Duration
	watch   screenWatch
}

func runScenario(t *testing.T, served *site, driver signin.Driver, signer *signin.Signer, launched *browser, pageURL string, login signin.Login) scenarioRun {
	t.Helper()
	served.steer(t, pageURL)
	waitForPage(t, driver, launched.target, pageURL, func(page signin.Page) bool { return len(page.Fields) > 0 })
	postsBefore := len(served.readPosts(t))
	stopWatching := watchScreen(t, launched.target)
	started := time.Now()
	result, err := runSignIn(signer, launched.target, login)
	run := scenarioRun{result: result, err: err, elapsed: time.Since(started)}
	run.watch = stopWatching()
	run.posts = served.readPosts(t)[postsBefore:]
	return run
}

func runSignIn(signer *signin.Signer, target signin.Target, login signin.Login) (signin.Result, error) {
	destination, err := signer.Inspect(context.Background(), target)
	if err != nil {
		return signin.Result{}, err
	}
	return signer.Run(context.Background(), destination, func(context.Context) (signin.Login, error) { return login, nil })
}

func checkNoPasswordPrompt(t *testing.T, driver signin.Driver, target signin.Target) {
	t.Helper()
	snapshot, err := driver.Observe(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	process := signin.ProcessInstance{PID: snapshot.Page().Process.PID, StartTime: snapshot.Page().Process.StartTime}
	snapshot.Close()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		prompts, err := driver.ListPrompts(context.Background(), process)
		if err != nil {
			t.Fatal(err)
		}
		for _, prompt := range prompts {
			if prompt.SecureFieldCount > 0 || strings.Contains(strings.ToLower(prompt.Title), "password") {
				t.Errorf("the browser still offers to save the password: %+v", prompt)
				return
			}
		}
	}
}

// checkFieldsWereCleared submits the page again, as a user would, and reads what its fields held: the post shows the
// page's own state, where an accessibility read of a background window shows nothing.
func checkFieldsWereCleared(t *testing.T, served *site, driver signin.Driver, target signin.Target) {
	t.Helper()
	postsBefore := len(served.readPosts(t))
	snapshot, err := driver.Observe(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	buttons := snapshot.Page().Buttons
	if len(buttons) == 0 {
		t.Fatal("the wrong-password page has no button")
	}
	if err := snapshot.Press(buttons[0].ID); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if posts := served.readPosts(t)[postsBefore:]; len(posts) > 0 {
			if !strings.Contains(string(posts[0].JSON), `"password":""`) || !strings.Contains(string(posts[0].JSON), `"username":""`) {
				t.Errorf("the page still held entries after not-accepted: %s", posts[0].JSON)
			}
			return
		}
	}
	t.Error("the second submit never reached the site")
}

func checkAHungBrowserCannotHoldTheDriver(t *testing.T, served *site, driver signin.Driver, launched *browser) {
	t.Helper()
	served.steer(t, served.Main+"/plain")
	page := waitForPage(t, driver, launched.target, served.Main+"/plain", func(page signin.Page) bool { return len(page.Fields) > 0 })
	snapshot, err := driver.Observe(context.Background(), launched.target)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	process := signin.ProcessInstance{PID: page.Process.PID, StartTime: page.Process.StartTime}
	if err := syscall.Kill(launched.target.PID, syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	defer syscall.Kill(launched.target.PID, syscall.SIGCONT)
	const slack = 500 * time.Millisecond
	for name, call := range map[string]struct {
		bound time.Duration
		run   func(ctx context.Context) error
	}{
		"Observe": {time.Second, func(ctx context.Context) error {
			_, err := driver.Observe(ctx, launched.target)
			return err
		}},
		"ListPrompts": {time.Second, func(ctx context.Context) error {
			_, err := driver.ListPrompts(ctx, process)
			return err
		}},
		"Clear without a deadline": {operationTimeout, func(context.Context) error {
			return snapshot.Clear(page.Fields[0].ID)
		}},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), call.bound)
		started := time.Now()
		err := call.run(ctx)
		elapsed := time.Since(started)
		cancel()
		t.Logf("%s, stopped: %s returned %v after %v", launched.name(), name, err, elapsed.Round(time.Millisecond))
		if err == nil || elapsed > call.bound+slack {
			t.Errorf("%s on a stopped browser: %v after %v, want an error within %v", name, err, elapsed, call.bound)
		}
	}
}

func checkHiddenFieldsKeptTheirValues(t *testing.T, served *site, driver signin.Driver, target signin.Target) {
	t.Helper()
	postsBefore := len(served.readPosts(t))
	snapshot, err := driver.Observe(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	index := slices.IndexFunc(snapshot.Page().Buttons, func(button signin.Button) bool { return button.Title == "Report" })
	if index < 0 {
		t.Fatalf("the hidden page shows no Report button: %+v", snapshot.Page().Buttons)
	}
	if err := snapshot.Press(snapshot.Page().Buttons[index].ID); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if posts := served.readPosts(t)[postsBefore:]; len(posts) > 0 {
			var kept struct{ UsernameLength, PasswordLength int }
			if json.Unmarshal(posts[0].JSON, &kept) != nil || kept.PasswordLength != len(integrationPassword) {
				t.Errorf("the hidden password field reported %s, want it to keep the password", posts[0].JSON)
			}
			t.Logf("the hidden fields still hold %d and %d characters", kept.UsernameLength, kept.PasswordLength)
			return
		}
	}
	t.Error("the report never reached the site")
}

// browsersVariable lists the browsers the integration tests drive, comma-separated: apps on the Mac, such as
// "/Applications/Brave Browser.app,/tmp/Google Chrome.app", and executables on Linux, such as
// "/usr/lib/chromium/chromium,/opt/google/chrome/chrome". Each runs as its own instance with a throwaway profile.
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

func listBrowsers(t *testing.T) []string {
	t.Helper()
	apps := []string{}
	for _, app := range strings.Split(os.Getenv(browsersVariable), ",") {
		if app = strings.TrimSpace(app); app != "" {
			apps = append(apps, app)
		}
	}
	if len(apps) == 0 {
		t.Skipf("set %s to the browsers to drive", browsersVariable)
	}
	if !New(appinfo.NewSystemResolver()).HasAccess() {
		t.Skip("this process cannot read browser windows: no Accessibility permission, or no X11 display and accessibility bus")
	}
	return apps
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

var update = flag.Bool("update", false, "write the recorded pages to core/signin/testdata/pages")

// recordingAddresses are fixed so the recorded pages' URLs do not change between recordings.
const recordingMainAddress, recordingOtherAddress = "127.0.0.1:47811", "127.0.0.1:47812"

type recordedPage struct {
	name      string
	path      string
	isOther   bool
	minFields int
	minFrames int
}

var recordedPages = []recordedPage{
	{name: "plain", path: "/plain", minFields: 2},
	{name: "controlled", path: "/controlled", minFields: 2},
	{name: "iframe", path: "/iframe", minFields: 2, minFrames: 2},
	{name: "srcdoc", path: "/srcdoc", minFields: 2, minFrames: 2},
	{name: "step1", path: "/step1", minFields: 1},
	{name: "step2", path: "/step2?username=user%40example.com", minFields: 1},
	{name: "spa-steps", path: "/spa-steps", minFields: 1},
	{name: "totp", path: "/totp", minFields: 2},
	{name: "totp-code", path: "/totp-code", minFields: 1},
	{name: "split-totp", path: "/split-totp", minFields: 8},
	{name: "spoof", path: "/spoof", minFields: 2},
	{name: "signup", path: "/signup", minFields: 3},
	{name: "newsletter", path: "/newsletter", minFields: 1},
	{name: "wrong", path: "/wrong", minFields: 2},
	{name: "invisible", path: "/invisible", minFields: 3},
	{name: "phish", path: "/phish", isOther: true, minFields: 2},
	{name: "return", path: "/return", minFields: 2},
	{name: "signed-in", path: "/signed-in"},
}

// TestIntegrationRecordPages reads every fixture page in each browser and compares it with
// core/signin/testdata/pages/<browser>/<page>.json; with -update it writes them instead.
func TestIntegrationRecordPages(t *testing.T) {
	apps := listBrowsers(t)
	served := startSite(t, recordingMainAddress, recordingOtherAddress)
	driver := New(appinfo.NewSystemResolver())
	for _, app := range apps {
		launched := launchBrowser(t, app, served.Main+"/signed-in")
		waitForPage(t, driver, launched.target, served.Main+"/signed-in", func(signin.Page) bool { return true })
		var browserID string
		for _, page := range recordedPages {
			pageURL := served.Main + page.path
			if page.isOther {
				pageURL = served.Other + page.path
			}
			served.steer(t, pageURL)
			recorded := waitForPage(t, driver, launched.target, pageURL, func(read signin.Page) bool {
				return len(read.Fields) >= page.minFields && len(read.Frames) >= max(page.minFrames, 1)
			})
			browserID = recordedBrowserID(recorded.Process)
			if browserID == "" {
				t.Fatalf("%s is %+v, which is not a known browser", app, recorded.Process)
			}
			encoded, err := json.MarshalIndent(normalizePage(recorded), "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join("..", "..", "core", "signin", "testdata", "pages", browserID, page.name+".json")
			if *update {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
					t.Fatal(err)
				}
				continue
			}
			stored, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(bytes.TrimSpace(stored), encoded) {
				t.Errorf("%s in %s differs from %s (%v); rerun with -update to record it:\n%s", page.name, browserID, path, err, encoded)
			}
		}
		t.Logf("recorded %d pages in %s", len(recordedPages), browserID)
	}
}

// normalizePage keeps what core/signin reads from a page and numbers its documents and elements from 1, so a recording
// changes only when the browser shows the page differently.
func normalizePage(page signin.Page) signin.Page {
	normalized := signin.Page{Process: normalizeProcess(page.Process)}
	next := uint64(0)
	number := func() uint64 {
		next++
		return next
	}
	for _, frame := range page.Frames {
		frame.Document = signin.DocumentID(number())
		normalized.Frames = append(normalized.Frames, frame)
	}
	for _, field := range page.Fields {
		field.ID = signin.ElementID(number())
		normalized.Fields = append(normalized.Fields, field)
	}
	for _, button := range page.Buttons {
		button.ID = signin.ElementID(number())
		normalized.Buttons = append(normalized.Buttons, button)
	}
	return normalized
}
