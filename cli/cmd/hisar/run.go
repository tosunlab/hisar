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

	"github.com/tryhisar/hisar/cli/internal/dotenv"
	"github.com/tryhisar/hisar/cli/internal/masking"
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

type runOption struct {
	environmentFiles  []string
	isMaskingDisabled bool
	commandArgs       []string
}

type runStdio struct {
	stdin  *os.File
	stdout *os.File
	stderr *os.File
}

type referenceResolver func(ctx context.Context, references []string, commandLine []string) ([]string, error)

// isForegroundJob tells whether the terminal already sends its SIGINT and SIGQUIT to the command as well.
var isForegroundJob = checkForegroundJob

func parseRunOption(flags *flag.FlagSet, args []string) (runOption, error) {
	var option runOption
	environmentFiles := repeatedFlag{}
	flags.Var(&environmentFiles, "env-file", "read variables from this dotenv file (repeatable; later files win)")
	flags.BoolVar(&option.isMaskingDisabled, "no-masking", false, "connect the command straight to the terminal; its output is not masked")
	if err := flags.Parse(args); err != nil {
		return option, err
	}
	option.environmentFiles = environmentFiles
	option.commandArgs = flags.Args()
	if len(option.commandArgs) == 0 {
		return option, errUsage
	}
	return option, nil
}

func runRun(ctx context.Context, flags *flag.FlagSet, args []string) error {
	option, err := parseRunOption(flags, args)
	if err != nil {
		return err
	}
	exitCode, err := runWithSecret(ctx, option, os.Environ(), resolveReferences, runStdio{stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr})
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return exitCodeError(exitCode)
	}
	return nil
}

// buildRunEnvironment applies the env files in order; a real variable wins over the files unless its value is a reference.
func buildRunEnvironment(environ []string, fileVariables []dotenv.Variable) []dotenv.Variable {
	var variables []dotenv.Variable
	indexByName := map[string]int{}
	for _, entry := range environ {
		name, value, hasEquals := strings.Cut(entry, "=")
		if !hasEquals || name == "" {
			continue
		}
		if index, isKnown := indexByName[name]; isKnown {
			variables[index].Value = value
			continue
		}
		indexByName[name] = len(variables)
		variables = append(variables, dotenv.Variable{Name: name, Value: value})
	}
	isFromFile := map[string]bool{}
	for _, fileVariable := range fileVariables {
		index, isKnown := indexByName[fileVariable.Name]
		if !isKnown {
			indexByName[fileVariable.Name] = len(variables)
			variables = append(variables, fileVariable)
			isFromFile[fileVariable.Name] = true
			continue
		}
		if isFromFile[fileVariable.Name] || secretreference.IsReference(variables[index].Value) {
			variables[index].Value = fileVariable.Value
			isFromFile[fileVariable.Name] = true
		}
	}
	return variables
}

func listUniqueReferences(variables []dotenv.Variable) []string {
	var references []string
	isListed := map[string]bool{}
	for _, variable := range variables {
		if secretreference.IsReference(variable.Value) && !isListed[variable.Value] {
			isListed[variable.Value] = true
			references = append(references, variable.Value)
		}
	}
	return references
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
func runWithSecret(ctx context.Context, option runOption, environ []string, resolve referenceResolver, stdio runStdio) (int, error) {
	var fileVariables []dotenv.Variable
	for _, path := range option.environmentFiles {
		variablesInFile, err := dotenv.ReadFile(path)
		if err != nil {
			return 0, err
		}
		fileVariables = append(fileVariables, variablesInFile...)
	}
	variables := buildRunEnvironment(environ, fileVariables)
	commandPath, exitCode, err := findCommandPath(option.commandArgs[0])
	if err != nil {
		fmt.Fprintln(stdio.stderr, "hisar:", err)
		return exitCode, nil
	}

	references := listUniqueReferences(variables)
	var secretValues []string
	if len(references) == 0 {
		fmt.Fprintln(stdio.stderr, "hisar: no secret references found")
	} else {
		values, err := resolve(ctx, references, option.commandArgs)
		if err != nil {
			return 0, err
		}
		valueByReference := map[string]string{}
		for index, reference := range references {
			valueByReference[reference] = values[index]
		}
		for index, variable := range variables {
			value, isReference := valueByReference[variable.Value]
			if !isReference {
				continue
			}
			variables[index].Value = value
			secretValues = append(secretValues, value)
			if !option.isMaskingDisabled && len(value) < masking.MinimumLength {
				fmt.Fprintf(stdio.stderr, "hisar: %s is shorter than %d bytes and is not masked\n", variable.Name, masking.MinimumLength)
			}
		}
	}

	environmentEntries := make([]string, 0, len(variables))
	for _, variable := range variables {
		environmentEntries = append(environmentEntries, variable.Name+"="+variable.Value)
	}
	command := exec.Command(commandPath, option.commandArgs[1:]...)
	command.Args[0] = option.commandArgs[0]
	command.Env = environmentEntries
	command.Stdin = stdio.stdin
	if option.isMaskingDisabled {
		command.Stdout, command.Stderr = stdio.stdout, stdio.stderr
		return startAndWait(command, stdio.stderr, nil)
	}
	return runMasked(command, stdio, secretValues)
}

func runMasked(command *exec.Cmd, stdio runStdio, secretValues []string) (int, error) {
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
			masking.CopyMasked(masking.NewSecretMasker(stream.output, secretValues), stream.reader, masking.FlushDelay, stop)
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
	signal.Notify(signalChannel, forwardedSignals...)
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
