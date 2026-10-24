package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type browserApp struct {
	Name string
	App  string
}

// browser is one instance of a browser app, started by the run with a throwaway profile; the user's own windows and
// profiles are never touched.
type browser struct {
	app     browserApp
	profile string
	pid     int
	window  cuaWindow
	site    *site
}

func launchBrowser(driver cuaDriver, app browserApp, profileRoot, pageURL, title string, arguments ...string) (*browser, error) {
	if err := os.MkdirAll(profileRoot, 0o700); err != nil {
		return nil, err
	}
	profile, err := os.MkdirTemp(profileRoot, "profile-")
	if err != nil {
		return nil, err
	}
	launched := &browser{app: app, profile: profile}
	layout, err := readDisplayLayout()
	if err != nil {
		return nil, err
	}
	open := append([]string{"-n", "-a", app.App, "--args", "--user-data-dir=" + profile, "--no-first-run",
		"--no-default-browser-check", "--use-mock-keychain", "--disable-background-networking", "--disable-component-update",
		"--disable-sync", "--disable-default-apps",
		fmt.Sprintf("--window-position=%d,%d", int(layout.Frame.X)+80, int(layout.Frame.Y)+80),
		"--window-size=1100,750"}, arguments...)
	if output, err := exec.Command("open", append(open, pageURL)...).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("open %s: %v: %s", app.App, err, output)
	}
	for deadline := time.Now().Add(30 * time.Second); launched.pid == 0; time.Sleep(200 * time.Millisecond) {
		if time.Now().After(deadline) {
			launched.quit()
			return nil, fmt.Errorf("%s did not start", app.Name)
		}
		launched.pid = findMainProcess(profile)
	}
	launched.window, err = driver.waitForWindow(launched.pid, 30*time.Second, func(shown string) bool { return shown == title })
	if err != nil {
		launched.quit()
		return nil, err
	}
	if err := launched.placeOnDisplay(driver, layout); err != nil {
		launched.quit()
		return nil, err
	}
	return launched, nil
}

func findMainProcess(profile string) int {
	output, err := exec.Command("ps", "-axww", "-o", "pid=,args=").Output()
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(output), "\n") {
		if strings.Contains(line, "--user-data-dir="+profile) && !strings.Contains(line, "--type=") {
			pid, _ := strconv.Atoi(strings.Fields(line)[0])
			return pid
		}
	}
	return 0
}

// showPage steers the browser to pageURL and waits for its window to carry title. It goes through served's signed-in
// page first, steered by the site the browser shows now (a page follows only its own site), so a title the previous
// page shared cannot be mistaken for the new page's.
func (launched *browser) showPage(driver cuaDriver, served *site, pageURL, title string) error {
	visit := randomHex(8)
	for _, step := range []struct{ url, title string }{{served.Main + "/signed-in?visit=" + visit, "Signed in " + visit}, {pageURL, title}} {
		if err := launched.site.steer(step.url); err != nil {
			return err
		}
		window, err := driver.waitForWindow(launched.pid, 20*time.Second, func(shown string) bool { return shown == step.title })
		if err != nil {
			return fmt.Errorf("%s did not show %s: %w", launched.app.Name, step.url, err)
		}
		launched.window = window
		launched.site = served
	}
	// The title changes before the page's script and fields settle.
	time.Sleep(400 * time.Millisecond)
	return nil
}

func (launched *browser) quit() {
	if launched.pid != 0 {
		syscall.Kill(launched.pid, syscall.SIGTERM)
		for attempt := 0; attempt < 50 && syscall.Kill(launched.pid, 0) == nil; attempt++ {
			time.Sleep(100 * time.Millisecond)
		}
		syscall.Kill(launched.pid, syscall.SIGKILL)
	}
}

func readBundleName(app string) string {
	return strings.TrimSuffix(filepath.Base(app), ".app")
}
