package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tryhisar/hisar/cli/internal/dotenv"
)

const helperEnvironment = "HISAR_TEST_RUN_HELPER"

func TestMain(m *testing.M) {
	if mode := os.Getenv(helperEnvironment); mode != "" {
		os.Exit(runTestHelper(mode))
	}
	os.Exit(m.Run())
}

// runTestHelper is the command hisar run starts in these tests: the test binary itself in a helper mode.
func runTestHelper(mode string) int {
	secret := os.Getenv("SECRET")
	switch mode {
	case "print":
		fmt.Printf("stdout: %s\n", secret)
		fmt.Fprintf(os.Stderr, "stderr: %s\n", secret)
		if hashPath := os.Getenv("HASH_FILE"); hashPath != "" {
			hash := sha256.Sum256([]byte(secret))
			os.WriteFile(hashPath, []byte(hex.EncodeToString(hash[:])), 0o600)
		}
		exitCode, _ := strconv.Atoi(os.Getenv("EXIT_CODE"))
		return exitCode
	case "split":
		middle := len(secret) / 2
		os.Stdout.WriteString("split: " + secret[:middle])
		time.Sleep(30 * time.Millisecond)
		os.Stdout.WriteString(secret[middle:] + "\n")
		return 0
	case "stdin":
		data, _ := io.ReadAll(os.Stdin)
		os.Stdout.Write(data)
		return 0
	case "stdout-kind":
		information, _ := os.Stdout.Stat()
		if information.Mode().IsRegular() {
			fmt.Println("regular file")
		} else {
			fmt.Println("pipe")
		}
		return 0
	case "environment":
		for _, name := range []string{"SECRET", "PLAIN", "FROM_FILE", "OVERRIDDEN"} {
			fmt.Printf("%s=%s\n", name, os.Getenv(name))
		}
		return 0
	}
	return runSignalHelper(mode)
}

func createHelperEnvironment(mode string, extras ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH"), helperEnvironment + "=" + mode}, extras...)
}

type recordingResolver struct {
	valuesByReference map[string]string
	calls             [][]string
	commandLines      [][]string
	err               error
}

func (resolver *recordingResolver) resolve(_ context.Context, references []string, commandLine []string) ([]string, error) {
	resolver.calls = append(resolver.calls, references)
	resolver.commandLines = append(resolver.commandLines, commandLine)
	if resolver.err != nil {
		return nil, resolver.err
	}
	values := make([]string, 0, len(references))
	for _, reference := range references {
		values = append(values, resolver.valuesByReference[reference])
	}
	return values, nil
}

type capturedStdio struct {
	stdio      runStdio
	stdoutPath string
	stderrPath string
}

func createCapturedStdio(t *testing.T, stdinText string) capturedStdio {
	t.Helper()
	directory := t.TempDir()
	captured := capturedStdio{stdoutPath: filepath.Join(directory, "stdout"), stderrPath: filepath.Join(directory, "stderr")}
	stdinPath := filepath.Join(directory, "stdin")
	os.WriteFile(stdinPath, []byte(stdinText), 0o600)
	open := func(path string, flag int) *os.File {
		file, err := os.OpenFile(path, flag, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { file.Close() })
		return file
	}
	captured.stdio = runStdio{
		stdin:  open(stdinPath, os.O_RDONLY),
		stdout: open(captured.stdoutPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC),
		stderr: open(captured.stderrPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC),
	}
	return captured
}

func (captured capturedStdio) read(t *testing.T) (string, string) {
	t.Helper()
	stdout, _ := os.ReadFile(captured.stdoutPath)
	stderr, _ := os.ReadFile(captured.stderrPath)
	return string(stdout), string(stderr)
}

func getTestBinary(t *testing.T) string {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunMasksTheSecretAndReturnsTheExitCode(t *testing.T) {
	const secret = "correct-horse-battery-staple"
	hashPath := filepath.Join(t.TempDir(), "hash")
	environmentFile := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(environmentFile, []byte("SECRET=hisar://Personal/Example/password\nEXIT_CODE=3\nHASH_FILE="+hashPath+"\n"), 0o600)
	resolver := &recordingResolver{valuesByReference: map[string]string{"hisar://Personal/Example/password": secret}}
	captured := createCapturedStdio(t, "")
	option := runOption{environmentFiles: []string{environmentFile}, commandArgs: []string{getTestBinary(t), "--flag", "it's"}}
	exitCode, err := runWithSecret(context.Background(), option, createHelperEnvironment("print"), resolver.resolve, captured.stdio)
	if err != nil || exitCode != 3 {
		t.Fatalf("exit code %d, %v", exitCode, err)
	}
	stdout, stderr := captured.read(t)
	if stdout != "stdout: <concealed by hisar>\n" || stderr != "stderr: <concealed by hisar>\n" {
		t.Errorf("stdout %q, stderr %q", stdout, stderr)
	}
	expectedHash := sha256.Sum256([]byte(secret))
	if hash, _ := os.ReadFile(hashPath); string(hash) != hex.EncodeToString(expectedHash[:]) {
		t.Errorf("the command did not get the value: %q", hash)
	}
	if len(resolver.calls) != 1 || !slices.Equal(resolver.commandLines[0], option.commandArgs) {
		t.Errorf("requests %q, command lines %q", resolver.calls, resolver.commandLines)
	}
}

func TestRunMasksAValueWrittenInTwoParts(t *testing.T) {
	resolver := &recordingResolver{valuesByReference: map[string]string{"hisar://Personal/Example/password": "split-across-writes"}}
	captured := createCapturedStdio(t, "")
	environment := createHelperEnvironment("split", "SECRET=hisar://Personal/Example/password")
	if _, err := runWithSecret(context.Background(), runOption{commandArgs: []string{getTestBinary(t)}}, environment, resolver.resolve, captured.stdio); err != nil {
		t.Fatal(err)
	}
	if stdout, _ := captured.read(t); stdout != "split: <concealed by hisar>\n" {
		t.Errorf("stdout %q", stdout)
	}
}

func TestRunWithoutMaskingConnectsTheCommandDirectly(t *testing.T) {
	resolver := &recordingResolver{valuesByReference: map[string]string{"hisar://Personal/Example/password": "shown-as-is"}}
	captured := createCapturedStdio(t, "")
	environment := createHelperEnvironment("print", "SECRET=hisar://Personal/Example/password", "EXIT_CODE=0")
	if _, err := runWithSecret(context.Background(), runOption{isMaskingDisabled: true, commandArgs: []string{getTestBinary(t)}}, environment, resolver.resolve, captured.stdio); err != nil {
		t.Fatal(err)
	}
	if stdout, stderr := captured.read(t); stdout != "stdout: shown-as-is\n" || stderr != "stderr: shown-as-is\n" {
		t.Errorf("stdout %q, stderr %q", stdout, stderr)
	}

	for _, testCase := range []struct {
		isMaskingDisabled bool
		expected          string
	}{{true, "regular file\n"}, {false, "pipe\n"}} {
		captured := createCapturedStdio(t, "")
		runWithSecret(context.Background(), runOption{isMaskingDisabled: testCase.isMaskingDisabled, commandArgs: []string{getTestBinary(t)}}, createHelperEnvironment("stdout-kind"), resolver.resolve, captured.stdio)
		if stdout, _ := captured.read(t); stdout != testCase.expected {
			t.Errorf("masking off %v: the command's stdout is a %q", testCase.isMaskingDisabled, stdout)
		}
	}
}

func TestRunPassesStdinThrough(t *testing.T) {
	captured := createCapturedStdio(t, "typed into the command\n")
	if _, err := runWithSecret(context.Background(), runOption{commandArgs: []string{getTestBinary(t)}}, createHelperEnvironment("stdin"), (&recordingResolver{}).resolve, captured.stdio); err != nil {
		t.Fatal(err)
	}
	if stdout, stderr := captured.read(t); stdout != "typed into the command\n" || stderr != "hisar: no secret references found\n" {
		t.Errorf("stdout %q, stderr %q", stdout, stderr)
	}
}

func TestRunWithoutReferencesStillRuns(t *testing.T) {
	resolver := &recordingResolver{}
	captured := createCapturedStdio(t, "")
	exitCode, err := runWithSecret(context.Background(), runOption{commandArgs: []string{getTestBinary(t)}}, createHelperEnvironment("print", "SECRET=plain value", "EXIT_CODE=5"), resolver.resolve, captured.stdio)
	if err != nil || exitCode != 5 || len(resolver.calls) != 0 {
		t.Fatalf("exit code %d, %v, %d requests", exitCode, err, len(resolver.calls))
	}
	if stdout, stderr := captured.read(t); stdout != "stdout: plain value\n" || stderr != "hisar: no secret references found\nstderr: plain value\n" {
		t.Errorf("stdout %q, stderr %q", stdout, stderr)
	}
}

func TestRunEnvironmentFromFilesAndReferences(t *testing.T) {
	directory := t.TempDir()
	firstFile, secondFile := filepath.Join(directory, "first.env"), filepath.Join(directory, "second.env")
	os.WriteFile(firstFile, []byte("FROM_FILE=first\nPLAIN=from a file\nOVERRIDDEN=hisar://Personal/First/password\n"), 0o600)
	os.WriteFile(secondFile, []byte("export FROM_FILE=\"second\"\nOVERRIDDEN=hisar://Personal/Second/password\n"), 0o600)
	resolver := &recordingResolver{valuesByReference: map[string]string{
		"hisar://Personal/Example/password": "value-one", "hisar://Personal/Second/password": "value-two",
	}}
	captured := createCapturedStdio(t, "")
	environment := createHelperEnvironment("environment", "SECRET=hisar://Personal/Example/password", "PLAIN=from the environment", "OVERRIDDEN=hisar://Personal/Environment/password")
	option := runOption{isMaskingDisabled: true, environmentFiles: []string{firstFile, secondFile}, commandArgs: []string{getTestBinary(t)}}
	if _, err := runWithSecret(context.Background(), option, environment, resolver.resolve, captured.stdio); err != nil {
		t.Fatal(err)
	}
	if stdout, _ := captured.read(t); stdout != "SECRET=value-one\nPLAIN=from the environment\nFROM_FILE=second\nOVERRIDDEN=value-two\n" {
		t.Errorf("environment:\n%s", stdout)
	}
	if len(resolver.calls) != 1 || !slices.Equal(resolver.calls[0], []string{"hisar://Personal/Example/password", "hisar://Personal/Second/password"}) {
		t.Errorf("one request with both references: %q", resolver.calls)
	}
}

func TestRunWarnsAboutShortValues(t *testing.T) {
	resolver := &recordingResolver{valuesByReference: map[string]string{"hisar://Personal/Pin/password": "123"}}
	captured := createCapturedStdio(t, "")
	environment := createHelperEnvironment("print", "SECRET=hisar://Personal/Pin/password", "EXIT_CODE=0")
	runWithSecret(context.Background(), runOption{commandArgs: []string{getTestBinary(t)}}, environment, resolver.resolve, captured.stdio)
	if stdout, stderr := captured.read(t); stdout != "stdout: 123\n" || !strings.HasPrefix(stderr, "hisar: SECRET is shorter than 4 bytes and is not masked\n") {
		t.Errorf("stdout %q, stderr %q", stdout, stderr)
	}
}

func TestRunFailsBeforeStartingOnAResolveError(t *testing.T) {
	hashPath := filepath.Join(t.TempDir(), "hash")
	resolver := &recordingResolver{err: errors.New("hisar://Personal/Missing/password: no item in vault \"Personal\" is titled \"Missing\"")}
	captured := createCapturedStdio(t, "")
	environment := createHelperEnvironment("print", "SECRET=hisar://Personal/Missing/password", "HASH_FILE="+hashPath)
	if _, err := runWithSecret(context.Background(), runOption{commandArgs: []string{getTestBinary(t)}}, environment, resolver.resolve, captured.stdio); err == nil || !strings.Contains(err.Error(), "Missing") {
		t.Errorf("error %v", err)
	}
	if _, err := os.Stat(hashPath); !errors.Is(err, os.ErrNotExist) {
		t.Error("the command ran although a reference did not resolve")
	}
}

func TestRunReportsAMissingCommandBeforeResolving(t *testing.T) {
	resolver := &recordingResolver{}
	captured := createCapturedStdio(t, "")
	exitCode, err := runWithSecret(context.Background(), runOption{commandArgs: []string{"hisar-no-such-command"}}, []string{"PATH=" + os.Getenv("PATH"), "SECRET=hisar://Personal/Example/password"}, resolver.resolve, captured.stdio)
	if err != nil || exitCode != 127 || len(resolver.calls) != 0 {
		t.Errorf("exit code %d, %v, %d requests", exitCode, err, len(resolver.calls))
	}
	if _, stderr := captured.read(t); stderr != "hisar: hisar-no-such-command: command not found\n" {
		t.Errorf("stderr %q", stderr)
	}
	notExecutable := filepath.Join(t.TempDir(), "not-executable")
	os.WriteFile(notExecutable, []byte("#!/bin/sh\n"), 0o600)
	if exitCode, _ := runWithSecret(context.Background(), runOption{commandArgs: []string{notExecutable}}, nil, resolver.resolve, captured.stdio); exitCode != 126 {
		t.Errorf("not executable: exit code %d", exitCode)
	}
}

func TestParseRunOption(t *testing.T) {
	option, err := parseRunOption(createTestFlagSet(), []string{"--env-file", "a.env", "--env-file=b.env", "--no-masking", "--", "npm", "test", "--env-file", "c"})
	if err != nil || !option.isMaskingDisabled || !slices.Equal(option.environmentFiles, []string{"a.env", "b.env"}) || !slices.Equal(option.commandArgs, []string{"npm", "test", "--env-file", "c"}) {
		t.Errorf("option %+v, %v", option, err)
	}
	if option, err := parseRunOption(createTestFlagSet(), []string{"npm", "--no-masking"}); err != nil || option.isMaskingDisabled || !slices.Equal(option.commandArgs, []string{"npm", "--no-masking"}) {
		t.Errorf("without --: %+v, %v", option, err)
	}
	if _, err := parseRunOption(createTestFlagSet(), []string{"--no-masking", "--"}); !errors.Is(err, errUsage) {
		t.Errorf("no command: %v", err)
	}
}

func TestCheckAllReferenceNamesEveryBadReference(t *testing.T) {
	err := checkReferences([]string{"hisar://a/b/c", "hisar://a/b", "hisar://a/%zz/c"})
	if err == nil || !strings.Contains(err.Error(), "hisar://a/b: a secret reference has three parts") || !strings.Contains(err.Error(), `hisar://a/%zz/c: "%zz" is not a valid percent-encoding`) {
		t.Errorf("error %v", err)
	}
}

func TestBuildRunEnvironment(t *testing.T) {
	environ := []string{
		"PATH=/usr/bin",
		"PLAIN_IN_BOTH=from the environment",
		"REFERENCE_IN_BOTH=hisar://Personal/Environment/password",
		"REFERENCE_ONLY_HERE=hisar://Personal/Only/password",
		"BROKEN_ENTRY",
	}
	fileVariables := []dotenv.Variable{
		{Name: "PLAIN_IN_BOTH", Value: "from the first file"},
		{Name: "REFERENCE_IN_BOTH", Value: "hisar://Personal/First/password"},
		{Name: "FILE_ONLY", Value: "first"},
		{Name: "REFERENCE_IN_BOTH", Value: "hisar://Personal/Second/password"},
		{Name: "FILE_ONLY", Value: "second"},
	}
	variables := buildRunEnvironment(environ, fileVariables)
	expectedValues := []dotenv.Variable{
		{Name: "PATH", Value: "/usr/bin"},
		{Name: "PLAIN_IN_BOTH", Value: "from the environment"},
		{Name: "REFERENCE_IN_BOTH", Value: "hisar://Personal/Second/password"},
		{Name: "REFERENCE_ONLY_HERE", Value: "hisar://Personal/Only/password"},
		{Name: "FILE_ONLY", Value: "second"},
	}
	if !slices.Equal(variables, expectedValues) {
		t.Errorf("environment:\n%q\nwant:\n%q", variables, expectedValues)
	}
	references := listUniqueReferences(append(variables, dotenv.Variable{Name: "AGAIN", Value: "hisar://Personal/Only/password"}))
	if !slices.Equal(references, []string{"hisar://Personal/Second/password", "hisar://Personal/Only/password"}) {
		t.Errorf("references %q", references)
	}
}
