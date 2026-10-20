package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/localstate"
)

// stateNotRunning is the state of a hisar that could not reach the Hisar app and fell back to standalone.
const stateNotRunning = "not-running"

// statusReport is what `hisar status --json` prints. The omp plugin reads it, so its JSON tags never change.
type statusReport struct {
	Mode    string `json:"mode"`
	State   string `json:"state"`
	Account string `json:"account"`
}

func describeAppStatus(hello cliprotocol.HelloResult) statusReport {
	switch hello.State {
	case cliprotocol.StateSignedOut:
		return statusReport{Mode: "app", State: cliprotocol.StateSignedOut}
	case cliprotocol.StateLocked:
		return statusReport{Mode: "app", State: cliprotocol.StateLocked, Account: hello.Email}
	default:
		return statusReport{Mode: "app", State: cliprotocol.StateUnlocked, Account: hello.Email}
	}
}

// describeStandaloneStatus calls a fallback to standalone not-running; a chosen standalone mode asks for the master
// password on every command, so it is locked once signed in.
func describeStandaloneStatus(currentState *localstate.State, isForced bool) statusReport {
	report := statusReport{Mode: "standalone"}
	if currentState.IsLoggedIn() {
		report.Account = currentState.Email
	}
	switch {
	case !isForced:
		report.State = stateNotRunning
	case currentState.IsLoggedIn():
		report.State = cliprotocol.StateLocked
	default:
		report.State = cliprotocol.StateSignedOut
	}
	return report
}

func writeStatusJSON(output io.Writer, report statusReport) error {
	return json.NewEncoder(output).Encode(report)
}

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
	isJSON := flags.Bool("json", false, "print mode, state and account as JSON")
	if _, err := parse(flags, args); err != nil {
		return err
	}
	if *isJSON {
		return writeStatusJSON(os.Stdout, describeAppStatus(client.hello))
	}
	writeAppStatus(os.Stdout, commandMode{client: client, socketPath: client.socketPath})
	return nil
}

func runStatus(_ context.Context, flags *flag.FlagSet, args []string) error {
	isJSON := flags.Bool("json", false, "print mode, state and account as JSON")
	if _, err := parse(flags, args); err != nil {
		return err
	}
	if refusal := checkStandaloneAllowed(); refusal != nil {
		writeRefusedStatus(os.Stdout, selectedMode, refusal)
		return nil
	}
	statePath, err := localstate.GetPath("HISAR_STATE", "state.json")
	if err != nil {
		return err
	}
	currentState, err := localstate.Load(statePath)
	if err != nil {
		return err
	}
	if *isJSON {
		return writeStatusJSON(os.Stdout, describeStandaloneStatus(currentState, isStandaloneForcedByOption || isStandaloneRequested()))
	}
	writeStandaloneStatus(os.Stdout, selectedMode, currentState, statePath)
	return nil
}
