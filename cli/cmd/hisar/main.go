package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
)

var version = "development"

type runFunction func(ctx context.Context, flags *flag.FlagSet, allArg []string) error

type appRunFunction func(ctx context.Context, client *appClient, flags *flag.FlagSet, allArg []string) error

// command: runThroughApp is the app-mode variant; isLocal commands need no vault, appAlternative explains
// where to do a standalone-only command when the app is in use, and isHidden keeps it out of the usage.
type command struct {
	name           string
	argumentUsage  string
	summary        string
	run            runFunction
	runThroughApp  appRunFunction
	isLocal        bool
	appAlternative string
	isHidden       bool
}

var allCommand = []command{
	{name: "status", summary: "show whether hisar uses the Hisar app or its own sign-in, and the account", run: runStatus, runThroughApp: runStatusThroughApp},
	{name: "signup", argumentUsage: "<invite-link>", summary: "create your account from an invite", run: runSignup, appAlternative: "create the account in the Hisar app"},
	{name: "login", summary: "sign in on this device", run: runLogin, appAlternative: "sign in to the Hisar app"},
	{name: "logout", summary: "sign out on this device", run: runLogout, appAlternative: "sign out in the Hisar app"},
	{name: "whoami", summary: "show the signed-in account", run: runWhoami, runThroughApp: runWhoamiThroughApp},
	{name: "vaults", summary: "list vaults", run: runListAllVault, runThroughApp: runListAllVaultThroughApp},
	{name: "vault-create", argumentUsage: "<name>", summary: "create a shared vault", run: runCreateVault, runThroughApp: runCreateVaultThroughApp},
	{name: "ls", argumentUsage: "[-v vault]", summary: "list items", run: runListAllItem, runThroughApp: runListAllItemThroughApp},
	{name: "get", argumentUsage: "<item> [-f field] [-v vault]", summary: "show an item, or one field of it (e.g. -f password, -f code)", run: runGet, runThroughApp: runGetThroughApp},
	{name: "add", argumentUsage: "[-v vault]", summary: "add a login", run: runAdd, runThroughApp: runAddThroughApp},
	{name: "edit", argumentUsage: "<item> [-v vault]", summary: "edit an item", run: runEdit, runThroughApp: runEditThroughApp},
	{name: "rm", argumentUsage: "<item> [-v vault]", summary: "delete an item", run: runRemove, runThroughApp: runRemoveThroughApp},
	{name: "import", argumentUsage: "<file> [-v vault] [-format f] [-y]", summary: "import a 1Password, Bitwarden or browser CSV export", run: runImport, runThroughApp: runImportThroughApp},
	{name: "generate", argumentUsage: "[-n length] [-no-symbols]", summary: "generate a password", run: runGenerate, isLocal: true},
	{name: "people", summary: "list the people you share vaults with, with their security codes", run: runListAllPerson, runThroughApp: runListAllPersonThroughApp},
	{name: "share", argumentUsage: "<vault> <email> [-r role]", summary: "invite someone to a vault by email (role: owner, editor, reader)", run: runShare, runThroughApp: runShareThroughApp},
	{name: "unshare", argumentUsage: "<vault> <email>", summary: "remove someone from a vault and rotate its key", run: runUnshare, runThroughApp: runUnshareThroughApp},
	{name: "invitations", argumentUsage: "[accept|decline|cancel <id>]", summary: "list share invitations, answer them, and finish accepted shares", run: runInvitations, runThroughApp: runInvitationsThroughApp},
	{name: "invite", argumentUsage: "<email>", summary: "email someone an invite to create a Hisar account", run: runInvite, runThroughApp: runInviteThroughApp},
	{name: "emergency-kit", argumentUsage: "[-o file]", summary: "save your Emergency Kit PDF (sign-in address, email, Secret Key)", run: runEmergencyKit, appAlternative: "save it in the Hisar app under Settings → Emergency Kit"},
	{name: "devices", summary: "list your signed-in devices", run: runListAllDevice, runThroughApp: runListAllDeviceThroughApp},
	{name: "revoke-device", argumentUsage: "<id>", summary: "sign out a device", run: runRevokeDevice, runThroughApp: runRevokeDeviceThroughApp},
}

var allRemovedRecoveryCommandName = []string{"recover", "recovery-setup", "recovery-enable", "recovery-start", "recovery-list", "recovery-complete"}

var errRecoveryRemoved = errors.New("account recovery was removed; sign in with your Emergency Kit")

func init() {
	for _, name := range allRemovedRecoveryCommandName {
		allCommand = append(allCommand, command{name: name, run: runRemovedRecoveryCommand, isLocal: true, isHidden: true})
	}
}

func runRemovedRecoveryCommand(context.Context, *flag.FlagSet, []string) error {
	return errRecoveryRemoved
}

func printUsage(output io.Writer) {
	fmt.Fprintln(output, "usage: hisar [--standalone] <command> [arguments]\n\ncommands:")
	for _, command := range allCommand {
		if command.isHidden {
			continue
		}
		fmt.Fprintf(output, "  %-18s %-35s %s\n", command.name, command.argumentUsage, command.summary)
	}
	fmt.Fprintln(output, `
hisar uses the Hisar app when it is running and lets the command-line tool use it;
otherwise it signs in on its own. --standalone or HISAR_STANDALONE=1 always signs in on its own.
hisar --version prints the version.`)
}

type globalOption struct {
	isStandalone bool
	isVersion    bool
	isHelp       bool
}

// parseGlobalOption reads the options before the command name.
func parseGlobalOption(allArg []string) (globalOption, []string, error) {
	var option globalOption
	for len(allArg) > 0 {
		switch allArg[0] {
		case "--standalone", "-standalone":
			option.isStandalone = true
		case "--version", "-version":
			option.isVersion = true
		case "--help", "-help", "-h", "help":
			option.isHelp = true
		default:
			if len(allArg[0]) > 0 && allArg[0][0] == '-' {
				return option, nil, fmt.Errorf("unknown option %s", allArg[0])
			}
			return option, allArg, nil
		}
		allArg = allArg[1:]
	}
	return option, allArg, nil
}

func findCommand(name string) (command, bool) {
	for _, candidate := range allCommand {
		if candidate.name == name {
			return candidate, true
		}
	}
	return command{}, false
}

func runCommand(ctx context.Context, selected command, isStandaloneForced bool, flags *flag.FlagSet, allArg []string) error {
	if selected.isLocal {
		return selected.run(ctx, flags, allArg)
	}
	selectedMode = chooseMode(isStandaloneForced)
	if selectedMode.client == nil {
		return selected.run(ctx, flags, allArg)
	}
	defer selectedMode.client.Close()
	if selected.runThroughApp == nil {
		return describeStandaloneOnly(selected.name, selected.appAlternative)
	}
	return selected.runThroughApp(ctx, selectedMode.client, flags, allArg)
}

func main() {
	option, allArg, err := parseGlobalOption(os.Args[1:])
	switch {
	case err != nil:
		fmt.Fprintln(os.Stderr, "hisar:", err)
		printUsage(os.Stderr)
		os.Exit(2)
	case option.isVersion:
		fmt.Println("hisar", version)
		return
	case option.isHelp:
		printUsage(os.Stdout)
		return
	case len(allArg) == 0:
		printUsage(os.Stderr)
		os.Exit(2)
	}
	selected, isFound := findCommand(allArg[0])
	if !isFound {
		printUsage(os.Stderr)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	flags := flag.NewFlagSet(selected.name, flag.ContinueOnError)
	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: hisar %s %s\n", selected.name, selected.argumentUsage)
		flags.PrintDefaults()
	}
	err = runCommand(ctx, selected, option.isStandalone, flags, allArg[1:])
	switch {
	case errors.Is(err, errUsage):
		flags.Usage()
		os.Exit(2)
	case errors.Is(err, flag.ErrHelp):
		os.Exit(2)
	case err != nil:
		fmt.Fprintln(os.Stderr, "hisar:", err)
		os.Exit(1)
	}
}
