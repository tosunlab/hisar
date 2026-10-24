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

type runFunction func(ctx context.Context, flags *flag.FlagSet, args []string) error

type appRunFunction func(ctx context.Context, client *appClient, flags *flag.FlagSet, args []string) error

// backendRunFunction is a command written once for both modes; runCommand passes the app or the standalone backend.
type backendRunFunction func(ctx context.Context, access backend, flags *flag.FlagSet, args []string) error

// command: withBackend works through the app or standalone; run is standalone-only and runThroughApp its app-mode
// variant; isLocal commands choose the mode themselves, if they need a vault; appAlternative explains where to do a
// standalone-only command when the app is in use, and isHidden keeps it out of the usage.
type command struct {
	name           string
	argumentUsage  string
	summary        string
	withBackend    backendRunFunction
	run            runFunction
	runThroughApp  appRunFunction
	isLocal        bool
	appAlternative string
	isHidden       bool
}

var commands = []command{
	{name: "status", argumentUsage: "[--json]", summary: "show whether hisar uses the Hisar app or its own sign-in, and the account; --json prints mode, state and account", run: runStatus, runThroughApp: runStatusThroughApp},
	{name: "signup", argumentUsage: "[-o file] [-y] <invite-link>", summary: "create your account from an invite with durable recovery", run: runSignup, appAlternative: "create the account in the Hisar app"},
	{name: "login", summary: "sign in on this device", run: runLogin, appAlternative: "sign in to the Hisar app"},
	{name: "logout", summary: "sign out on this device", run: runLogout, appAlternative: "sign out in the Hisar app"},
	{name: "whoami", summary: "show the signed-in account", withBackend: runWhoami},
	{name: "vaults", summary: "list vaults", withBackend: runListVaults},
	{name: "vault-create", argumentUsage: "<name>", summary: "create a shared vault", withBackend: runCreateVault},
	{name: "vault", argumentUsage: "rename|delete|leave|create|give|take <vault>", summary: "rename, delete or leave a vault, create one, or give an organization vault to a team (" + vaultUsage + ")", withBackend: runVault},
	{name: "ls", argumentUsage: "[-v vault] [--tag tag] [--favorites]", summary: "list items; --tag keeps those with a tag, --favorites lists your favorites in every vault", withBackend: runListItems},
	{name: "get", argumentUsage: "<item> [-f field] [-v vault]", summary: "show an item, or one field of it (e.g. -f password, -f code)", withBackend: runGet},
	{name: "read", argumentUsage: "<hisar://vault/item/field> [--no-newline]", summary: "print one secret, for scripts", run: runRead, isLocal: true},
	{name: "run", argumentUsage: "[--env-file f]... [--no-masking] -- <cmd>", summary: "run a command with the secrets its hisar:// references name, masked in its output", run: runRun, isLocal: true},
	{name: "mcp", argumentUsage: "[--no-reveal]", summary: "serve coding agents over MCP (stdio): they use secrets without seeing them, after you approve in the Hisar app", run: runMCP, isLocal: true},
	{name: "add", argumentUsage: "[-v vault] [--type t]", summary: "add an item; --type is one of " + listItemTypeNames() + " (default login)", withBackend: runAdd},
	{name: "edit", argumentUsage: "<item> [-v vault]", summary: "edit an item", withBackend: runEdit},
	{name: "rm", argumentUsage: "<item> [-v vault]", summary: "delete an item", withBackend: runRemove},
	{name: "favorite", argumentUsage: "<item> [-v vault]", summary: "add an item to your Favorites, on all your devices (only you see them)", withBackend: runFavorite},
	{name: "unfavorite", argumentUsage: "<item> [-v vault]", summary: "remove an item from your Favorites", withBackend: runUnfavorite},
	{name: "import", argumentUsage: "<file> [-v vault] [-format f] [-y]", summary: "import a Hisar export, a 1Password, Bitwarden, LastPass or browser CSV export, or a .env file (--item, --keys, --dry-run)", withBackend: runImport},
	{name: "export", argumentUsage: "[-o file] [--format encrypted|plain]", summary: "export every vault you can open, encrypted with a passphrase you choose or as plain JSON", withBackend: runExport},
	{name: "generate", argumentUsage: "[-n length] [-no-symbols]", summary: "generate a password", run: runGenerate, isLocal: true},
	{name: "people", summary: "list the people you share vaults with, with their security codes", withBackend: runListPeople},
	{name: "share", argumentUsage: "<vault> <email> [-r role]", summary: "invite someone to a vault by email (role: owner, editor, reader)", withBackend: runShare},
	{name: "unshare", argumentUsage: "<vault> <email>", summary: "remove someone from a vault and rotate its key", withBackend: runUnshare},
	{name: "invitations", argumentUsage: "[accept|decline|cancel|confirm <id>]", summary: "list share invitations, answer them, and review accepted recipients", withBackend: runInvitations},
	{name: "invite", argumentUsage: "<email>", summary: "email someone an invite to create a Hisar account", withBackend: runInvite},
	{name: "orgs", summary: "list your organizations and invitations to them; gives team members the vault keys they wait for and rotates the vaults someone lost", withBackend: runListOrganizations},
	{name: "org", argumentUsage: "<action> <org> [arguments]", summary: "manage an organization (" + organizationUsage + ")", withBackend: runOrganization},
	{name: "team", argumentUsage: "<action> <org> <team> [arguments]", summary: "manage an organization's teams (" + teamUsage + ")", withBackend: runTeam},
	{name: "share-link", argumentUsage: "<item>|--text|open <url>|revoke <id>", summary: "share an item or text with anyone through an end-to-end encrypted link (--field, --expires 1h|1d|7d|30d, --views, --passphrase)", run: runShareLink, isLocal: true},
	{name: "share-links", summary: "list your live share links", withBackend: runListShareLinks},
	{name: "account", argumentUsage: "change-password|replace-secret-key [-o file]|delete|pending [list|resume|kit|acknowledge|discard]", summary: "change credentials, delete your account, or resume protected setup and Emergency Kit delivery", run: runAccount, appAlternative: "use the Hisar app under Settings → Security (master password, Secret Key) or Settings → Account (delete), or its pending recovery screen"},
	{name: "emergency-kit", argumentUsage: "[-o file]", summary: "save your Emergency Kit PDF (sign-in address, email, Secret Key)", run: runEmergencyKit, appAlternative: "save it in the Hisar app under Settings → Security → Emergency Kit"},
	{name: "devices", summary: "list your signed-in devices", withBackend: runListDevices},
	{name: "revoke-device", argumentUsage: "<id>", summary: "sign out a device", withBackend: runRevokeDevice},
}

var removedRecoveryCommandNames = []string{"recover", "recovery-setup", "recovery-enable", "recovery-start", "recovery-list", "recovery-complete"}

var errRecoveryRemoved = errors.New("account recovery was removed; sign in with your Emergency Kit")

func init() {
	for _, name := range removedRecoveryCommandNames {
		commands = append(commands, command{name: name, run: runRemovedRecoveryCommand, isLocal: true, isHidden: true})
	}
}

func runRemovedRecoveryCommand(context.Context, *flag.FlagSet, []string) error {
	return errRecoveryRemoved
}

func printUsage(output io.Writer) {
	fmt.Fprintln(output, "usage: hisar [--standalone] <command> [arguments]\n\ncommands:")
	for _, command := range commands {
		if command.isHidden {
			continue
		}
		fmt.Fprintf(output, "  %-18s %-35s %s\n", command.name, command.argumentUsage, command.summary)
	}
	fmt.Fprintln(output, `
hisar uses the Hisar app when it lets the command-line tool use it, and starts the app when it is
installed but not running (HISAR_NO_LAUNCH=1 never starts it); otherwise it signs in on its own. --standalone or HISAR_STANDALONE=1 always signs in on its own.
hisar --version prints the version.`)
}

type globalOption struct {
	isStandalone bool
	isVersion    bool
	isHelp       bool
}

// parseGlobalOption reads the options before the command name.
func parseGlobalOption(args []string) (globalOption, []string, error) {
	var option globalOption
	for len(args) > 0 {
		switch args[0] {
		case "--standalone", "-standalone":
			option.isStandalone = true
		case "--version", "-version":
			option.isVersion = true
		case "--help", "-help", "-h", "help":
			option.isHelp = true
		default:
			if len(args[0]) > 0 && args[0][0] == '-' {
				return option, nil, fmt.Errorf("unknown option %s", args[0])
			}
			return option, args, nil
		}
		args = args[1:]
	}
	return option, args, nil
}

func findCommand(name string) (command, bool) {
	for _, candidate := range commands {
		if candidate.name == name {
			return candidate, true
		}
	}
	return command{}, false
}

func runCommand(ctx context.Context, selected command, isStandaloneForced bool, flags *flag.FlagSet, args []string) error {
	isStandaloneForcedByOption = isStandaloneForced
	if selected.isLocal {
		return selected.run(ctx, flags, args)
	}
	if selected.withBackend != nil {
		selectedMode = chooseModeWithLaunch(isStandaloneForced, true)
		var access backend = &standaloneBackend{}
		if selectedMode.client != nil {
			defer selectedMode.client.Close()
			access = appBackend{client: selectedMode.client}
		}
		return access.finish(selected.withBackend(ctx, access, flags, args))
	}
	selectedMode = chooseModeWithLaunch(isStandaloneForced, selected.runThroughApp != nil)
	if selectedMode.client == nil {
		return selected.run(ctx, flags, args)
	}
	defer selectedMode.client.Close()
	if selected.runThroughApp == nil {
		return describeStandaloneOnly(selected.name, selected.appAlternative)
	}
	return selected.runThroughApp(ctx, selectedMode.client, flags, args)
}

func main() {
	option, args, err := parseGlobalOption(os.Args[1:])
	switch {
	case err != nil:
		fmt.Fprintln(standardError(), "hisar:", err)
		printUsage(os.Stderr)
		os.Exit(2)
	case option.isVersion:
		fmt.Println("hisar", version)
		return
	case option.isHelp:
		printUsage(os.Stdout)
		return
	case len(args) == 0:
		printUsage(os.Stderr)
		os.Exit(2)
	}
	selected, isFound := findCommand(args[0])
	if !isFound {
		printUsage(os.Stderr)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	flags := flag.NewFlagSet(selected.name, flag.ContinueOnError)
	flags.Usage = func() {
		fmt.Fprintf(standardError(), "usage: hisar %s %s\n", selected.name, selected.argumentUsage)
		flags.PrintDefaults()
	}
	err = runCommand(ctx, selected, option.isStandalone, flags, args[1:])
	var exitCode exitCodeError
	updateMessage, isUpdateRequired := describeUpdateRequired(err)
	switch {
	case isUpdateRequired:
		fmt.Fprintln(standardError(), "hisar:", updateMessage)
		os.Exit(1)
	case errors.As(err, &exitCode):
		os.Exit(int(exitCode))
	case errors.Is(err, errUsage):
		flags.Usage()
		os.Exit(2)
	case errors.Is(err, flag.ErrHelp):
		os.Exit(2)
	case err != nil:
		fmt.Fprintln(standardError(), "hisar:", escapeControlCharacters(err.Error()))
		os.Exit(1)
	}
}

func describeStandaloneOnly(commandName, appAlternative string) error {
	return fmt.Errorf("hisar is using the Hisar app, so `%s` is not available here; %s, or run `hisar --standalone %s`", commandName, appAlternative, commandName)
}
