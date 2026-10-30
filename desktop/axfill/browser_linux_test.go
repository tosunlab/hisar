package axfill

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"

	"github.com/tryhisar/hisar/core/agentharness"
	"github.com/tryhisar/hisar/core/signin"
)

// screenWatch is what the screen did during a sign-in: the active windows in turn.
type screenWatch struct {
	active []uint32
}

// isDisturbed: another window became the active one, which ends a sign-in the way a user switching windows does.
func (watch screenWatch) isDisturbed() bool {
	return len(watch.active) > 1
}

func (watch screenWatch) String() string {
	return fmt.Sprintf("active windows %#x", watch.active)
}

// check has nothing to check on Linux: Hisar signs in only in the window in front.
func (screenWatch) check(*testing.T, string, *browser) {}

func readActiveWindow(t *testing.T, connection *xgb.Conn) uint32 {
	t.Helper()
	windows := readWindowList(t, connection, "_NET_ACTIVE_WINDOW")
	if len(windows) == 0 {
		return 0
	}
	return windows[0]
}

func readWindowList(t *testing.T, connection *xgb.Conn, name string) []uint32 {
	t.Helper()
	atom, err := xproto.InternAtom(connection, true, uint16(len(name)), name).Reply()
	if err != nil {
		t.Fatal(err)
	}
	root := xproto.Setup(connection).DefaultScreen(connection).Root
	reply, err := xproto.GetProperty(connection, false, root, atom.Atom, xproto.AtomWindow, 0, 1024).Reply()
	if err != nil {
		t.Fatal(err)
	}
	windows := []uint32{}
	for offset := 0; offset+4 <= len(reply.Value); offset += 4 {
		windows = append(windows, xgb.Get32(reply.Value[offset:]))
	}
	return windows
}

func connectDisplay(t *testing.T) *xgb.Conn {
	t.Helper()
	connection, err := xgb.NewConn()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(connection.Close)
	return connection
}

func watchScreen(t *testing.T, target signin.Target) (stop func() screenWatch) {
	t.Helper()
	connection := connectDisplay(t)
	done := make(chan struct{})
	finished := make(chan struct{})
	active := []uint32{readActiveWindow(t, connection)}
	go func() {
		defer close(finished)
		for {
			select {
			case <-done:
				return
			case <-time.After(25 * time.Millisecond):
			}
			if window := readActiveWindow(t, connection); window != active[len(active)-1] {
				active = append(active, window)
			}
		}
	}()
	return func() screenWatch {
		close(done)
		<-finished
		return screenWatch{active: active}
	}
}

func (launched *browser) name() string {
	return filepath.Base(launched.app)
}

// launchBrowser starts the browser executable app with accessibility on, as the sign_in refusal tells the user to,
// and waits for its window to be the active one.
func launchBrowser(t *testing.T, app, pageURL string, arguments ...string) *browser {
	t.Helper()
	profile, err := os.MkdirTemp("", "hisar-sign-in-profile-")
	if err != nil {
		t.Fatal(err)
	}
	launched := &browser{app: app, profile: profile}
	switches := []string{"--user-data-dir=" + profile, "--no-first-run", "--no-default-browser-check", "--password-store=basic",
		rendererAccessibilitySwitch, "--disable-features=Translate", "--window-size=1200,900"}
	if os.Geteuid() == 0 {
		switches = append(switches, "--no-sandbox")
	}
	command := exec.Command(app, append(append(switches, arguments...), pageURL)...)
	command.Env = append(os.Environ(), "ACCESSIBILITY_ENABLED=1")
	command.Stdout, command.Stderr = nil, nil
	if err := command.Start(); err != nil {
		t.Fatalf("starting %s: %v", app, err)
	}
	go command.Wait()
	launched.target.PID = command.Process.Pid
	t.Cleanup(launched.quit)
	connection := connectDisplay(t)
	display := &display{}
	for deadline := time.Now().Add(30 * time.Second); launched.target.WindowID == 0; time.Sleep(200 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("%s did not open a window", app)
		}
		windows := readWindowList(t, connection, "_NET_CLIENT_LIST")
		active := readActiveWindow(t, connection)
		if !slices.Contains(windows, active) {
			continue
		}
		if state, err := display.readWindow(active); err == nil && state.pid == launched.target.PID {
			launched.target.WindowID = active
		}
	}
	return launched
}

var browserIDs = map[string]string{
	"/opt/google/chrome/chrome":    "chrome",
	"/usr/lib/chromium/chromium":   "chromium",
	"/opt/brave.com/brave/brave":   "brave",
	"/opt/microsoft/msedge/msedge": "edge",
}

// recordedBrowserID is the fixture folder of a browser: on Linux its executable names it, under linux-<id> beside the
// Mac's folders.
func recordedBrowserID(process agentharness.Process) string {
	if id := browserIDs[process.Path]; id != "" {
		return "linux-" + id
	}
	return ""
}

// normalizeProcess keeps what identifies the browser on Linux, its executable.
func normalizeProcess(process agentharness.Process) agentharness.Process {
	return agentharness.Process{Path: process.Path}
}
