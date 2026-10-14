//go:build !windows

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"
)

func createTestFlagSet() *flag.FlagSet {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	return flags
}

// runSignalHelper: "catch" reports the first forwarded signal and exits 7; "sleep" lets a signal end it.
func runSignalHelper(mode string) int {
	readyPath := os.Getenv("READY_FILE")
	switch mode {
	case "catch":
		signalChannel := make(chan os.Signal, 1)
		signal.Notify(signalChannel, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
		os.WriteFile(readyPath, []byte("ready"), 0o600)
		select {
		case receivedSignal := <-signalChannel:
			fmt.Printf("got %s\n", receivedSignal)
			return 7
		case <-time.After(2 * time.Second):
			fmt.Println("no signal")
			return 8
		}
	case "sleep":
		os.WriteFile(readyPath, []byte("ready"), 0o600)
		time.Sleep(10 * time.Second)
		return 0
	}
	return 99
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the helper did not start")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

type runResult struct {
	exitCode int
	err      error
}

// startHelperRun runs hisar run in this process and sends it a signal once the helper is ready.
func startHelperRun(t *testing.T, mode string, isMaskingDisabled bool, sentSignal syscall.Signal) (runResult, capturedStdio) {
	t.Helper()
	readyPath := t.TempDir() + "/ready"
	captured := createCapturedStdio(t, "")
	resultChannel := make(chan runResult, 1)
	go func() {
		exitCode, err := runWithSecret(context.Background(), runOption{isMaskingDisabled: isMaskingDisabled, commandArgs: []string{getTestBinary(t)}},
			createHelperEnvironment(mode, "READY_FILE="+readyPath), (&recordingResolver{}).resolve, captured.stdio)
		resultChannel <- runResult{exitCode: exitCode, err: err}
	}()
	waitForFile(t, readyPath)
	syscall.Kill(os.Getpid(), sentSignal)
	select {
	case result := <-resultChannel:
		return result, captured
	case <-time.After(15 * time.Second):
		t.Fatal("hisar run did not finish")
		return runResult{}, captured
	}
}

func setForegroundJob(t *testing.T, isForeground bool) {
	t.Helper()
	original := isForegroundJob
	isForegroundJob = func() bool { return isForeground }
	t.Cleanup(func() { isForegroundJob = original })
}

func TestRunForwardsSignals(t *testing.T) {
	setForegroundJob(t, true)
	for _, testCase := range []struct {
		sentSignal        syscall.Signal
		isMaskingDisabled bool
		expected          string
	}{
		{syscall.SIGTERM, false, "got terminated\n"},
		{syscall.SIGHUP, false, "got hangup\n"},
		{syscall.SIGTERM, true, "got terminated\n"},
	} {
		result, captured := startHelperRun(t, "catch", testCase.isMaskingDisabled, testCase.sentSignal)
		stdout, _ := captured.read(t)
		if result.err != nil || result.exitCode != 7 || stdout != testCase.expected {
			t.Errorf("%s: exit code %d, %v, stdout %q", testCase.sentSignal, result.exitCode, result.err, stdout)
		}
	}
}

func TestRunForwardsInterruptOnlyWhenTheTerminalDoesNot(t *testing.T) {
	setForegroundJob(t, false)
	for _, sentSignal := range []syscall.Signal{syscall.SIGINT, syscall.SIGQUIT} {
		result, captured := startHelperRun(t, "catch", false, sentSignal)
		if stdout, _ := captured.read(t); result.exitCode != 7 || stdout != fmt.Sprintf("got %s\n", sentSignal) {
			t.Errorf("%s in the background: exit code %d, stdout %q", sentSignal, result.exitCode, stdout)
		}
	}
	setForegroundJob(t, true)
	result, captured := startHelperRun(t, "catch", false, syscall.SIGINT)
	if stdout, _ := captured.read(t); result.exitCode != 8 || stdout != "no signal\n" {
		t.Errorf("SIGINT in the foreground was forwarded a second time: exit code %d, stdout %q", result.exitCode, stdout)
	}
}

func TestRunExitCodeOfACommandEndedByASignal(t *testing.T) {
	setForegroundJob(t, true)
	result, _ := startHelperRun(t, "sleep", false, syscall.SIGTERM)
	if result.err != nil || result.exitCode != 128+int(syscall.SIGTERM) {
		t.Errorf("exit code %d, %v; want %d", result.exitCode, result.err, 128+int(syscall.SIGTERM))
	}
	result, _ = startHelperRun(t, "sleep", true, syscall.SIGHUP)
	if result.exitCode != 128+int(syscall.SIGHUP) {
		t.Errorf("without masking: exit code %d", result.exitCode)
	}
}

func TestCheckForegroundJobWithoutATerminal(t *testing.T) {
	if _, err := os.Open("/dev/tty"); err != nil && checkForegroundJob() {
		t.Error("a process without a terminal counts as its foreground job")
	}
	if !strings.Contains(fmt.Sprint(forwardedSignals), "interrupt") {
		t.Errorf("forwarded signals %v", forwardedSignals)
	}
}
