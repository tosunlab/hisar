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

// Browser is a browser Hisar signs in with, recognized by the code signature the system verified for its process.
type Browser struct {
	ID        string
	Name      string
	Signature agentharness.Signature
	// UserDataDirectory is the default profile folder under the home folder, where the browser writes
	// DevToolsActivePort while it accepts remote debugging.
	UserDataDirectory string
}

// browsers is every browser the real-browser test signed in with. The Team IDs are the ones codesign -dv reports for
// the released apps; browsers_test.go pins them.
var browsers = []Browser{
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

const devToolsProbeTimeout = 300 * time.Millisecond

func findBrowser(signature agentharness.Signature) (Browser, bool) {
	for _, browser := range browsers {
		if signature == browser.Signature {
			return browser, true
		}
	}
	return Browser{}, false
}

func listBrowserNames() string {
	names := make([]string, len(browsers))
	for index, browser := range browsers {
		names[index] = browser.Name
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// isDebuggable reports whether any program on the computer could drive the browser's pages through the DevTools
// protocol and read what Hisar enters: it was started with a remote-debugging switch, or its profile has a
// DevToolsActivePort file whose port accepts connections. A process whose arguments cannot be read counts as debuggable.
func isDebuggable(process agentharness.Process, browser Browser, homeDirectory string) bool {
	if len(process.Arguments) == 0 {
		return true
	}
	userDataDirectory := filepath.Join(homeDirectory, browser.UserDataDirectory)
	for _, argument := range process.Arguments[1:] {
		if !strings.HasPrefix(argument, "-") {
			continue
		}
		name, value, _ := strings.Cut(strings.TrimLeft(argument, "-"), "=")
		switch name {
		case "remote-debugging-port", "remote-debugging-pipe":
			return true
		case "user-data-dir":
			userDataDirectory = value
		}
	}
	if !filepath.IsAbs(userDataDirectory) {
		if process.WorkingDirectory == "" {
			return true
		}
		userDataDirectory = filepath.Join(process.WorkingDirectory, userDataDirectory)
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
