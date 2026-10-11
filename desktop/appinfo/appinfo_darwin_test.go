//go:build darwin && !ios && cgo

package appinfo

import (
	"bytes"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const terminalBundlePath = "/System/Applications/Utilities/Terminal.app"

func TestDescribeTerminalOnMacOS(t *testing.T) {
	resolver := CreateSystemResolver()
	app := resolver.DescribeApp(terminalBundlePath)
	if app.Name != "Terminal" || !strings.HasPrefix(app.IconURL, "data:image/png;base64,") || len(app.IconURL) < 1000 {
		t.Errorf("Terminal: name %q, icon %d bytes", app.Name, len(app.IconURL))
	}
	if app.Signature.Status != SignatureStatusApple {
		t.Errorf("Terminal's signature: %+v", app.Signature)
	}
	icon, err := resolver.GetIconPNG(terminalBundlePath, PhoneIconSize)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(icon))
	if err != nil || decoded.Bounds().Dx() != PhoneIconSize || decoded.Bounds().Dy() != PhoneIconSize {
		t.Errorf("phone icon: %v, %v", decoded.Bounds(), err)
	}
}

func TestSignatureOnMacOS(t *testing.T) {
	if signature := getSignature("/usr/bin/ssh-keygen"); signature.Status != SignatureStatusApple {
		t.Errorf("ssh-keygen: %+v", signature)
	}
	unsignedPath := t.TempDir() + "/unsigned"
	if err := os.WriteFile(unsignedPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if signature := getSignature(unsignedPath); signature.Status != SignatureStatusUnsigned {
		t.Errorf("a script: %+v", signature)
	}
	if signature := getSignature(t.TempDir() + "/missing"); signature.Status != SignatureStatusInvalid {
		t.Errorf("a missing file: %+v", signature)
	}
	if teamID := getTeamID("/usr/bin/ssh-keygen"); teamID != "" {
		t.Errorf("an Apple tool has the Team ID %q", teamID)
	}
}

func TestReadThisProcessOnMacOS(t *testing.T) {
	allArgument, err := ListAllProcessArgument(os.Getpid())
	if err != nil || !slices.Equal(allArgument, os.Args) {
		t.Errorf("arguments %q, want %q: %v", allArgument, os.Args, err)
	}
	workingDirectory, err := GetProcessWorkingDirectory(os.Getpid())
	expected, _ := os.Getwd()
	if err != nil || workingDirectory != expected {
		t.Errorf("working directory %q, want %q: %v", workingDirectory, expected, err)
	}
	if _, err := GetProcessWorkingDirectory(999999); err == nil {
		t.Error("a missing process has a working directory")
	}
	executablePath, _ := os.Executable()
	executablePath, _ = filepath.EvalSymlinks(executablePath)
	if path, err := GetProcessPath(os.Getpid()); err != nil || path != executablePath {
		t.Errorf("path %q, want %q: %v", path, executablePath, err)
	}
	if allProcessID, err := listAllProcessID(); err != nil || !slices.Contains(allProcessID, os.Getpid()) {
		t.Errorf("this process is not running: %v", err)
	}
}

func TestResolveChildOfThisProcessOnMacOS(t *testing.T) {
	command := exec.Command("/bin/sleep", "5")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer command.Process.Kill()
	requester := CreateSystemResolver().Resolve(command.Process.Pid)
	if len(requester.AllChainProcess) == 0 || GetProgramName(requester.AllChainProcess[0]) != "sleep" {
		t.Fatalf("chain %+v", requester.AllChainProcess)
	}
	if requester.Name == "" || requester.Kind == "" {
		t.Errorf("requester %+v", requester)
	}
	if requester.Kind == RequesterKindApp && (requester.App.Path == "" || requester.App.IconURL == "" || requester.App.Signature.Status == "") {
		t.Errorf("app %+v", requester.App)
	}
	logRequester(t, "a child of this test", CreateSystemResolver(), command.Process.Pid)
}

func logRequester(t *testing.T, label string, resolver *Resolver, pid int) Requester {
	t.Helper()
	startedAt := time.Now()
	requester := resolver.Resolve(pid)
	firstDuration := time.Since(startedAt)
	startedAt = time.Now()
	resolver.Resolve(pid)
	cachedDuration := time.Since(startedAt)
	icon, _ := resolver.GetIconPNG(requester.App.Path, PhoneIconSize)
	t.Logf("%s, pid %d: %q (%s) %s, dialog icon %d bytes as a data URL, phone icon %d bytes, signature %+v, chain %q, resolved in %v, then %v cached",
		label, pid, requester.Name, requester.Kind, requester.App.Path, len(requester.App.IconURL), len(icon), requester.App.Signature, listAllProgramName(requester.AllChainProcess),
		firstDuration.Round(time.Millisecond), cachedDuration.Round(time.Millisecond))
	return requester
}

// TestResolveRunningAppsOnMacOS resolves shells and tools that other apps on this Mac started, when those apps are running.
func TestResolveRunningAppsOnMacOS(t *testing.T) {
	resolver := CreateSystemResolver()
	allProcessID, err := listAllProcessID()
	if err != nil {
		t.Fatal(err)
	}
	allLabelByMarker := map[string]string{"/iTermServer-": "a shell in iTerm2", "/Visual Studio Code.app/Contents/Frameworks/Code Helper (Plugin).app/": "Visual Studio Code's extension host"}
	allFoundLabel := map[string]bool{}
	for _, pid := range allProcessID {
		parentPID, err := resolver.system.GetParentPID(pid)
		if err != nil || parentPID <= launchdPID {
			continue
		}
		path, _ := GetProcessPath(pid)
		grandparentPID, _ := resolver.system.GetParentPID(parentPID)
		ancestorPath, _ := GetProcessPath(grandparentPID)
		if getOutermostBundlePath(path) != "" {
			ancestorPath = path
		}
		for marker, label := range allLabelByMarker {
			if allFoundLabel[label] || !strings.Contains(ancestorPath, marker) {
				continue
			}
			allFoundLabel[label] = true
			requester := logRequester(t, label, resolver, pid)
			if requester.Kind != RequesterKindApp || requester.App.IconURL == "" || requester.App.Signature.Status != SignatureStatusDeveloper {
				t.Errorf("%s: %+v", label, requester)
			}
		}
	}
	for _, label := range allLabelByMarker {
		if !allFoundLabel[label] {
			t.Logf("%s: not running", label)
		}
	}
}
