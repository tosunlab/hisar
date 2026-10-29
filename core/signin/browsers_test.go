package signin

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/tryhisar/hisar/core/agentharness"
)

func TestBrowsersAreTheSignedReleases(t *testing.T) {
	// codesign -dv on the released apps, October 2026.
	expected := map[string]agentharness.Signature{
		"brave":  {Identifier: "com.brave.Browser", TeamID: "KL8N8XSYF4"},
		"chrome": {Identifier: "com.google.Chrome", TeamID: "EQHXZ8M8AV"},
		"edge":   {Identifier: "com.microsoft.edgemac", TeamID: "UBF8T346G9"},
	}
	if len(macBrowsers) != len(expected) {
		t.Fatalf("%d browsers, want %d", len(macBrowsers), len(expected))
	}
	for id, signature := range expected {
		browser, isFound := macPlatform.findBrowser(agentharness.Process{Signature: signature})
		if !isFound || browser.ID != id {
			t.Errorf("%s: %+v, %v", id, browser, isFound)
		}
	}
	for _, signature := range []agentharness.Signature{
		{},
		{Identifier: "com.google.Chrome"},
		{Identifier: "com.google.Chrome", TeamID: "ABCDE12345"},
		{Identifier: "com.google.chrome.for.testing"},
		{Identifier: "com.apple.Safari", TeamID: "ABCDE12345"},
		{Identifier: "company.thebrowser.Browser", TeamID: "S6N382Y83G"},
	} {
		if browser, isFound := macPlatform.findBrowser(agentharness.Process{Signature: signature}); isFound {
			t.Errorf("%+v is %s", signature, browser.ID)
		}
	}
}

func TestDebuggableBrowsers(t *testing.T) {
	brave, _ := macPlatform.findBrowser(agentharness.Process{Signature: agentharness.Signature{Identifier: "com.brave.Browser", TeamID: "KL8N8XSYF4"}})
	home := t.TempDir()
	process := func(arguments ...string) agentharness.Process {
		return agentharness.Process{Arguments: append([]string{"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser"}, arguments...)}
	}
	for name, arguments := range map[string][]string{
		"a port":                  {"--remote-debugging-port=9222"},
		"any free port":           {"--remote-debugging-port=0"},
		"a pipe":                  {"--remote-debugging-pipe"},
		"a single-dash switch":    {"-remote-debugging-port=9222"},
		"a relative profile":      {"--user-data-dir=profile"},
		"no readable arguments":   nil,
		"a port after a profile":  {"--user-data-dir=/tmp/x", "--remote-debugging-port=9333"},
		"a port among page links": {"https://example.com", "--remote-debugging-port=9222"},
	} {
		running := process(arguments...)
		if arguments == nil {
			running.Arguments = nil
		}
		if !macPlatform.isDebuggable(running, brave, home) {
			t.Errorf("%s: not debuggable", name)
		}
	}
	for name, arguments := range map[string][]string{
		"no switches":        {},
		"an extension":       {"--load-extension=/tmp/extension"},
		"a page link":        {"https://example.com/?remote-debugging-port=1"},
		"another profile":    {"--user-data-dir=" + t.TempDir()},
		"a relative profile": {"--user-data-dir=profile"},
	} {
		running := process(arguments...)
		running.WorkingDirectory = t.TempDir()
		if macPlatform.isDebuggable(running, brave, home) {
			t.Errorf("%s: debuggable", name)
		}
	}
}

func TestDevToolsActivePortCountsWhileItsPortListens(t *testing.T) {
	brave, _ := macPlatform.findBrowser(agentharness.Process{Signature: agentharness.Signature{Identifier: "com.brave.Browser", TeamID: "KL8N8XSYF4"}})
	home := t.TempDir()
	profile := filepath.Join(home, brave.UserDataDirectory)
	if err := os.MkdirAll(profile, 0o700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	activePort := []byte(strconv.Itoa(port) + "\n/devtools/browser/0f6e7d0c-0000-4000-8000-000000000000\n")
	if err := os.WriteFile(filepath.Join(profile, "DevToolsActivePort"), activePort, 0o600); err != nil {
		t.Fatal(err)
	}
	running := agentharness.Process{Arguments: []string{"Brave Browser"}}
	if !macPlatform.isDebuggable(running, brave, home) {
		t.Error("the default profile's live port does not count")
	}
	custom := t.TempDir()
	os.WriteFile(filepath.Join(custom, "DevToolsActivePort"), activePort, 0o600)
	if !macPlatform.isDebuggable(agentharness.Process{Arguments: []string{"Brave Browser", "--user-data-dir=" + custom}}, brave, home) {
		t.Error("a --user-data-dir profile's live port does not count")
	}
	listener.Close()
	if macPlatform.isDebuggable(running, brave, home) {
		t.Error("a file left behind by a browser that quit counts")
	}
}
