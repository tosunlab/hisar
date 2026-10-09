package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
)

type command struct {
	name          string
	argumentUsage string
	summary       string
	run           func(ctx context.Context, flags *flag.FlagSet, allArg []string) error
}

var allCommand = []command{
	{"signup", "<invite-link>", "create your account from an invite", runSignup},
	{"login", "", "sign in on this device", runLogin},
	{"logout", "", "sign out on this device", runLogout},
	{"whoami", "", "show the signed-in account", runWhoami},
	{"vaults", "", "list vaults", runListAllVault},
	{"vault-create", "<name>", "create a shared vault", runCreateVault},
	{"ls", "[-v vault]", "list items", runListAllItem},
	{"get", "<item> [-f field] [-v vault]", "show an item, or one field of it (e.g. -f password, -f code)", runGet},
	{"add", "[-v vault]", "add a login", runAdd},
	{"edit", "<item> [-v vault]", "edit an item", runEdit},
	{"rm", "<item> [-v vault]", "delete an item", runRemove},
	{"import", "<file> [-v vault] [-format f] [-y]", "import a 1Password, Bitwarden or browser CSV export", runImport},
	{"generate", "[-n length] [-no-symbols]", "generate a password", runGenerate},
	{"members", "", "list family members", runListAllMember},
	{"share", "<vault> <email> [-r role]", "share a vault (role: owner, editor, reader)", runShare},
	{"unshare", "<vault> <email>", "remove someone from a vault and rotate its key", runUnshare},
	{"invite", "<email> [-r role]", "invite someone (admin only)", runInvite},
	{"emergency-kit", "[-o file]", "save your Emergency Kit PDF (sign-in address, email, Secret Key)", runEmergencyKit},
	{"devices", "", "list your signed-in devices", runListAllDevice},
	{"revoke-device", "<id>", "sign out a device", runRevokeDevice},
	{"recovery-setup", "", "create the recovery key and wrap your vaults to it (admin only)", runRecoverySetup},
	{"recovery-enable", "", "make your vaults recoverable by the admin", runRecoveryEnable},
	{"recovery-start", "<email>", "create a recovery link for someone (admin only)", runRecoveryStart},
	{"recover", "<recovery-link>", "choose a new master password and Secret Key", runRecover},
	{"recovery-list", "", "list pending recoveries (admin only)", runListAllPendingRecovery},
	{"recovery-complete", "<email>", "give someone their vaults back after recovery (admin only)", runRecoveryComplete},
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "usage: hisar <command> [arguments]\n\ncommands:")
	for _, command := range allCommand {
		fmt.Fprintf(os.Stderr, "  %-18s %-35s %s\n", command.name, command.argumentUsage, command.summary)
	}
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	for _, command := range allCommand {
		if command.name != os.Args[1] {
			continue
		}
		flags := flag.NewFlagSet(command.name, flag.ContinueOnError)
		flags.Usage = func() {
			fmt.Fprintf(os.Stderr, "usage: hisar %s %s\n", command.name, command.argumentUsage)
			flags.PrintDefaults()
		}
		err := command.run(ctx, flags, os.Args[2:])
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
		return
	}
	printUsage()
	os.Exit(2)
}
