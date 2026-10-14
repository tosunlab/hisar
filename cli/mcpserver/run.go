package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/tryhisar/hisar/cli/internal/masking"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/secretreference"
)

const (
	DefaultTimeout        = 120 * time.Second
	MaximumTimeout        = 600 * time.Second
	MaximumOutputLength   = 64 << 10
	defaultKillDelay      = 5 * time.Second
	outputDrainDuration   = time.Second
	maximumVariableCount  = cliprotocol.MaxAgentReferenceCount
	maximumArgumentLength = 1 << 20
)

var variableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type RunInput struct {
	Command          []string          `json:"command" jsonschema:"the program and its arguments, e.g. [\"npm\", \"test\"]; it runs without a shell"`
	Env              map[string]string `json:"env" jsonschema:"environment variables to set, each mapped to a hisar://vault/item/field secret reference from search_items"`
	WorkingDirectory string            `json:"workingDirectory,omitempty" jsonschema:"the directory to run in; relative to the directory hisar mcp started in (usually the project), which is also the default"`
	TimeoutSeconds   int               `json:"timeoutSeconds,omitempty" jsonschema:"stop the command after this many seconds; default 120, at most 600"`
}

type RunOutput struct {
	ExitCode          int      `json:"exitCode" jsonschema:"the command's exit code; 128 + the signal number when a signal ended it"`
	Stdout            string   `json:"stdout" jsonschema:"standard output with every secret value replaced by <concealed by hisar>"`
	Stderr            string   `json:"stderr" jsonschema:"standard error with every secret value replaced by <concealed by hisar>"`
	IsStdoutTruncated bool     `json:"isStdoutTruncated" jsonschema:"stdout was longer than 64 KiB; its first and last 32 KiB are kept"`
	IsStderrTruncated bool     `json:"isStderrTruncated" jsonschema:"stderr was longer than 64 KiB; its first and last 32 KiB are kept"`
	DurationSeconds   float64  `json:"durationSeconds"`
	IsTimedOut        bool     `json:"isTimedOut" jsonschema:"the command ran out of time and was stopped"`
	Warnings          []string `json:"warnings,omitempty" jsonschema:"variables whose values are too short to be masked"`
}

// outputCapture keeps the first and the last half of MaximumOutputLength and counts what falls between.
type outputCapture struct {
	head          []byte
	tail          []byte
	omittedLength int
}

func (capture *outputCapture) Write(chunk []byte) (int, error) {
	written := len(chunk)
	half := MaximumOutputLength / 2
	if room := half - len(capture.head); room > 0 {
		taken := min(room, len(chunk))
		capture.head = append(capture.head, chunk[:taken]...)
		chunk = chunk[taken:]
	}
	capture.tail = append(capture.tail, chunk...)
	if excess := len(capture.tail) - half; excess > 0 {
		capture.omittedLength += excess
		capture.tail = append(capture.tail[:0], capture.tail[excess:]...)
	}
	return written, nil
}

func (capture *outputCapture) getText() (string, bool) {
	text := string(capture.head)
	if capture.omittedLength > 0 {
		text += fmt.Sprintf("\n[… %d bytes not shown …]\n", capture.omittedLength)
	}
	return strings.ToValidUTF8(text+string(capture.tail), "�"), capture.omittedLength > 0
}

type preparedCommand struct {
	path             string
	arguments        []string
	workingDirectory string
	variableNames    []string
	references       []string
	timeout          time.Duration
}

// prepareCommand checks everything that can fail before the user is asked: names, references, directory, program.
func prepareCommand(input RunInput, defaultDirectory string) (preparedCommand, error) {
	prepared := preparedCommand{arguments: input.Command, timeout: DefaultTimeout}
	if len(input.Command) == 0 || input.Command[0] == "" {
		return prepared, errors.New("command is empty; pass the program and its arguments, e.g. [\"npm\", \"test\"]")
	}
	if len(input.Env) == 0 {
		return prepared, errors.New("env is empty; map at least one variable to a hisar:// reference (run commands without secrets with your own tools)")
	}
	if len(input.Env) > maximumVariableCount {
		return prepared, fmt.Errorf("at most %d variables can come from Hisar at once", maximumVariableCount)
	}
	totalLength := 0
	for _, argument := range input.Command {
		totalLength += len(argument)
		if strings.ContainsRune(argument, 0) {
			return prepared, errors.New("an argument contains a NUL byte")
		}
	}
	if totalLength > maximumArgumentLength {
		return prepared, errors.New("the command's arguments are longer than 1 MiB")
	}
	for name := range input.Env {
		prepared.variableNames = append(prepared.variableNames, name)
	}
	slices.Sort(prepared.variableNames)
	var failures []string
	for _, name := range prepared.variableNames {
		reference := input.Env[name]
		switch {
		case !variableNamePattern.MatchString(name):
			failures = append(failures, fmt.Sprintf("%q is not a valid variable name", name))
		case !secretreference.IsReference(reference):
			failures = append(failures, fmt.Sprintf("%s: the value must be a hisar://vault/item/field reference", name))
		default:
			if _, err := secretreference.Parse(reference); err != nil {
				failures = append(failures, fmt.Sprintf("%s: %s: %v", name, reference, err))
			}
		}
		prepared.references = append(prepared.references, reference)
	}
	if len(failures) > 0 {
		return prepared, errors.New(strings.Join(failures, "; "))
	}
	if input.TimeoutSeconds < 0 || time.Duration(input.TimeoutSeconds)*time.Second > MaximumTimeout {
		return prepared, fmt.Errorf("timeoutSeconds must be between 1 and %d", int(MaximumTimeout.Seconds()))
	}
	if input.TimeoutSeconds > 0 {
		prepared.timeout = time.Duration(input.TimeoutSeconds) * time.Second
	}
	prepared.workingDirectory = defaultDirectory
	if input.WorkingDirectory != "" {
		prepared.workingDirectory = input.WorkingDirectory
		if !filepath.IsAbs(prepared.workingDirectory) {
			prepared.workingDirectory = filepath.Join(defaultDirectory, prepared.workingDirectory)
		}
	}
	prepared.workingDirectory = filepath.Clean(prepared.workingDirectory)
	if directoryInfo, err := os.Stat(prepared.workingDirectory); err != nil || !directoryInfo.IsDir() {
		return prepared, fmt.Errorf("working directory %s does not exist", prepared.workingDirectory)
	}
	program := input.Command[0]
	if strings.ContainsRune(program, filepath.Separator) && !filepath.IsAbs(program) {
		program = filepath.Join(prepared.workingDirectory, program)
	}
	path, err := exec.LookPath(program)
	switch {
	case errors.Is(err, fs.ErrPermission):
		return prepared, fmt.Errorf("%s: permission denied", input.Command[0])
	case err != nil && !errors.Is(err, exec.ErrDot):
		return prepared, fmt.Errorf("%s: command not found", input.Command[0])
	}
	prepared.path = path
	return prepared, nil
}

// buildEnvironment replaces or adds the variables that come from Hisar.
func buildEnvironment(baseEntries []string, valuesByName map[string]string) []string {
	entries := make([]string, 0, len(baseEntries)+len(valuesByName))
	for _, entry := range baseEntries {
		name, _, _ := strings.Cut(entry, "=")
		if _, isReplaced := valuesByName[name]; !isReplaced {
			entries = append(entries, entry)
		}
	}
	names := make([]string, 0, len(valuesByName))
	for name := range valuesByName {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		entries = append(entries, name+"="+valuesByName[name])
	}
	return entries
}

// runPrepared starts the command in its own process group, masks its output, and stops the group at the timeout.
func runPrepared(ctx context.Context, prepared preparedCommand, environmentEntries []string, secretValues []string, killDelay time.Duration) (RunOutput, error) {
	var stdout, stderr outputCapture
	stdoutMasker := masking.NewSecretMasker(&stdout, secretValues)
	stderrMasker := masking.NewSecretMasker(&stderr, secretValues)
	command := exec.Command(prepared.path, prepared.arguments[1:]...)
	command.Args[0] = prepared.arguments[0]
	command.Dir = prepared.workingDirectory
	command.Env = environmentEntries
	command.Stdout, command.Stderr = stdoutMasker, stderrMasker
	command.WaitDelay = outputDrainDuration
	startInProcessGroup(command)
	startedAt := time.Now()
	if err := command.Start(); err != nil {
		return RunOutput{}, fmt.Errorf("%s: %v", prepared.arguments[0], err)
	}
	waitResult := make(chan error, 1)
	go func() { waitResult <- command.Wait() }()
	timer := time.NewTimer(prepared.timeout)
	defer timer.Stop()
	output := RunOutput{}
	var waitError error
	select {
	case waitError = <-waitResult:
	case <-timer.C:
		output.IsTimedOut = true
		waitError = stopProcessGroup(command, waitResult, killDelay)
	case <-ctx.Done():
		waitError = stopProcessGroup(command, waitResult, killDelay)
	}
	output.DurationSeconds = time.Since(startedAt).Round(time.Millisecond).Seconds()
	stdoutMasker.Flush()
	stderrMasker.Flush()
	output.Stdout, output.IsStdoutTruncated = stdout.getText()
	output.Stderr, output.IsStderrTruncated = stderr.getText()
	var exitError *exec.ExitError
	if waitError != nil && !errors.As(waitError, &exitError) && !errors.Is(waitError, exec.ErrWaitDelay) {
		return output, waitError
	}
	output.ExitCode = command.ProcessState.ExitCode()
	if status, isWaitStatus := command.ProcessState.Sys().(syscall.WaitStatus); isWaitStatus && status.Signaled() {
		output.ExitCode = 128 + int(status.Signal())
	}
	return output, ctx.Err()
}
