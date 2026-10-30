package signin

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/agentharness"
)

// Browser is a browser Hisar signs in with: on the Mac by the code signature the system verified for its process, on
// Linux by an executable the system's packages own.
type Browser struct {
	ID        string
	Name      string
	Signature agentharness.Signature
	// UserDataDirectory is the default profile folder under the home folder, where the browser writes
	// DevToolsActivePort while it accepts remote debugging.
	UserDataDirectory string
	// LaunchCommand starts the browser with its pages readable through AT-SPI (Linux).
	LaunchCommand string
}

// macBrowsers is every browser the real-browser test signed in with on the Mac. The Team IDs are the ones codesign -dv
// reports for the released apps; browsers_test.go pins them.
var macBrowsers = []Browser{
	{
		ID: "brave", Name: "Brave Browser",
		Signature:         agentharness.Signature{Identifier: "com.brave.Browser", TeamID: "KL8N8XSYF4"},
		UserDataDirectory: "Library/Application Support/BraveSoftware/Brave-Browser",
	},
	{
		ID: "chrome", Name: "Google Chrome",
		Signature:         agentharness.Signature{Identifier: "com.google.Chrome", TeamID: "EQHXZ8M8AV"},
		UserDataDirectory: "Library/Application Support/Google/Chrome",
	},
	{
		ID: "edge", Name: "Microsoft Edge",
		Signature:         agentharness.Signature{Identifier: "com.microsoft.edgemac", TeamID: "UBF8T346G9"},
		UserDataDirectory: "Library/Application Support/Microsoft Edge",
	},
}

// linuxBrowsers are the Chromium browsers at the paths their vendors' .deb and .rpm packages, and the distributions'
// Chromium, install them at. Linux has no code signatures, so a browser is its executable, owned by root and writable
// by root only, as packages leave it (isSystemExecutable).
var linuxBrowsers = []Browser{
	{ID: "chrome", Name: "Google Chrome", UserDataDirectory: ".config/google-chrome", LaunchCommand: "ACCESSIBILITY_ENABLED=1 google-chrome --force-renderer-accessibility"},
	{ID: "chromium", Name: "Chromium", UserDataDirectory: ".config/chromium", LaunchCommand: "ACCESSIBILITY_ENABLED=1 chromium --force-renderer-accessibility"},
	{
		ID: "brave", Name: "Brave", UserDataDirectory: ".config/BraveSoftware/Brave-Browser",
		LaunchCommand: "ACCESSIBILITY_ENABLED=1 brave-browser --force-renderer-accessibility",
	},
	{
		ID: "edge", Name: "Microsoft Edge", UserDataDirectory: ".config/microsoft-edge",
		LaunchCommand: "ACCESSIBILITY_ENABLED=1 microsoft-edge --force-renderer-accessibility",
	},
}

// linuxExecutables are the paths each Linux browser's packages install its main program at, by browser ID; Browser
// stays comparable, as a Destination is.
var linuxExecutables = map[string][]string{
	"chrome":   {"/opt/google/chrome/chrome"},
	"chromium": {"/usr/lib/chromium/chromium", "/usr/lib/chromium-browser/chromium-browser", "/usr/lib64/chromium-browser/chromium-browser"},
	"brave":    {"/opt/brave.com/brave/brave"},
	"edge":     {"/opt/microsoft/msedge/msedge"},
}

// platform is what differs between the systems Hisar signs in on: which browsers it knows and how it recognizes one,
// and the words its refusals use for the system.
type platform struct {
	browsers []Browser
	// isLinux recognizes a browser by its executable rather than its code signature.
	isLinux bool
	// isSystemExecutable reports a file root owns and only root can change, in directories only root can change.
	isSystemExecutable func(path string) bool
}

var (
	macPlatform   = platform{browsers: macBrowsers}
	linuxPlatform = platform{browsers: linuxBrowsers, isLinux: true, isSystemExecutable: isSystemExecutable}
)

// platformOf is the platform of the system whose windows driver reads.
func platformOf(driver Driver) platform {
	if driver.System() == SystemLinux {
		return linuxPlatform
	}
	return macPlatform
}

const devToolsProbeTimeout = 300 * time.Millisecond

func (platform platform) findBrowser(process agentharness.Process) (Browser, bool) {
	for _, browser := range platform.browsers {
		switch {
		case platform.isLinux:
			for _, executable := range linuxExecutables[browser.ID] {
				if process.Path == executable && platform.isSystemExecutable(process.Path) {
					return browser, true
				}
			}
		case process.Signature == browser.Signature:
			return browser, true
		}
	}
	return Browser{}, false
}

func (platform platform) listBrowserNames() string {
	names := make([]string, len(platform.browsers))
	for index, browser := range platform.browsers {
		names[index] = browser.Name
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// isDebuggable reports whether any program on the computer could drive the browser's pages through the DevTools
// protocol and read what Hisar enters: it was started with a remote-debugging switch, or its profile has a
// DevToolsActivePort file whose port accepts connections. A process whose arguments cannot be read counts as debuggable.
//
// Chromium rewrites its command line on Linux into one string of its arguments joined by spaces, so there each
// argument is split again, and a profile folder that is not there, as one whose name had a space would be, counts as
// debuggable.
func (platform platform) isDebuggable(process agentharness.Process, browser Browser, homeDirectory string) bool {
	arguments := process.Arguments
	if platform.isLinux {
		arguments = nil
		for _, argument := range process.Arguments {
			arguments = append(arguments, strings.Fields(argument)...)
		}
	}
	if len(arguments) == 0 {
		return true
	}
	userDataDirectory := filepath.Join(homeDirectory, browser.UserDataDirectory)
	hasUserDataDirectory := false
	for _, argument := range arguments[1:] {
		if !strings.HasPrefix(argument, "-") {
			continue
		}
		name, value, _ := strings.Cut(strings.TrimLeft(argument, "-"), "=")
		switch name {
		case "remote-debugging-port", "remote-debugging-pipe":
			return true
		case "user-data-dir":
			userDataDirectory, hasUserDataDirectory = value, true
		}
	}
	if !filepath.IsAbs(userDataDirectory) {
		if process.WorkingDirectory == "" {
			return true
		}
		userDataDirectory = filepath.Join(process.WorkingDirectory, userDataDirectory)
	}
	if platform.isLinux && hasUserDataDirectory {
		if info, err := os.Stat(userDataDirectory); err != nil || !info.IsDir() {
			return true
		}
	}
	return isDevToolsPortOpen(filepath.Join(userDataDirectory, "DevToolsActivePort"))
}

func isDevToolsPortOpen(path string) bool {
	contents, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	firstLine, _, _ := strings.Cut(string(contents), "\n")
	port, err := strconv.Atoi(strings.TrimSpace(firstLine))
	if err != nil || port < 1 || port > 65535 {
		return false
	}
	connection, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), devToolsProbeTimeout)
	if err != nil {
		return false
	}
	connection.Close()
	return true
}
