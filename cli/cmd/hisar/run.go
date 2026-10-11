package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/tryhisar/hisar/core/secretreference"
)

const (
	exitCodeCannotRun   = 126
	exitCodeNotFound    = 127
	outputDrainDuration = time.Second
)

// exitCodeError ends hisar with the command's exit code and prints nothing.
type exitCodeError int

func (code exitCodeError) Error() string { return fmt.Sprintf("exit code %d", int(code)) }

type multipleValue []string

func (allValue *multipleValue) String() string { return strings.Join(*allValue, ", ") }

func (allValue *multipleValue) Set(value string) error {
	*allValue = append(*allValue, value)
	return nil
}

type runOption struct {
	allEnvironmentFile []string
	isMaskingDisabled  bool
	allCommandArg      []string
}

type runStdio struct {
	stdin  *os.File
	stdout *os.File
	stderr *os.File
}

type referenceResolver func(ctx context.Context, allReference []string, commandLine []string) ([]string, error)

// isForegroundJob tells whether the terminal already sends its SIGINT and SIGQUIT to the command as well.
var isForegroundJob = checkForegroundJob

func parseRunOption(flags *flag.FlagSet, allArg []string) (runOption, error) {
	var option runOption
	allEnvironmentFile := multipleValue{}
	flags.Var(&allEnvironmentFile, "env-file", "read variables from this dotenv file (repeatable; later files win)")
	flags.BoolVar(&option.isMaskingDisabled, "no-masking", false, "connect the command straight to the terminal; its output is not masked")
	if err := flags.Parse(allArg); err != nil {
		return option, err
	}
	option.allEnvironmentFile = allEnvironmentFile
	option.allCommandArg = flags.Args()
	if len(option.allCommandArg) == 0 {
		return option, errUsage
	}
	return option, nil
}

func runRun(ctx context.Context, flags *flag.FlagSet, allArg []string) error {
	option, err := parseRunOption(flags, allArg)
	if err != nil {
		return err
	}
	exitCode, err := runWithSecret(ctx, option, os.Environ(), resolveAllReference, runStdio{stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr})
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return exitCodeError(exitCode)
	}
	return nil
}

// buildRunEnvironment applies the env files in order; a real variable wins over the files unless its value is a reference.
func buildRunEnvironment(allEnviron []string, allFileVariable []environmentVariable) []environmentVariable {
	var allVariable []environmentVariable
	indexByName := map[string]int{}
	for _, entry := range allEnviron {
		name, value, hasEquals := strings.Cut(entry, "=")
		if !hasEquals || name == "" {
			continue
		}
		if index, isKnown := indexByName[name]; isKnown {
			allVariable[index].value = value
			continue
		}
		indexByName[name] = len(allVariable)
		allVariable = append(allVariable, environmentVariable{name: name, value: value})
	}
	isFromFile := map[string]bool{}
	for _, fileVariable := range allFileVariable {
		index, isKnown := indexByName[fileVariable.name]
		if !isKnown {
			indexByName[fileVariable.name] = len(allVariable)
			allVariable = append(allVariable, fileVariable)
			isFromFile[fileVariable.name] = true
			continue
		}
		if isFromFile[fileVariable.name] || secretreference.IsReference(allVariable[index].value) {
			allVariable[index].value = fileVariable.value
			isFromFile[fileVariable.name] = true
		}
	}
	return allVariable
}

func listAllUniqueReference(allVariable []environmentVariable) []string {
	var allReference []string
	isListed := map[string]bool{}
	for _, variable := range allVariable {
		if secretreference.IsReference(variable.value) && !isListed[variable.value] {
			isListed[variable.value] = true
			allReference = append(allReference, variable.value)
		}
	}
	return allReference
}

func findCommandPath(name string) (string, int, error) {
	path, err := exec.LookPath(name)
	switch {
	case err == nil:
		return path, 0, nil
	case errors.Is(err, fs.ErrPermission):
		return "", exitCodeCannotRun, fmt.Errorf("%s: permission denied", name)
	}
	return "", exitCodeNotFound, fmt.Errorf("%s: command not found", name)
}

// runWithSecret resolves the references, starts the command with the values in its environment and returns its exit code.
func runWithSecret(ctx context.Context, option runOption, allEnviron []string, resolve referenceResolver, stdio runStdio) (int, error) {
	var allFileVariable []environmentVariable
	for _, path := range option.allEnvironmentFile {
		allVariableInFile, err := readEnvironmentFile(path)
		if err != nil {
			return 0, err
		}
		allFileVariable = append(allFileVariable, allVariableInFile...)
	}
	allVariable := buildRunEnvironment(allEnviron, allFileVariable)
	commandPath, exitCode, err := findCommandPath(option.allCommandArg[0])
	if err != nil {
		fmt.Fprintln(stdio.stderr, "hisar:", err)
		return exitCode, nil
	}

	allReference := listAllUniqueReference(allVariable)
	var allSecretValue []string
	if len(allReference) == 0 {
		fmt.Fprintln(stdio.stderr, "hisar: no secret references found")
	} else {
		allValue, err := resolve(ctx, allReference, option.allCommandArg)
		if err != nil {
			return 0, err
		}
		valueByReference := map[string]string{}
		for index, reference := range allReference {
			valueByReference[reference] = allValue[index]
		}
		for index, variable := range allVariable {
			value, isReference := valueByReference[variable.value]
			if !isReference {
				continue
			}
			allVariable[index].value = value
			allSecretValue = append(allSecretValue, value)
			if !option.isMaskingDisabled && len(value) < minimumMaskedLength {
				fmt.Fprintf(stdio.stderr, "hisar: %s is shorter than %d bytes and is not masked\n", variable.name, minimumMaskedLength)
			}
		}
	}

	allEnvironmentEntry := make([]string, 0, len(allVariable))
	for _, variable := range allVariable {
		allEnvironmentEntry = append(allEnvironmentEntry, variable.name+"="+variable.value)
	}
	command := exec.Command(commandPath, option.allCommandArg[1:]...)
	command.Args[0] = option.allCommandArg[0]
	command.Env = allEnvironmentEntry
	command.Stdin = stdio.stdin
	if option.isMaskingDisabled {
		command.Stdout, command.Stderr = stdio.stdout, stdio.stderr
		return startAndWait(command, stdio.stderr, nil)
	}
	return runMasked(command, stdio, allSecretValue)
}

func runMasked(command *exec.Cmd, stdio runStdio, allSecretValue []string) (int, error) {
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		return 0, err
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		stdoutReader.Close()
		stdoutWriter.Close()
		return 0, err
	}
	defer stdoutReader.Close()
	defer stderrReader.Close()
	command.Stdout, command.Stderr = stdoutWriter, stderrWriter
	stop := make(chan struct{})
	var waitGroup sync.WaitGroup
	for _, stream := range []struct {
		reader *os.File
		output *os.File
	}{{stdoutReader, stdio.stdout}, {stderrReader, stdio.stderr}} {
		waitGroup.Go(func() {
			copyMasked(createSecretMasker(stream.output, allSecretValue), stream.reader, maskingFlushDelay, stop)
		})
	}
	afterStart := func() {
		stdoutWriter.Close()
		stderrWriter.Close()
	}
	exitCode, err := startAndWait(command, stdio.stderr, afterStart)
	afterStart()
	copyDone := make(chan struct{})
	go func() {
		waitGroup.Wait()
		close(copyDone)
	}()
	select {
	case <-copyDone:
	case <-time.After(outputDrainDuration):
		close(stop)
		<-copyDone
	}
	return exitCode, err
}

// startAndWait forwards signals to the command while it runs; a command a signal ended exits 128 + its number.
func startAndWait(command *exec.Cmd, stderr *os.File, afterStart func()) (int, error) {
	signalChannel := make(chan os.Signal, 8)
	signal.Notify(signalChannel, allForwardedSignal...)
	defer signal.Stop(signalChannel)
	if err := command.Start(); err != nil {
		fmt.Fprintf(stderr, "hisar: %s: %v\n", command.Args[0], err)
		return exitCodeCannotRun, nil
	}
	if afterStart != nil {
		afterStart()
	}
	done := make(chan struct{})
	forwardingDone := make(chan struct{})
	defer func() {
		close(done)
		<-forwardingDone
	}()
	go func() {
		defer close(forwardingDone)
		for {
			select {
			case receivedSignal := <-signalChannel:
				if !isTerminalSignal(receivedSignal) || !isForegroundJob() {
					command.Process.Signal(receivedSignal)
				}
			case <-done:
				return
			}
		}
	}()
	err := command.Wait()
	var exitError *exec.ExitError
	if err != nil && !errors.As(err, &exitError) {
		return 0, err
	}
	if status, isWaitStatus := command.ProcessState.Sys().(syscall.WaitStatus); isWaitStatus && status.Signaled() {
		return 128 + int(status.Signal()), nil
	}
	return command.ProcessState.ExitCode(), nil
}
