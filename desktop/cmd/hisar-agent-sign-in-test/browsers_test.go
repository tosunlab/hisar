package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestLaunchBrowserSelectsTheFixtureWindowBeforeAdmission(t *testing.T) {
	directory := t.TempDir()
	profiles := filepath.Join(directory, "profiles")
	process := exec.Command("/bin/sleep", "30")
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		process.Process.Kill()
		process.Wait()
	})
	pid := process.Process.Pid
	windows := fmt.Sprintf(`{"windows":[{"window_id":1,"pid":%d,"title":"DevTools","bounds":{"width":500,"height":463}},{"window_id":2,"pid":%d,"title":"Plain sign-in","bounds":{"width":1100,"height":750}}]}`, pid, pid)
	for name, script := range map[string]string{
		"open":   "#!/bin/sh\nexit 0\n",
		"ps":     fmt.Sprintf("#!/bin/sh\nfor profile in %q/profile-*; do printf '%d /test-browser --user-data-dir=%%s\\n' \"$profile\"; done\n", profiles, pid),
		"driver": "#!/bin/sh\nprintf '%s\\n' '" + windows + "'\n",
		"yabai": "#!/bin/sh\ncase \"$*\" in\n" +
			"'-m query --windows --window 2') printf '%s\\n' '{\"is-floating\":true,\"frame\":{\"x\":80,\"y\":80,\"w\":1100,\"h\":750}}';;\n" +
			"*) printf '%s\\n' '{\"frame\":{\"x\":1137,\"y\":50,\"w\":500,\"h\":463}}';;\nesac\n",
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", directory)
	t.Setenv("HISAR_TEST_CUA_SOCKET", filepath.Join(directory, "s", "cua.sock"))
	t.Setenv("HISAR_TEST_DISPLAY_LAYOUT", `{"display":1,"visibleSpace":3,"otherSpace":1,"builtInSpaces":[1,3],"frame":{"x":0,"y":0,"w":1512,"h":982}}`)
	launched, err := launchBrowser(cuaDriver{path: filepath.Join(directory, "driver")}, browserApp{Name: "Test browser", App: "/test/Browser.app"}, profiles, "http://127.0.0.1/plain", "Plain sign-in")
	if err != nil {
		t.Fatal(err)
	}
	if launched.window.WindowID != 2 {
		t.Fatalf("selected window %d, want the fixture window 2", launched.window.WindowID)
	}
}
