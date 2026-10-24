//go:build darwin && !ios && cgo

package axfill

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"slices"
	"strconv"
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

func watchScreen(t *testing.T, target signin.Target) (stop func() (frontmost []int, wasHidden bool)) {
	t.Helper()
	done := make(chan struct{})
	finished := make(chan struct{})
	frontmost := []int{readFrontmostPID(t)}
	wasHidden := false
	go func() {
		defer close(finished)
		for {
			select {
			case <-done:
				return
			case <-time.After(25 * time.Millisecond):
			}
			if pid := readFrontmostPID(t); pid != frontmost[len(frontmost)-1] {
				frontmost = append(frontmost, pid)
			}
			if windows, err := listWindowIDs(target.PID); err == nil && !slices.Contains(windows, target.WindowID) {
				wasHidden = true
			}
		}
	}()
	return func() ([]int, bool) {
		close(done)
		<-finished
		return frontmost, wasHidden
	}
}

func readFrontmostPID(t *testing.T) int {
	t.Helper()
	front, err := exec.Command("lsappinfo", "front").Output()
	if err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("lsappinfo", "info", "-only", "pid", strings.TrimSpace(string(front))).Output()
	if err != nil {
		t.Fatal(err)
	}
	_, value, _ := strings.Cut(strings.TrimSpace(string(output)), "=")
	pid, _ := strconv.Atoi(value)
	return pid
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
	apps := listBrowserApps(t)
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
				if !run.isDisturbed() || attempt == 5 {
					break
				}
				t.Logf("%s %s: the user switched apps or Spaces during the sign-in (%s, frontmost apps %v, window hidden %v); again",
					launched.name(), scenario.page, run.result.Outcome, run.frontmost, run.wasHidden)
			}
			encoded, _ := json.Marshal(run.result)
			if run.err != nil && strings.Contains(run.err.Error(), integrationPassword) || strings.Contains(string(encoded), integrationPassword) {
				t.Fatalf("%s: the password reached the result", scenario.page)
			}
			t.Logf("%s %s: %s, err %v, %d posts, %v, frontmost apps %v (browser %d)", launched.name(), scenario.page, encoded, run.err, len(run.posts),
				run.elapsed.Round(time.Millisecond), run.frontmost, launched.target.PID)
			if slices.Contains(run.frontmost, launched.target.PID) {
				t.Errorf("%s: the browser became the frontmost app", scenario.page)
			}
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
	result    signin.Result
	err       error
	posts     []post
	elapsed   time.Duration
	frontmost []int
	wasHidden bool
}

// isDisturbed: the window left the screen or the frontmost app changed. Either ends a sign-in the way a user switching to
// a full-screen app's Space does, and switching back to the browser's Space makes the browser frontmost; a browser that
// Hisar itself activates does so in every attempt.
func (run scenarioRun) isDisturbed() bool {
	return run.wasHidden || len(run.frontmost) > 1
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
	run.frontmost, run.wasHidden = stopWatching()
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

func (launched *browser) name() string {
	return strings.TrimSuffix(launched.app[strings.LastIndex(launched.app, "/")+1:], ".app")
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
