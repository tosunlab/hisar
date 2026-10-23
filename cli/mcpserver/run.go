package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/tryhisar/hisar/cli/internal/dotenv"
	"github.com/tryhisar/hisar/cli/internal/masking"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/secretreference"
)

const (
	DefaultTimeout        = 120 * time.Second
	MaximumTimeout        = time.Hour
	MaximumOutputLength   = 64 << 10
	defaultKillDelay      = 5 * time.Second
	outputDrainDuration   = time.Second
	maximumSecretCount    = cliprotocol.MaxAgentReferenceCount
	maximumArgumentLength = 1 << 20
	shellPath             = "/bin/sh"
	direnvStatusTimeout   = 5 * time.Second
	// direnvAllowed is direnv's AllowStatus for an .envrc the user allowed.
	direnvAllowed = 0
)

var variableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type RunInput struct {
	Command          []string          `json:"command,omitempty" jsonschema:"the program and its arguments, e.g. [\"npm\", \"test\"]; it runs without a shell. Pass command or shell, not both"`
	Shell            string            `json:"shell,omitempty" jsonschema:"a command line that /bin/sh -c runs, for pipes, redirects and &&, e.g. \"npm test 2>&1 | tail -50\"; pass command or shell, not both"`
	Env              map[string]string `json:"env,omitempty" jsonschema:"environment variables to set, each mapped to a hisar://vault/item/field secret reference from search_items"`
	EnvFiles         []string          `json:"envFiles,omitempty" jsonschema:"dotenv files to read in order, relative to the working directory, as hisar run --env-file does: a value that is a whole hisar:// reference is resolved, other values are set as they are, and a variable already in the environment wins unless its value is a reference"`
	Stdin            string            `json:"stdin,omitempty" jsonschema:"the command's standard input: literal text, or one whole hisar:// reference whose value is sent instead, e.g. for docker login --password-stdin; empty without it"`
	WorkingDirectory string            `json:"workingDirectory,omitempty" jsonschema:"the directory to run in; relative to the directory hisar mcp started in (usually the project), which is also the default"`
	TimeoutSeconds   int               `json:"timeoutSeconds,omitempty" jsonschema:"stop the command after this many seconds; default 120, at most 3600"`
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

// preparedCommand is a checked run_with_secrets call whose references are not resolved yet.
type preparedCommand struct {
	path string
	// arguments is what runs and what the approval shows; arguments[0] is the program's name.
	arguments        []string
	workingDirectory string
	// variables is the whole environment; a value that is a reference becomes its secret.
	variables []dotenv.Variable
	// fileEnvironment is each NAME=value the env files set that is not a reference, which the approval shows.
	fileEnvironment []string
	// stdin is literal text or one reference.
	stdin      string
	references []string
	timeout    time.Duration
}

// prepareCommand checks everything that can fail before the user is asked: the command, names, env files,
// references, directory and program.
func prepareCommand(input RunInput, defaultDirectory string, environ []string) (preparedCommand, error) {
	prepared := preparedCommand{stdin: input.Stdin, timeout: DefaultTimeout}
	switch {
	case input.Shell != "" && len(input.Command) > 0:
		return prepared, errors.New("pass command or shell, not both")
	case input.Shell != "":
		prepared.arguments = []string{shellPath, "-c", input.Shell}
	case len(input.Command) == 0 || input.Command[0] == "":
		return prepared, errors.New("command is empty; pass the program and its arguments, e.g. [\"npm\", \"test\"], or a command line as shell")
	default:
		prepared.arguments = input.Command
	}
	totalLength := len(input.Stdin)
	for _, argument := range prepared.arguments {
		totalLength += len(argument)
		if strings.ContainsRune(argument, 0) {
			return prepared, errors.New("an argument contains a NUL byte")
		}
	}
	if totalLength > maximumArgumentLength {
		return prepared, errors.New("the command's arguments and stdin are longer than 1 MiB")
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
	variables, err := buildVariables(input, prepared.workingDirectory, environ)
	if err != nil {
		return prepared, err
	}
	prepared.variables = variables
	prepared.fileEnvironment = listFileEnvironment(variables, environ)
	prepared.references = dotenv.ListReferences(variables)
	if secretreference.IsReference(input.Stdin) && !slices.Contains(prepared.references, input.Stdin) {
		prepared.references = append(prepared.references, input.Stdin)
	}
	if len(prepared.references) == 0 {
		return prepared, errors.New("no secret to use: map a variable to a hisar:// reference in env, list envFiles with hisar:// values, " +
			"or pass a reference as stdin (run commands without secrets with your own tools)")
	}
	if len(prepared.references) > maximumSecretCount {
		return prepared, fmt.Errorf("at most %d secrets can come from Hisar at once", maximumSecretCount)
	}
	if direnvPath := findDirenv(prepared.workingDirectory, environ); direnvPath != "" {
		prepared.path = direnvPath
		prepared.arguments = append([]string{"direnv", "exec", prepared.workingDirectory}, prepared.arguments...)
		return prepared, nil
	}
	program := prepared.arguments[0]
	if strings.ContainsRune(program, filepath.Separator) && !filepath.IsAbs(program) {
		program = filepath.Join(prepared.workingDirectory, program)
	}
	path, err := exec.LookPath(program)
	switch {
	case errors.Is(err, fs.ErrPermission):
		return prepared, fmt.Errorf("%s: permission denied", prepared.arguments[0])
	case err != nil && !errors.Is(err, exec.ErrDot):
		return prepared, fmt.Errorf("%s: command not found", prepared.arguments[0])
	}
	prepared.path = path
	return prepared, nil
}

// buildVariables layers the env files over environ as hisar run does, then sets the env mapping over both, and
// checks every reference among them and stdin.
func buildVariables(input RunInput, workingDirectory string, environ []string) ([]dotenv.Variable, error) {
	paths := make([]string, len(input.EnvFiles))
	for index, path := range input.EnvFiles {
		if !filepath.IsAbs(path) {
			path = filepath.Join(workingDirectory, path)
		}
		paths[index] = path
	}
	fileVariables, err := dotenv.ReadFiles(paths)
	if err != nil {
		return nil, err
	}
	variables := dotenv.BuildEnvironment(environ, fileVariables)
	names := make([]string, 0, len(input.Env))
	for name := range input.Env {
		names = append(names, name)
	}
	slices.Sort(names)
	var failures []string
	for _, name := range names {
		reference := input.Env[name]
		switch {
		case !variableNamePattern.MatchString(name):
			failures = append(failures, fmt.Sprintf("%q is not a valid variable name", name))
			continue
		case !secretreference.IsReference(reference):
			failures = append(failures, fmt.Sprintf("%s: the value must be a hisar://vault/item/field reference", name))
			continue
		}
		index := slices.IndexFunc(variables, func(variable dotenv.Variable) bool { return variable.Name == name })
		if index < 0 {
			variables = append(variables, dotenv.Variable{Name: name, Value: reference})
		} else {
			variables[index].Value = reference
		}
	}
	for _, variable := range append(variables, dotenv.Variable{Name: "stdin", Value: input.Stdin}) {
		if !secretreference.IsReference(variable.Value) {
			continue
		}
		if _, err := secretreference.Parse(variable.Value); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %s: %v", variable.Name, variable.Value, err))
		}
	}
	if len(failures) > 0 {
		return nil, errors.New(strings.Join(failures, "; "))
	}
	return variables, nil
}

// listFileEnvironment returns NAME=value for each variable the env files set to a literal value: every one that is
// not a reference and not as environ has it, since the env mapping sets references only.
func listFileEnvironment(variables []dotenv.Variable, environ []string) []string {
	inherited := map[string]string{}
	for _, variable := range dotenv.BuildEnvironment(environ, nil) {
		inherited[variable.Name] = variable.Value
	}
	var fileEnvironment []string
	for _, variable := range variables {
		value, isInherited := inherited[variable.Name]
		if !secretreference.IsReference(variable.Value) && (!isInherited || value != variable.Value) {
			fileEnvironment = append(fileEnvironment, variable.Name+"="+variable.Value)
		}
	}
	return fileEnvironment
}

// findDirenv returns direnv's path when it is installed and the user allowed the .envrc that applies to directory,
// so the command gets the same environment as in the user's shell; otherwise "".
func findDirenv(directory string, environ []string) string {
	path, err := exec.LookPath("direnv")
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), direnvStatusTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, path, "status", "--json")
	command.Dir, command.Env = directory, environ
	output, err := command.Output()
	if err != nil {
		return ""
	}
	var status struct {
		State struct {
			FoundRC *struct {
				Allowed *int `json:"allowed"`
			} `json:"foundRC"`
		} `json:"state"`
	}
	if json.Unmarshal(output, &status) != nil || status.State.FoundRC == nil || status.State.FoundRC.Allowed == nil || *status.State.FoundRC.Allowed != direnvAllowed {
		return ""
	}
	return path
}

// run starts the command in its own process group, masks its output, reports it as progress, and stops the
// group at the timeout.
func (server *toolServer) run(ctx context.Context, prepared preparedCommand, environment []string, stdin string, secretValues []string, progress *progressReporter) (RunOutput, error) {
	var stdout, stderr outputCapture
	stdoutWriter, stderrWriter := io.Writer(&stdout), io.Writer(&stderr)
	finishProgress := func() {}
	if progress != nil {
		stream := &progressStream{reporter: progress}
		stdoutWriter, stderrWriter = io.MultiWriter(&stdout, stream), io.MultiWriter(&stderr, stream)
		finishProgress = stream.start(ctx)
	}
	stdoutMasker := masking.NewSecretMasker(stdoutWriter, secretValues)
	stderrMasker := masking.NewSecretMasker(stderrWriter, secretValues)
	command := exec.Command(prepared.path, prepared.arguments[1:]...)
	command.Args[0] = prepared.arguments[0]
	command.Dir = prepared.workingDirectory
	command.Env = environment
	if stdin != "" {
		command.Stdin = strings.NewReader(stdin)
	}
	command.Stdout, command.Stderr = stdoutMasker, stderrMasker
	command.WaitDelay = outputDrainDuration
	startedAt := time.Now()
	group, err := startProcessGroup(command)
	if err != nil {
		finishProgress()
		return RunOutput{}, fmt.Errorf("%s: %v", prepared.arguments[0], err)
	}
	defer group.close()
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
		waitError = group.stop(waitResult, server.config.KillDelay)
	case <-ctx.Done():
		waitError = group.stop(waitResult, server.config.KillDelay)
	}
	output.DurationSeconds = time.Since(startedAt).Round(time.Millisecond).Seconds()
	stdoutMasker.Flush()
	stderrMasker.Flush()
	finishProgress()
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
