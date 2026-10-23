package mcpserver

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

const childTestName = "^TestStopEndsTheProcessesTheCommandStarted$"

// TestStopEndsTheProcessesTheCommandStarted runs this test binary as a command that starts a child, as a command
// passes its secrets on to a child, and stops the command: the child must end with it.
func TestStopEndsTheProcessesTheCommandStarted(t *testing.T) {
	switch os.Getenv("HISAR_TEST_PROCESS") {
	case "command":
		child := exec.Command(os.Args[0], "-test.run="+childTestName)
		child.Env = append(os.Environ(), "HISAR_TEST_PROCESS=child")
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		os.WriteFile(os.Getenv("HISAR_TEST_PID_PATH"), []byte(strconv.Itoa(child.Process.Pid)), 0o600)
		time.Sleep(time.Minute)
		return
	case "child":
		time.Sleep(time.Minute)
		return
	}
	group, waitResult, child := startCommandWithChild(t)
	defer group.close()
	group.stop(waitResult, 0)
	if event, err := windows.WaitForSingleObject(child, 5000); event != windows.WAIT_OBJECT_0 {
		t.Errorf("the command's child still runs after the command was stopped: %d %v", event, err)
	}
}

// TestHisarExitingEndsTheProcessesTheCommandStarted closes the job's handle as Windows does when hisar mcp exits or
// crashes while a command runs: the command and its child must end too.
func TestHisarExitingEndsTheProcessesTheCommandStarted(t *testing.T) {
	group, _, child := startCommandWithChild(t)
	windows.CloseHandle(group.job)
	if event, err := windows.WaitForSingleObject(child, 5000); event != windows.WAIT_OBJECT_0 {
		t.Errorf("the command's child still runs after hisar mcp let go of its job: %d %v", event, err)
	}
}

// startCommandWithChild starts this test binary as a command in a job, and returns once the command started its
// child, with a handle to wait for the child.
func startCommandWithChild(t *testing.T) (processGroup, <-chan error, windows.Handle) {
	t.Helper()
	pidPath := filepath.Join(t.TempDir(), "child.pid")
	command := exec.Command(os.Args[0], "-test.run="+childTestName)
	command.Env = append(os.Environ(), "HISAR_TEST_PROCESS=command", "HISAR_TEST_PID_PATH="+pidPath)
	group, err := startProcessGroup(command)
	if err != nil {
		t.Fatal(err)
	}
	waitResult := make(chan error, 1)
	go func() { waitResult <- command.Wait() }()
	var childPID int
	for deadline := time.Now().Add(10 * time.Second); childPID == 0; time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the command did not start its child")
		}
		content, _ := os.ReadFile(pidPath)
		childPID, _ = strconv.Atoi(string(content))
	}
	child, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(childPID))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { windows.CloseHandle(child) })
	return group, waitResult, child
}
