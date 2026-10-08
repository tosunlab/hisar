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
	name    string
	args    string
	summary string
	run     func(ctx context.Context, fs *flag.FlagSet, args []string) error
}

var commands = []command{
	{"signup", "<invite-link>", "create your account from an invite", cmdSignup},
	{"login", "", "sign in on this device", cmdLogin},
	{"logout", "", "sign out on this device", cmdLogout},
	{"whoami", "", "show the signed-in account", cmdWhoami},
	{"vaults", "", "list vaults", cmdVaults},
	{"vault-create", "<name>", "create a shared vault", cmdVaultCreate},
	{"ls", "[-v vault]", "list items", cmdList},
	{"get", "<item> [-f field] [-v vault]", "show an item, or one field of it", cmdGet},
	{"add", "[-v vault]", "add a login", cmdAdd},
	{"edit", "<item> [-v vault]", "edit an item", cmdEdit},
	{"rm", "<item> [-v vault]", "delete an item", cmdRemove},
	{"generate", "[-n length] [-no-symbols]", "generate a password", cmdGenerate},
	{"members", "", "list family members", cmdMembers},
	{"share", "<vault> <email> [-r role]", "share a vault (role: owner, editor, reader)", cmdShare},
	{"invite", "<email> [-r role]", "invite someone (admin only)", cmdInvite},
	{"devices", "", "list your signed-in devices", cmdDevices},
	{"revoke-device", "<id>", "sign out a device", cmdRevokeDevice},
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: hisar <command> [arguments]\n\ncommands:")
	for _, c := range commands {
		fmt.Fprintf(os.Stderr, "  %-14s %-32s %s\n", c.name, c.args, c.summary)
	}
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	for _, c := range commands {
		if c.name != os.Args[1] {
			continue
		}
		fs := flag.NewFlagSet(c.name, flag.ContinueOnError)
		fs.Usage = func() {
			fmt.Fprintf(os.Stderr, "usage: hisar %s %s\n", c.name, c.args)
			fs.PrintDefaults()
		}
		err := c.run(ctx, fs, os.Args[2:])
		switch {
		case errors.Is(err, errUsage):
			fs.Usage()
			os.Exit(2)
		case errors.Is(err, flag.ErrHelp):
			os.Exit(2)
		case err != nil:
			fmt.Fprintln(os.Stderr, "hisar:", err)
			os.Exit(1)
		}
		return
	}
	usage()
	os.Exit(2)
}
