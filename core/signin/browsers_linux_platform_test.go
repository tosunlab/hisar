package signin

import (
	"errors"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/agentharness"
)

// testLinuxPlatform is Linux's with every packaged path counting as the system's, since the test computer has none of
// the browsers installed.
func testLinuxPlatform(systemPaths ...string) platform {
	linux := linuxPlatform
	linux.isSystemExecutable = func(path string) bool {
		for _, systemPath := range systemPaths {
			if path == systemPath {
				return true
			}
		}
		return false
	}
	return linux
}

// On Linux a browser is its packaged executable, and only one root owns and no one else can change: the same file at a
// path the user could write to, or anywhere else, is no browser.
func TestLinuxBrowsersAreTheirPackagedExecutables(t *testing.T) {
	linux := testLinuxPlatform("/opt/google/chrome/chrome", "/usr/lib/chromium/chromium", "/opt/brave.com/brave/brave", "/opt/microsoft/msedge/msedge")
	for path, id := range map[string]string{
		"/opt/google/chrome/chrome":    "chrome",
		"/usr/lib/chromium/chromium":   "chromium",
		"/opt/brave.com/brave/brave":   "brave",
		"/opt/microsoft/msedge/msedge": "edge",
	} {
		if browser, isFound := linux.findBrowser(agentharness.Process{Path: path}); !isFound || browser.ID != id {
			t.Errorf("%s: %+v, %v", path, browser, isFound)
		}
	}
	for _, process := range []agentharness.Process{
		{Path: "/usr/lib/chromium-browser/chromium-browser"},
		{Path: "/home/user/chrome-linux/chrome"},
		{Path: "/opt/google/chrome/chrome-wrapper"},
		{Path: ""},
		{Signature: agentharness.Signature{Identifier: "com.google.Chrome", TeamID: "EQHXZ8M8AV"}},
	} {
		if browser, isFound := linux.findBrowser(process); isFound {
			t.Errorf("%+v is %s", process, browser.ID)
		}
	}
}

// isSystemExecutable checks the file and every directory above it, so a file the user owns, or one in a directory the
// user can write to, never counts.
func TestSystemExecutablesAreRootsOnly(t *testing.T) {
	if !isSystemExecutable("/usr/bin/env") {
		t.Error("/usr/bin/env is not the system's")
	}
	for _, path := range []string{t.TempDir(), "relative/chrome", "/no/such/chrome"} {
		if isSystemExecutable(path) {
			t.Errorf("%s counts as the system's", path)
		}
	}
}

// Chromium on Linux rewrites its command line into one string, so the switches Hisar looks for are inside it.
func TestLinuxReadsChromiumsRewrittenCommandLine(t *testing.T) {
	home := t.TempDir()
	chromium, _ := testLinuxPlatform("/usr/lib/chromium/chromium").findBrowser(agentharness.Process{Path: "/usr/lib/chromium/chromium"})
	profile := t.TempDir()
	for title, isDebuggable := range map[string]bool{
		"/usr/lib/chromium/chromium --user-data-dir=" + profile + " --force-renderer-accessibility": false,
		"/usr/lib/chromium/chromium --force-renderer-accessibility --remote-debugging-port=0":       true,
		"/usr/lib/chromium/chromium --remote-debugging-pipe":                                        true,
		// A profile folder whose name had a space splits into one that is not there.
		"/usr/lib/chromium/chromium --user-data-dir=" + profile + "/My Profile": true,
		"/usr/lib/chromium/chromium --user-data-dir=/no/such/profile":           true,
	} {
		process := agentharness.Process{Path: "/usr/lib/chromium/chromium", Arguments: []string{title}, WorkingDirectory: "/"}
		if got := linuxPlatform.isDebuggable(process, chromium, home); got != isDebuggable {
			t.Errorf("%q: debuggable %v, want %v", title, got, isDebuggable)
		}
	}
}

func TestLinuxRefusalsNameTheLinuxWays(t *testing.T) {
	linux := testLinuxPlatform("/opt/google/chrome/chrome")
	target := Target{PID: 4242, WindowID: 0x4000002}
	for name, testCase := range map[string]struct {
		err     error
		reason  Reason
		message string
	}{
		"no accessibility": {
			err: &InaccessibleError{Path: "/opt/google/chrome/chrome"}, reason: ReasonInaccessible,
			message: "Google Chrome was started without accessibility, so Hisar can't read its pages. Ask the user to quit it and start it again with ACCESSIBILITY_ENABLED=1 google-chrome --force-renderer-accessibility, then call sign_in again.",
		},
		"no display or bus": {err: ErrNoAccess, reason: ReasonNoAccess, message: "this session has no X11 display with the XTest and X-Resource extensions, or no accessibility bus"},
		"not in front":      {err: ErrNotVisible, reason: ReasonNotVisible, message: "Window 67108866 is not the active window"},
		"no such window":    {err: ErrWindowNotFound, reason: ReasonWindowNotFound, message: "X11 window ID"},
	} {
		refusal := linux.refuseObserving(target, testCase.err)
		if refusal.Reason != testCase.reason || !strings.Contains(refusal.Message, testCase.message) || strings.Contains(refusal.Message, "macOS") {
			t.Errorf("%s: %+v", name, refusal)
		}
	}
	if refusal := linux.refuseObserving(target, errors.Join(errors.New("read"), &InaccessibleError{Path: "/tmp/chrome"})); refusal.Reason != ReasonInaccessible ||
		!strings.Contains(refusal.Message, "The browser was started without accessibility") {
		t.Errorf("an unknown browser: %+v", refusal)
	}
}
