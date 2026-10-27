package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// browserRun is Layer A in one browser: its own fixture sites and logins, so no grant one browser's scenarios leave
// covers another's.
type browserRun struct {
	*run
	app      browserApp
	site     *site
	picker   *site
	fixture  login
	work     login
	personal login
	browser  *browser
	agent    *agent
	captures int
}

type call struct {
	answer chan signInAnswer
	err    chan error
}

func (current *run) startCall(client *agent, window cuaWindow, loginID string) call {
	started := call{answer: make(chan signInAnswer, 1), err: make(chan error, 1)}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		answer, err := client.signIn(ctx, window, loginID)
		if canRetryPageRefusal(answer, err) {
			current.retries.Add(1)
			fmt.Printf("RETRY sign_in for %s window %d after a documented pre-entry page refusal\n", window.AppName, window.WindowID)
			answer, err = client.signIn(ctx, window, loginID)
		}
		started.answer <- answer
		started.err <- err
	}()
	return started
}

func canRetryPageRefusal(answer signInAnswer, err error) bool {
	if err != nil || !answer.IsError {
		return false
	}
	return answer.Text == "The window shows a page that is not a website. Open the site's sign-in page and call sign_in again." ||
		answer.Text == "The window shows about:blank, not a website. Open the site's sign-in page and call sign_in again."
}

func (started call) wait() (signInAnswer, error) {
	return <-started.answer, <-started.err
}

func (started call) isPending() bool {
	return len(started.answer) == 0
}

func (current *run) approval() approvalWindow {
	return approvalWindow{ax: current.ax, pid: current.desktop.pid()}
}

func (current *run) waitWithoutDialog(started call) (signInAnswer, error) {
	for started.isPending() {
		if tree, _ := current.approval().read(); tree != nil {
			current.approval().deny()
			started.wait()
			return signInAnswer{}, errors.New("an approval dialog appeared")
		}
		time.Sleep(250 * time.Millisecond)
	}
	return started.wait()
}

func loginPlans(name string, served, picker *site) []loginPlan {
	slug := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	return []loginPlan{
		{Title: "Fixture " + name, Username: "user-" + slug + "@example.com", Origin: served.Main, HasOneTimePassword: true},
		{Title: "Work " + name, Username: "work-" + slug + "@example.com", Origin: picker.Main},
		{Title: "Personal " + name, Username: "personal-" + slug + "@example.com", Origin: picker.Main},
	}
}

func (current *run) runLayerA(invite string, apps []browserApp, unsupported browserApp, only []string) []*browserRun {
	runs := []*browserRun{}
	plans := []loginPlan{}
	oneTimeSecret := newOneTimeSecret()
	for index, app := range apps {
		sites := [2]*site{}
		for which := range sites {
			served, err := startSite(current.sitePath, filepath.Join(current.directory, "logs", fmt.Sprintf("site-%d-%d.jsonl", index, which)), oneTimePasswordURI(oneTimeSecret))
			if err != nil {
				current.fatal(err)
			}
			current.onExit(served.stop)
			sites[which] = served
		}
		runs = append(runs, &browserRun{run: current, app: app, site: sites[0], picker: sites[1]})
		plans = append(plans, loginPlans(app.Name, sites[0], sites[1])...)
	}
	seeded, err := seedAccount(current.paths.State, current.paths.Cache, invite, "agent-sign-in@example.com", "agent sign-in master password "+randomHex(6), plans, oneTimeSecret)
	if err != nil {
		current.fatal(err)
	}
	current.account = seeded
	passwords := []string{}
	for _, seededLogin := range seeded.Logins {
		passwords = append(passwords, seededLogin.Password)
	}
	current.canaries = canaries{Passwords: passwords, MasterPassword: seeded.Password, OneTimeSecret: seeded.OneTimeSecret, Started: current.started}
	if err := writeCanaries(filepath.Join(current.directory, "canaries.json"), current.canaries); err != nil {
		current.fatal(err)
	}
	if err := current.startDesktop(); err != nil {
		current.fatal(err)
	}
	for _, browserRun := range runs {
		browserRun.fixture = seeded.findLogin("Fixture " + browserRun.app.Name)
		browserRun.work = seeded.findLogin("Work " + browserRun.app.Name)
		browserRun.personal = seeded.findLogin("Personal " + browserRun.app.Name)
		browserRun.runAll(only)
		if unsupported.App != "" && (len(only) == 0 || slices.Contains(only, "unsupported-browser")) {
			browserRun.scenario("unsupported-browser", browserRun.checkUnsupportedBrowser(unsupported))
		}
	}
	return runs
}

func (current *run) startDesktop() error {
	desktop, err := startDesktop(current.paths, current.account)
	if err != nil {
		return err
	}
	current.desktop = desktop
	current.onExit(desktop.stop)
	return nil
}

func (current *browserRun) runAll(only []string) {
	launched, err := launchBrowser(current.cua, current.app, filepath.Join(current.directory, "profiles"), current.site.Main+"/signed-in", "Signed in")
	if err != nil {
		current.report.add(result{Layer: "A", Browser: current.app.Name, Scenario: "launch", Detail: err.Error()})
		return
	}
	current.onExit(launched.quit)
	current.browser = launched
	launched.site = current.site
	if err := launched.bringToCurrentSpace(current.cua); err != nil {
		current.report.add(result{Layer: "A", Browser: current.app.Name, Scenario: "launch", Detail: err.Error()})
		return
	}
	if err := current.connect(); err != nil {
		current.report.add(result{Layer: "A", Browser: current.app.Name, Scenario: "connect", Detail: err.Error()})
		return
	}
	for _, scenario := range []struct {
		name string
		run  func() (string, error)
	}{
		{"plain", current.checkPlain},
		{"deny", current.checkDeny},
		{"controlled", current.checkControlled},
		{"two-step", current.checkTwoStep},
		{"totp", current.checkOneTimePassword},
		{"wrong-password", current.checkWrongPassword},
		{"phishing-origin", current.checkPhishing},
		{"cross-origin-iframe", current.checkRefused("/iframe", "Framed sign-in", "comes from "+current.site.Other)},
		{"spoof-field", current.checkRefused("/spoof", "Sign in", "is not a real password input")},
		{"sign-up-form", current.checkRefused("/signup", "Create an account", "more than one password field")},
		{"navigate-during-approval", current.checkNavigateDuringApproval},
		{"several-logins", current.checkSeveralLogins},
		{"for-1-hour", current.checkOneHour},
		{"always-survives-restart", current.checkAlways},
		{"debuggable-browser", current.checkDebuggable},
		{"another-space", current.checkAnotherSpace},
	} {
		if len(only) == 0 || slices.Contains(only, scenario.name) {
			current.scenario(scenario.name, scenario.run)
		}
	}
}

func (current *browserRun) connect() error {
	if current.agent != nil {
		current.agent.close()
	}
	transcript := &lockedBuffer{}
	client, err := current.connectAgent(transcript)
	if err != nil {
		return err
	}
	current.agent = client
	path := filepath.Join(current.directory, "logs", fmt.Sprintf("agent-%s-%d.log", strings.ReplaceAll(current.app.Name, " ", "-"), time.Now().UnixNano()))
	current.onExit(func() {
		client.close()
		os.WriteFile(path, transcript.Bytes(), 0o600)
	})
	return nil
}

func (current *browserRun) scenario(name string, check func() (string, error)) {
	started := time.Now()
	before := current.retries.Load()
	var detail string
	err := current.browser.bringToCurrentSpace(current.cua)
	if err == nil {
		_, err = current.cua.call("bring_to_front", map[string]any{"pid": current.browser.pid, "window_id": current.browser.window.WindowID})
	}
	if err == nil {
		detail, err = check()
	}
	passed := err == nil
	if err != nil {
		detail = strings.TrimSpace(detail + " " + err.Error())
	}
	if found := current.captureBrowser(name, current.browser); found != "" {
		passed = false
		detail += "; " + found
	}
	current.report.add(result{Layer: "A", Browser: current.app.Name, Scenario: name, Passed: passed, Detail: detail, Retries: int(current.retries.Load() - before), Elapsed: time.Since(started)})
}

func (current *browserRun) captureBrowser(name string, shown *browser) string {
	if shown == nil {
		return ""
	}
	if err := shown.bringToCurrentSpace(current.cua); err != nil {
		return "capturing the test browser: " + err.Error()
	}
	current.captures++
	directory := filepath.Join(current.directory, "captures")
	os.MkdirAll(directory, 0o700)
	base := filepath.Join(directory, fmt.Sprintf("%s-%02d-%s", strings.ReplaceAll(current.app.Name, " ", "-"), current.captures, name))
	text, err := current.cua.captureWindow(shown.window, base+".png")
	if err != nil {
		return "get_window_state: " + err.Error()
	}
	if err := os.WriteFile(base+".txt", []byte(text), 0o600); err != nil {
		return err.Error()
	}
	scan := newScanner(current.canaries, current.ocrPath, directory)
	for _, path := range []string{base + ".txt", base + ".png"} {
		if err := scan.scanFile(path); err != nil {
			return err.Error()
		}
	}
	found := []string{}
	for _, hit := range scan.report.Matches {
		if !hit.isAllowed() {
			found = append(found, fmt.Sprintf("%s in get_window_state %s", hit.Kind, hit.Medium))
		}
	}
	return strings.Join(found, ", ")
}

func (current *browserRun) show(path, title string) error {
	return current.browser.showPage(current.cua, current.site, current.site.Main+path, title)
}

func (current *browserRun) approveNext(started call, loginText, duration string) (signInAnswer, error) {
	if _, err := current.approval().wait(30 * time.Second); err != nil {
		started.wait()
		return signInAnswer{}, err
	}
	if err := current.approval().approve(loginText, duration, current.account.Password); err != nil {
		current.approval().deny()
		started.wait()
		return signInAnswer{}, err
	}
	return started.wait()
}

func expectSubmitted(answer signInAnswer, err error, cleanup string, steps int) error {
	switch {
	case err != nil:
		return err
	case answer.IsError:
		return fmt.Errorf("refused: %s", answer.Text)
	case answer.Outcome != "submitted" || answer.Cleanup != cleanup || len(answer.Steps) != steps:
		return fmt.Errorf("got %s/%s in %d steps, want submitted/%s in %d: %s", answer.Outcome, answer.Cleanup, len(answer.Steps), cleanup, steps, answer.Message)
	}
	return nil
}

func expectRefusal(answer signInAnswer, err error, sentence string) error {
	switch {
	case err != nil:
		return err
	case !answer.IsError:
		return fmt.Errorf("signed in (%s) where a refusal was due", answer.Outcome)
	case !strings.Contains(answer.Text, sentence):
		return fmt.Errorf("refused with %q, want %q", answer.Text, sentence)
	}
	return nil
}

func (current *browserRun) postCount(served *site) int {
	posts, _ := served.readPosts()
	return len(posts)
}

func (current *browserRun) checkPlain() (string, error) {
	if err := current.show("/plain", "Plain sign-in"); err != nil {
		return "", err
	}
	before := current.postCount(current.site)
	started := current.startCall(current.agent, current.browser.window, "")
	window := current.approval()
	tree, err := window.wait(30 * time.Second)
	if err != nil {
		started.wait()
		return "", err
	}
	defer func() {
		if started.isPending() {
			window.deny()
		}
	}()
	text := tree.text()
	for _, want := range []string{
		"Claude Code", "wants to sign in to " + current.site.Main, current.site.Main + " in " + readBundleName(current.app.App),
		"Agents › " + current.fixture.Title + ", " + current.fixture.Username, "Touch ID to approve",
	} {
		if !strings.Contains(text, want) {
			return "", fmt.Errorf("the dialog does not say %q:\n%s", want, text)
		}
	}
	changedAt := time.Now().Add(-time.Second)
	if err := window.chooseDuration("For 1 hour"); err != nil {
		return "", err
	}
	time.Sleep(time.Second)
	cancelled, restarted, err := touchIDRestarts(window.pid, changedAt)
	if err != nil {
		return "", err
	}
	if cancelled == 0 || restarted == 0 {
		return "", fmt.Errorf("changing the duration while Touch ID waited cancelled %d checks and started %d", cancelled, restarted)
	}
	shown, err := window.listDurations()
	if err != nil {
		return "", err
	}
	if !slices.Equal(shown, durations) {
		return "", fmt.Errorf("the duration menu offers %v, want %v", shown, durations)
	}
	if err := window.choosePassword(); err != nil {
		return "", err
	}
	if err := window.pressApprove(); err == nil {
		time.Sleep(time.Second)
	}
	if err := window.typePassword("not the master password"); err != nil {
		return "", err
	}
	if err := window.pressApprove(); err != nil {
		return "", err
	}
	time.Sleep(2 * time.Second)
	if !started.isPending() || current.postCount(current.site) != before {
		answer, _ := started.wait()
		return "", fmt.Errorf("an Approve without the master password answered the agent: %+v", answer)
	}
	if tree, _ := window.read(); tree == nil {
		return "", errors.New("the dialog closed after a wrong master password")
	}
	if err := window.typePassword(current.account.Password); err != nil {
		return "", err
	}
	if err := window.pressApprove(); err != nil {
		return "", err
	}
	answer, err := started.wait()
	if err := expectSubmitted(answer, err, "page-left", 1); err != nil {
		return "", err
	}
	posts := current.site.waitForPosts(before, 1, 5*time.Second)
	if len(posts) != 1 || posts[0].Form["username"] != current.fixture.Username || posts[0].Form["password"] != current.fixture.Password {
		return "", fmt.Errorf("the site got %d posts, not the login", len(posts))
	}
	return fmt.Sprintf("dialog named Claude Code, %s, %s and the login; menu %v; duration change cancelled %d Touch ID check(s) and started %d; Approve without the password and with a wrong one left the request pending; submitted, page-left",
		current.site.Main, readBundleName(current.app.App), shown, cancelled, restarted), nil
}

func (current *browserRun) checkDeny() (string, error) {
	if err := current.show("/plain", "Plain sign-in"); err != nil {
		return "", err
	}
	before := current.postCount(current.site)
	started := current.startCall(current.agent, current.browser.window, "")
	if _, err := current.approval().wait(30 * time.Second); err != nil {
		started.wait()
		return "", fmt.Errorf("Only this time did not ask again: %w", err)
	}
	if err := current.approval().deny(); err != nil {
		return "", err
	}
	answer, err := started.wait()
	if err := expectRefusal(answer, err, "denied"); err != nil {
		return "", err
	}
	if current.postCount(current.site) != before {
		return "", errors.New("a denied sign-in reached the site")
	}
	return "asked again after Only this time; Deny: " + answer.Text, nil
}

func (current *browserRun) checkControlled() (string, error) {
	if err := current.show("/controlled", "Controlled sign-in"); err != nil {
		return "", err
	}
	before := current.postCount(current.site)
	answer, err := current.approveNext(current.startCall(current.agent, current.browser.window, ""), "", "Only this time")
	if err := expectSubmitted(answer, err, "fields-removed", 1); err != nil {
		return "", err
	}
	posts := current.site.waitForPosts(before, 1, 5*time.Second)
	if len(posts) != 1 || !posts[0].holds(current.fixture.Password) {
		return "", fmt.Errorf("the controlled form sent %d posts without the password", len(posts))
	}
	return "submitted, fields-removed; the page's state held the login", nil
}

func (current *browserRun) checkTwoStep() (string, error) {
	if err := current.show("/step1", "Sign in, step 1"); err != nil {
		return "", err
	}
	before := current.postCount(current.site)
	answer, err := current.approveNext(current.startCall(current.agent, current.browser.window, ""), "", "Only this time")
	if err := expectSubmitted(answer, err, "page-left", 2); err != nil {
		return "", err
	}
	posts := current.site.waitForPosts(before, 2, 5*time.Second)
	if len(posts) != 2 || posts[1].Path != "/submit/step2" || posts[1].Form["password"] != current.fixture.Password {
		return "", fmt.Errorf("the site got %+v", len(posts))
	}
	return "submitted in 2 steps, page-left", nil
}

func (current *browserRun) checkOneTimePassword() (string, error) {
	if err := current.show("/totp", "Sign in with a code"); err != nil {
		return "", err
	}
	before := current.postCount(current.site)
	answer, err := current.approveNext(current.startCall(current.agent, current.browser.window, ""), "", "Only this time")
	if err := expectSubmitted(answer, err, "page-left", 2); err != nil {
		return "", err
	}
	posts := current.site.waitForPosts(before, 2, 5*time.Second)
	if len(posts) != 2 || posts[1].Path != "/submit/totp-code" || len(posts[1].Form["code"]) != 8 {
		return "", fmt.Errorf("the site got %d posts and no eight-digit code", len(posts))
	}
	if _, err := current.cua.waitForWindow(current.browser.pid, 10*time.Second, func(title string) bool { return title == "Signed in" }); err != nil {
		return "", fmt.Errorf("the site did not accept the code: %w", err)
	}
	return "submitted password then an eight-digit code the site accepted", nil
}

func (current *browserRun) checkWrongPassword() (string, error) {
	if err := current.show("/wrong", "Sign in"); err != nil {
		return "", err
	}
	before := current.postCount(current.site)
	answer, err := current.approveNext(current.startCall(current.agent, current.browser.window, ""), "", "Only this time")
	switch {
	case err != nil:
		return "", err
	case answer.IsError || answer.Outcome != "not-accepted" || answer.Cleanup != "fields-cleared":
		return "", fmt.Errorf("got %s/%s %s, want not-accepted/fields-cleared", answer.Outcome, answer.Cleanup, answer.Text)
	}
	if posts := current.site.waitForPosts(before, 1, 5*time.Second); len(posts) != 1 || !posts[0].holds(current.fixture.Password) {
		return "", errors.New("the wrong-password page never got the login")
	}
	if err := current.submitAgain(); err != nil {
		return "", err
	}
	posts := current.site.waitForPosts(before+1, 1, 5*time.Second)
	if len(posts) != 1 || !strings.Contains(string(posts[0].JSON), `"username":""`) || !strings.Contains(string(posts[0].JSON), `"password":""`) {
		return "", fmt.Errorf("the page still held entries after not-accepted: %s", posts)
	}
	return "not-accepted, fields-cleared; the page submitted again sent an empty username and password", nil
}

func (current *browserRun) submitAgain() error {
	output, err := current.cua.call("get_window_state", map[string]any{
		"pid": current.browser.pid, "window_id": current.browser.window.WindowID, "include_screenshot": false, "query": "Sign in",
	})
	if err != nil {
		return err
	}
	token, err := findButtonToken(output, "Sign in")
	if err != nil {
		return err
	}
	clicked, err := current.cua.call("click", map[string]any{"pid": current.browser.pid, "window_id": current.browser.window.WindowID, "element_token": token})
	if err == nil && strings.Contains(string(clicked), `"isError": true`) {
		err = fmt.Errorf("cua-driver click: %.300s", clicked)
	}
	return err
}

func (current *browserRun) checkPhishing() (string, error) {
	if err := current.browser.showPage(current.cua, current.site, current.site.Other+"/phish", "Sign in"); err != nil {
		return "", err
	}
	before := current.postCount(current.site)
	answer, err := current.waitWithoutDialog(current.startCall(current.agent, current.browser.window, ""))
	if err := expectRefusal(answer, err, "No login in the vaults available to agents is saved for "+current.site.Other); err != nil {
		return "", err
	}
	if current.postCount(current.site) != before {
		return "", errors.New("the phishing page got a post")
	}
	return "no dialog, no post: " + answer.Text, nil
}

func (current *browserRun) checkRefused(path, title, sentence string) func() (string, error) {
	return func() (string, error) {
		if err := current.show(path, title); err != nil {
			return "", err
		}
		before := current.postCount(current.site)
		answer, err := current.waitWithoutDialog(current.startCall(current.agent, current.browser.window, ""))
		if err := expectRefusal(answer, err, sentence); err != nil {
			return "", err
		}
		if current.postCount(current.site) != before {
			return "", errors.New("the page got a post")
		}
		return "no dialog, no post: " + answer.Text, nil
	}
}

func (current *browserRun) checkNavigateDuringApproval() (string, error) {
	if err := current.show("/plain", "Plain sign-in"); err != nil {
		return "", err
	}
	before := current.postCount(current.site)
	started := current.startCall(current.agent, current.browser.window, "")
	if _, err := current.approval().wait(30 * time.Second); err != nil {
		started.wait()
		return "", err
	}
	if err := current.site.steer(current.site.Main + "/signed-in"); err != nil {
		return "", err
	}
	if _, err := current.cua.waitForWindow(current.browser.pid, 10*time.Second, func(title string) bool { return title == "Signed in" }); err != nil {
		return "", err
	}
	if err := current.approval().approve("", "Only this time", current.account.Password); err != nil {
		current.approval().deny()
		started.wait()
		return "", err
	}
	answer, err := started.wait()
	if err := expectRefusal(answer, err, "The window changed while the user was approving"); err != nil {
		return "", err
	}
	if current.postCount(current.site) != before {
		return "", errors.New("the site got a post")
	}
	return "changed, nothing entered: " + answer.Text, nil
}

func (current *browserRun) checkSeveralLogins() (string, error) {
	if err := current.browser.showPage(current.cua, current.picker, current.picker.Main+"/plain", "Plain sign-in"); err != nil {
		return "", err
	}
	before := current.postCount(current.picker)
	started := current.startCall(current.agent, current.browser.window, "")
	tree, err := current.approval().wait(30 * time.Second)
	if err != nil {
		started.wait()
		return "", err
	}
	text := tree.text()
	if !strings.Contains(text, current.work.Username) || !strings.Contains(text, current.personal.Username) {
		current.approval().deny()
		started.wait()
		return "", fmt.Errorf("the dialog does not offer both logins:\n%s", text)
	}
	if err := current.approval().approve(current.personal.Username, "Only this time", current.account.Password); err != nil {
		current.approval().deny()
		started.wait()
		return "", err
	}
	answer, err := started.wait()
	if err := expectSubmitted(answer, err, "page-left", 1); err != nil {
		return "", err
	}
	posts := current.picker.waitForPosts(before, 1, 5*time.Second)
	if len(posts) != 1 || posts[0].Form["username"] != current.personal.Username || posts[0].Form["password"] != current.personal.Password {
		return "", errors.New("the site did not get the chosen login")
	}
	return "the dialog offered both logins; the chosen one signed in", nil
}

func (current *browserRun) checkOneHour() (string, error) {
	if err := current.show("/plain", "Plain sign-in"); err != nil {
		return "", err
	}
	answer, err := current.approveNext(current.startCall(current.agent, current.browser.window, ""), "", "For 1 hour")
	if err := expectSubmitted(answer, err, "page-left", 1); err != nil {
		return "", err
	}
	if err := current.show("/plain", "Plain sign-in"); err != nil {
		return "", err
	}
	before := current.postCount(current.site)
	answer, err = current.waitWithoutDialog(current.startCall(current.agent, current.browser.window, ""))
	if err := expectSubmitted(answer, err, "page-left", 1); err != nil {
		return "", fmt.Errorf("For 1 hour asked again or failed: %w", err)
	}
	if posts := current.site.waitForPosts(before, 1, 5*time.Second); len(posts) != 1 || posts[0].Form["password"] != current.fixture.Password {
		return "", errors.New("the second sign-in did not reach the site")
	}
	return "approved For 1 hour; the next sign-in ran without a dialog", nil
}

func (current *browserRun) checkAlways() (string, error) {
	if err := current.browser.showPage(current.cua, current.picker, current.picker.Main+"/plain", "Plain sign-in"); err != nil {
		return "", err
	}
	answer, err := current.approveNext(current.startCall(current.agent, current.browser.window, current.work.ID), "", "Always")
	if err := expectSubmitted(answer, err, "page-left", 1); err != nil {
		return "", err
	}
	current.agent.close()
	current.agent = nil
	current.desktop.stop()
	if err := current.startDesktop(); err != nil {
		return "", fmt.Errorf("restarting the app: %w", err)
	}
	if err := current.connect(); err != nil {
		return "", err
	}
	if err := current.browser.showPage(current.cua, current.picker, current.picker.Main+"/plain", "Plain sign-in"); err != nil {
		return "", err
	}
	answer, err = current.waitWithoutDialog(current.startCall(current.agent, current.browser.window, current.work.ID))
	if err := expectSubmitted(answer, err, "page-left", 1); err != nil {
		return "", fmt.Errorf("after the restart: %w", err)
	}
	grant := "Sign in to " + current.picker.Main + " as"
	listed, err := current.revokeGrant(grant)
	if err != nil {
		return "", err
	}
	if err := current.browser.showPage(current.cua, current.picker, current.picker.Main+"/plain", "Plain sign-in"); err != nil {
		return "", err
	}
	started := current.startCall(current.agent, current.browser.window, current.work.ID)
	if _, err := current.approval().wait(30 * time.Second); err != nil {
		started.wait()
		return "", fmt.Errorf("the revoked grant still signed in: %w", err)
	}
	current.approval().deny()
	started.wait()
	return "Always outlived the restart for the same harness program; Settings listed " + strconv.Quote(listed) + " with Revoke; after Revoke the agent was asked again", nil
}

func (current *browserRun) revokeGrant(name string) (string, error) {
	pid := current.desktop.pid()
	if err := current.desktop.showMainWindow(); err != nil {
		return "", err
	}
	if _, err := current.cua.waitForWindow(pid, 20*time.Second, func(title string) bool { return title == "Hisar" }); err != nil {
		return "", fmt.Errorf("the main window did not open: %w", err)
	}
	if err := current.waitAndPress(pid, "window=Hisar;role=AXButton;name=Settings"); err != nil {
		return "", err
	}
	if err := current.waitAndPress(pid, "window=Settings;role=AXButton;name=Developer tools"); err != nil {
		return "", err
	}
	tree, err := current.ax.window(pid, "Settings")
	if err != nil {
		return "", err
	}
	text := tree.text()
	row := ""
	for line := range strings.Lines(text) {
		if strings.HasPrefix(line, name) {
			row = strings.TrimSpace(line)
		}
	}
	if row == "" || !strings.Contains(text, "Until you revoke it") {
		return "", fmt.Errorf("Settings lists no grant %q:\n%s", name, text)
	}
	if err := current.ax.press(pid, "window=Settings;role=AXButton;name=Revoke;after="+row); err != nil {
		return "", err
	}
	time.Sleep(time.Second)
	if tree, err := current.ax.window(pid, "Settings"); err != nil || strings.Contains(tree.text(), row) {
		return "", fmt.Errorf("Settings still lists %q after Revoke", row)
	}
	current.ax.press(pid, "window=Settings;role=AXButton;name=close button")
	return row, nil
}

func (current *browserRun) waitAndPress(pid int, selector string) error {
	var err error
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(300 * time.Millisecond) {
		if err = current.ax.press(pid, selector); err == nil {
			time.Sleep(500 * time.Millisecond)
			return nil
		}
	}
	tree, treeError := current.ax.window(pid, "Hisar")
	return fmt.Errorf("%w; main window AX (%v):\n%s", err, treeError, tree.text())
}

func (current *browserRun) checkDebuggable() (string, error) {
	debuggable, err := launchBrowser(current.cua, current.app, filepath.Join(current.directory, "profiles"), current.site.Main+"/plain", "Plain sign-in", "--remote-debugging-port=0")
	if err != nil {
		return "", err
	}
	defer debuggable.quit()
	if err := debuggable.bringToCurrentSpace(current.cua); err != nil {
		return "", err
	}
	answer, err := current.waitWithoutDialog(current.startCall(current.agent, debuggable.window, ""))
	if found := current.captureBrowser("debuggable-browser-target", debuggable); found != "" {
		return "", errors.New(found)
	}
	if err := expectRefusal(answer, err, "was started with remote debugging"); err != nil {
		return "", err
	}
	return "no dialog: " + answer.Text, nil
}

func (current *browserRun) checkUnsupportedBrowser(app browserApp) func() (string, error) {
	return func() (string, error) {
		unsupported, err := launchBrowser(current.cua, app, filepath.Join(current.directory, "profiles"), current.site.Main+"/plain", "Plain sign-in")
		if err != nil {
			return "", err
		}
		defer unsupported.quit()
		if err := unsupported.bringToCurrentSpace(current.cua); err != nil {
			return "", err
		}
		answer, err := current.waitWithoutDialog(current.startCall(current.agent, unsupported.window, ""))
		if found := current.captureBrowser("unsupported-browser-target", unsupported); found != "" {
			return "", errors.New(found)
		}
		if err := expectRefusal(answer, err, "not a browser Hisar signs in with"); err != nil {
			return "", err
		}
		return "no dialog: " + answer.Text, nil
	}
}

func (current *browserRun) checkAnotherSpace() (string, error) {
	if err := current.show("/plain", "Plain sign-in"); err != nil {
		return "", err
	}
	here, other, err := findSpaces()
	if err != nil {
		return "", err
	}
	if err := moveWindow(current.browser.window.WindowID, other); err != nil {
		return "", err
	}
	defer func() {
		moveWindow(current.browser.window.WindowID, here)
		current.browser.bringToCurrentSpace(current.cua)
	}()
	if _, err := current.cua.waitForWindow(current.browser.pid, 10*time.Second, func(string) bool { return true }); err != nil {
		return "", err
	}
	answer, err := current.waitWithoutDialog(current.startCall(current.agent, current.browser.window, ""))
	if err := expectRefusal(answer, err, "is on another Space"); err != nil {
		return "", err
	}
	return "no dialog: " + answer.Text, nil
}

func (shown *browser) bringToCurrentSpace(driver cuaDriver) error {
	windows, err := driver.listWindows(shown.pid)
	if err != nil {
		return err
	}
	for _, window := range windows {
		if window.WindowID == shown.window.WindowID && window.OnCurrentSpace {
			return nil
		}
	}
	here, _, err := findSpaces()
	if err != nil {
		return fmt.Errorf("the test window is on another Space and yabai cannot move it: %w", err)
	}
	if err := moveWindow(shown.window.WindowID, here); err != nil {
		return err
	}
	time.Sleep(time.Second)
	return nil
}

func findSpaces() (here, other int, err error) {
	layout, err := readDisplayLayout()
	if err != nil {
		return 0, 0, err
	}
	return layout.VisibleSpace, layout.OtherSpace, nil
}

func moveWindow(windowID uint32, space int) error {
	if output, err := exec.Command("yabai", "-m", "window", strconv.Itoa(int(windowID)), "--space", strconv.Itoa(space)).CombinedOutput(); err != nil {
		return fmt.Errorf("yabai -m window %d --space %d: %v: %s", windowID, space, err, output)
	}
	return nil
}
