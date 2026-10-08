package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

var stdin = bufio.NewReader(os.Stdin)

func readLine() (string, error) {
	line, err := stdin.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func prompt(label, def string) (string, error) {
	if def != "" {
		fmt.Fprintf(os.Stderr, "%s [%s]: ", label, def)
	} else {
		fmt.Fprintf(os.Stderr, "%s: ", label)
	}
	v, err := readLine()
	if err != nil {
		return "", err
	}
	if v = strings.TrimSpace(v); v == "" {
		return def, nil
	}
	return v, nil
}

func promptRequired(label, def string) (string, error) {
	for {
		v, err := prompt(label, def)
		if err != nil || v != "" {
			return v, err
		}
		fmt.Fprintf(os.Stderr, "%s is required\n", label)
	}
}

// promptSecret reads without echo on a terminal, or a plain line when stdin is piped.
func promptSecret(label string) ([]byte, error) {
	fmt.Fprintf(os.Stderr, "%s: ", label)
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		return b, err
	}
	line, err := readLine()
	fmt.Fprintln(os.Stderr)
	return []byte(line), err
}

func promptNewPassword() ([]byte, error) {
	for {
		pw, err := promptSecret("Master password")
		if err != nil {
			return nil, err
		}
		if len(pw) < 10 {
			fmt.Fprintln(os.Stderr, "Use at least 10 characters.")
			continue
		}
		again, err := promptSecret("Repeat master password")
		if err != nil {
			return nil, err
		}
		if string(pw) == string(again) {
			clear(again)
			return pw, nil
		}
		fmt.Fprintln(os.Stderr, "Passwords do not match.")
	}
}

func confirm(label string) (bool, error) {
	v, err := prompt(label+" (y/N)", "")
	return strings.EqualFold(v, "y") || strings.EqualFold(v, "yes"), err
}

// parse allows flags before and after positional arguments.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

var errUsage = errors.New("usage")
