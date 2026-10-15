package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

var stdinReader = bufio.NewReader(os.Stdin)

func readLine() (string, error) {
	line, err := stdinReader.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func prompt(label, defaultValue string) (string, error) {
	if defaultValue != "" {
		fmt.Fprintf(os.Stderr, "%s [%s]: ", label, defaultValue)
	} else {
		fmt.Fprintf(os.Stderr, "%s: ", label)
	}
	answer, err := readLine()
	if err != nil {
		return "", err
	}
	if answer = strings.TrimSpace(answer); answer == "" {
		return defaultValue, nil
	}
	return answer, nil
}

// promptLines reads lines until one that is only "." or the end of the input, so blank lines stay in the text. Blank
// lines at the start and end are dropped.
func promptLines(label string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s:\n", label)
	var lines []string
	for {
		line, err := readLine()
		if errors.Is(err, io.EOF) || (err == nil && line == ".") {
			return strings.Trim(strings.Join(lines, "\n"), "\n"), nil
		}
		if err != nil {
			return "", err
		}
		lines = append(lines, line)
	}
}

func promptRequired(label, defaultValue string) (string, error) {
	for {
		answer, err := prompt(label, defaultValue)
		if err != nil || answer != "" {
			return answer, err
		}
		fmt.Fprintf(os.Stderr, "%s is required\n", label)
	}
}

// promptSecret reads without echo on a terminal, or a plain line when stdin is piped.
func promptSecret(label string) ([]byte, error) {
	fmt.Fprintf(os.Stderr, "%s: ", label)
	stdinFD := int(os.Stdin.Fd())
	if term.IsTerminal(stdinFD) {
		secret, err := term.ReadPassword(stdinFD)
		fmt.Fprintln(os.Stderr)
		return secret, err
	}
	line, err := readLine()
	fmt.Fprintln(os.Stderr)
	return []byte(line), err
}

func promptNewPassword() ([]byte, error) {
	for {
		password, err := promptSecret("Master password")
		if err != nil {
			return nil, err
		}
		if len(password) < 10 {
			fmt.Fprintln(os.Stderr, "Use at least 10 characters.")
			continue
		}
		repeatedPassword, err := promptSecret("Repeat master password")
		if err != nil {
			return nil, err
		}
		if string(password) == string(repeatedPassword) {
			clear(repeatedPassword)
			return password, nil
		}
		fmt.Fprintln(os.Stderr, "Passwords do not match.")
	}
}

func confirm(label string) (bool, error) {
	answer, err := prompt(label+" (y/N)", "")
	return strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes"), err
}

// parse allows flags before and after positional arguments.
func parse(flags *flag.FlagSet, args []string) ([]string, error) {
	var positionals []string
	for {
		if err := flags.Parse(args); err != nil {
			return nil, err
		}
		args = flags.Args()
		if len(args) == 0 {
			return positionals, nil
		}
		positionals = append(positionals, args[0])
		args = args[1:]
	}
}

var errUsage = errors.New("usage")

// repeatedFlag collects every value of a flag given more than once.
type repeatedFlag []string

func (values *repeatedFlag) String() string { return strings.Join(*values, ",") }

func (values *repeatedFlag) Set(value string) error {
	*values = append(*values, value)
	return nil
}
