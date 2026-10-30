//go:build darwin && !ios && cgo

package axfill

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/agentharness"
	"github.com/tryhisar/hisar/core/signin"
)

// screenWatch is what the screen did during a sign-in: the frontmost apps in turn, and whether the window left the
// screen.
type screenWatch struct {
	frontmost []int
	wasHidden bool
}

// isDisturbed: the window left the screen or the frontmost app changed. Either ends a sign-in the way a user switching to
// a full-screen app's Space does, and switching back to the browser's Space makes the browser frontmost; a browser that
// Hisar itself activates does so in every attempt.
func (watch screenWatch) isDisturbed() bool {
	return watch.wasHidden || len(watch.frontmost) > 1
}

func (watch screenWatch) String() string {
	return fmt.Sprintf("frontmost apps %v, window hidden %v", watch.frontmost, watch.wasHidden)
}

// check fails a sign-in that brought the browser to the front: Hisar fills a background window on the Mac.
func (watch screenWatch) check(t *testing.T, page string, launched *browser) {
	t.Helper()
	if slices.Contains(watch.frontmost, launched.target.PID) {
		t.Errorf("%s: the browser became the frontmost app", page)
	}
}

func watchScreen(t *testing.T, target signin.Target) (stop func() screenWatch) {
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
	return func() screenWatch {
		close(done)
		<-finished
		return screenWatch{frontmost: frontmost, wasHidden: wasHidden}
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

func (launched *browser) name() string {
	return strings.TrimSuffix(launched.app[strings.LastIndex(launched.app, "/")+1:], ".app")
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

var browserIDs = map[string]string{
	"com.brave.Browser":     "brave",
	"com.google.Chrome":     "chrome",
	"com.microsoft.edgemac": "edge",
}

// recordedBrowserID is the fixture folder of a browser: on the Mac its code signature names it.
func recordedBrowserID(process agentharness.Process) string {
	return browserIDs[process.Signature.Identifier]
}

// normalizeProcess keeps what identifies the browser on the Mac, its code signature.
func normalizeProcess(process agentharness.Process) agentharness.Process {
	return agentharness.Process{Signature: process.Signature}
}
