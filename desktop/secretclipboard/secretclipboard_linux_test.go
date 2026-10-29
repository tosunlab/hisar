//go:build linux && cgo

package secretclipboard

import (
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
)

// The tests read the clipboard the way other programs do, with xclip on X11 and wl-paste on Wayland; CI's desktop-linux
// job runs them on Xvfb and on a headless sway.

func openFor(t *testing.T, server Server, variable string, tools ...string) *Clipboard {
	t.Helper()
	if os.Getenv(variable) == "" {
		t.Skipf("no %s", variable)
	}
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("no %s", tool)
		}
	}
	clipboard, err := Open(server)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(clipboard.Close)
	return clipboard
}

func read(t *testing.T, name string, arguments ...string) (string, error) {
	t.Helper()
	output, err := exec.Command(name, arguments...).Output()
	return string(output), err
}

func mustRead(t *testing.T, name string, arguments ...string) string {
	t.Helper()
	output, err := read(t, name, arguments...)
	if err != nil {
		t.Fatalf("%s %s: %v", name, strings.Join(arguments, " "), err)
	}
	return output
}

// waitUntil polls for a change another program makes: the clipboard's new owner tells Hisar asynchronously.
func waitUntil(t *testing.T, what string, condition func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if condition() {
			return
		}
	}
	t.Fatalf("timed out waiting until %s", what)
}

func TestX11CopyIsMarkedSecretAndPastes(t *testing.T) {
	clipboard := openFor(t, X11, "DISPLAY", "xclip")
	const password = "correct horse battery staple"
	if err := clipboard.SetText(password); err != nil {
		t.Fatal(err)
	}
	targets := strings.Fields(mustRead(t, "xclip", "-selection", "clipboard", "-t", "TARGETS", "-o"))
	for _, target := range []string{"x-kde-passwordManagerHint", "UTF8_STRING", "text/plain;charset=utf-8", "STRING"} {
		if !slices.Contains(targets, target) {
			t.Errorf("TARGETS %v lacks %s", targets, target)
		}
	}
	if hint := mustRead(t, "xclip", "-selection", "clipboard", "-t", "x-kde-passwordManagerHint", "-o"); hint != "secret" {
		t.Errorf("hint = %q", hint)
	}
	for _, target := range []string{"UTF8_STRING", "STRING", "text/plain;charset=utf-8"} {
		if pasted := mustRead(t, "xclip", "-selection", "clipboard", "-t", target, "-o"); pasted != password {
			t.Errorf("%s pasted %q", target, pasted)
		}
	}
	if text, ok := clipboard.GetText(); !ok || text != password {
		t.Errorf("GetText = %q, %v", text, ok)
	}
}

func TestX11NonASCIICopyIsNotOfferedAsLatin1(t *testing.T) {
	clipboard := openFor(t, X11, "DISPLAY", "xclip")
	const password = "şifre-ğüç"
	if err := clipboard.SetText(password); err != nil {
		t.Fatal(err)
	}
	if targets := strings.Fields(mustRead(t, "xclip", "-selection", "clipboard", "-t", "TARGETS", "-o")); slices.Contains(targets, "STRING") {
		t.Errorf("TARGETS %v offers STRING", targets)
	}
	if pasted := mustRead(t, "xclip", "-selection", "clipboard", "-t", "UTF8_STRING", "-o"); pasted != password {
		t.Errorf("pasted %q", pasted)
	}
}

func TestX11AnotherProgramsCopyEndsHisarsCopy(t *testing.T) {
	clipboard := openFor(t, X11, "DISPLAY", "xclip")
	if err := clipboard.SetText("hunter2"); err != nil {
		t.Fatal(err)
	}
	other := exec.Command("xclip", "-selection", "clipboard", "-loops", "1")
	other.Stdin = strings.NewReader("an ordinary copy")
	if err := other.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { other.Process.Kill(); other.Wait() })
	waitUntil(t, "Hisar lost the clipboard", func() bool { _, ok := clipboard.GetText(); return !ok })
	// Clearing after another program copied leaves its copy alone.
	if err := clipboard.SetText(""); err != nil {
		t.Fatal(err)
	}
	if pasted := mustRead(t, "xclip", "-selection", "clipboard", "-o"); pasted != "an ordinary copy" {
		t.Errorf("pasted %q after Hisar cleared", pasted)
	}
}

func TestX11EmptyTextGivesTheClipboardUp(t *testing.T) {
	clipboard := openFor(t, X11, "DISPLAY", "xclip")
	if err := clipboard.SetText("hunter2"); err != nil {
		t.Fatal(err)
	}
	if err := clipboard.SetText(""); err != nil {
		t.Fatal(err)
	}
	if _, ok := clipboard.GetText(); ok {
		t.Error("GetText answers after the clipboard was given up")
	}
	if pasted, err := read(t, "xclip", "-selection", "clipboard", "-o"); err == nil && pasted != "" {
		t.Errorf("the clipboard still holds %q", pasted)
	}
}

func TestWaylandCopyIsMarkedSecretAndPastes(t *testing.T) {
	clipboard := openFor(t, Wayland, "WAYLAND_DISPLAY", "wl-paste", "wl-copy")
	const password = "correct horse battery staple"
	if err := clipboard.SetText(password); err != nil {
		t.Fatal(err)
	}
	var types []string
	waitUntil(t, "the compositor offers Hisar's copy", func() bool {
		output, _ := read(t, "wl-paste", "--list-types")
		types = strings.Fields(output)
		return slices.Contains(types, "x-kde-passwordManagerHint")
	})
	for _, mimeType := range []string{"text/plain;charset=utf-8", "text/plain"} {
		if !slices.Contains(types, mimeType) {
			t.Errorf("types %v lack %s", types, mimeType)
		}
	}
	if hint := mustRead(t, "wl-paste", "--no-newline", "--type", "x-kde-passwordManagerHint"); hint != "secret" {
		t.Errorf("hint = %q", hint)
	}
	if pasted := mustRead(t, "wl-paste", "--no-newline", "--type", "text/plain;charset=utf-8"); pasted != password {
		t.Errorf("pasted %q", pasted)
	}

	if err := exec.Command("wl-copy", "an ordinary copy").Run(); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "Hisar lost the clipboard", func() bool { _, ok := clipboard.GetText(); return !ok })
	if err := clipboard.SetText(""); err != nil {
		t.Fatal(err)
	}
	if pasted := mustRead(t, "wl-paste", "--no-newline"); pasted != "an ordinary copy" {
		t.Errorf("pasted %q after Hisar cleared", pasted)
	}
}
