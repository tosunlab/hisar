package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/localstate"
)

func writeAppStatus(output io.Writer, mode commandMode) {
	hello := mode.client.hello
	fmt.Fprintf(output, "mode:    Hisar app (%s)\n", mode.socketPath)
	switch hello.State {
	case cliprotocol.StateSignedOut:
		fmt.Fprintln(output, "account: the Hisar app is not signed in")
		return
	case cliprotocol.StateLocked:
		fmt.Fprintf(output, "account: %s on %s\nstate:   locked; the next command asks to unlock it\n", hello.Email, hello.Server)
	default:
		fmt.Fprintf(output, "account: %s on %s\nstate:   unlocked\n", hello.Email, hello.Server)
	}
	if hello.IsApproved {
		fmt.Fprintln(output, "access:  approved for this terminal")
	} else {
		fmt.Fprintln(output, "access:  the next command asks for approval in Hisar")
	}
}

func writeStandaloneStatus(output io.Writer, mode commandMode, currentState *localstate.State, statePath string) {
	fmt.Fprintf(output, "mode:    standalone (%s)\n", mode.reason)
	if !currentState.IsLoggedIn() {
		fmt.Fprintln(output, "account: not signed in; run `hisar login` or `hisar signup <invite-link>`")
	} else {
		fmt.Fprintf(output, "account: %s on %s\n", currentState.Email, currentState.Server)
	}
	fmt.Fprintf(output, "state:   %s\n", statePath)
}

func runStatusThroughApp(_ context.Context, client *appClient, flags *flag.FlagSet, args []string) error {
	if _, err := parse(flags, args); err != nil {
		return err
	}
	writeAppStatus(os.Stdout, commandMode{client: client, socketPath: client.socketPath})
	return nil
}

func runStatus(_ context.Context, flags *flag.FlagSet, args []string) error {
	if _, err := parse(flags, args); err != nil {
		return err
	}
	statePath, err := localstate.GetPath("HISAR_STATE", "state.json")
	if err != nil {
		return err
	}
	currentState, err := localstate.Load(statePath)
	if err != nil {
		return err
	}
	writeStandaloneStatus(os.Stdout, selectedMode, currentState, statePath)
	return nil
}
