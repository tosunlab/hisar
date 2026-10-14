package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/tryhisar/hisar/core/apitypes"
)

func invite(ctx context.Context, args []string) error {
	flagSet := flag.NewFlagSet("invite", flag.ExitOnError)
	email := flagSet.String("email", "", "email address of the invitee")
	role := flagSet.String("role", "", "admin or member")
	flagSet.Parse(args)

	database, server, err := setup(ctx)
	if err != nil {
		return err
	}
	defer database.Close()

	accounts, err := database.ListAccounts(ctx)
	if err != nil {
		return err
	}
	defaultRole := apitypes.RoleMember
	if len(accounts) == 0 {
		defaultRole = apitypes.RoleAdmin
	}

	input := bufio.NewReader(os.Stdin)
	if *email == "" {
		*email = prompt(input, "Email", "")
	}
	if *role == "" {
		*role = prompt(input, "Role (admin/member)", defaultRole)
	}
	*email = strings.ToLower(strings.TrimSpace(*email))
	if !strings.Contains(*email, "@") {
		return fmt.Errorf("invalid email %q", *email)
	}
	if *role != apitypes.RoleAdmin && *role != apitypes.RoleMember {
		return fmt.Errorf("invalid role %q", *role)
	}

	response, err := server.CreateInvite(ctx, *email, *role, nil)
	if err != nil {
		return err
	}
	fmt.Printf("Invite for %s (%s), valid for 7 days:\n%s\n", *email, *role, response.URL)
	return nil
}

func prompt(input *bufio.Reader, label, defaultValue string) string {
	if defaultValue != "" {
		fmt.Printf("%s [%s]: ", label, defaultValue)
	} else {
		fmt.Printf("%s: ", label)
	}
	line, _ := input.ReadString('\n')
	if line = strings.TrimSpace(line); line == "" {
		return defaultValue
	}
	return line
}
