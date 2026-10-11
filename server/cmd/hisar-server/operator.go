package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/server/internal/plan"
	"github.com/tryhisar/hisar/server/internal/store"
)

const operatorUsage = `usage: hisar-server operator <command> [--email address] [--yes]

commands:
  show-plan        show one account's plan, subscription and usage
  grant-founding   make an account a founding member: Pro free for life
  revoke-founding  end an account's founding membership

Missing values are asked for; --yes skips the confirmation, for scripts.`

type operatorConsole struct {
	input  *bufio.Reader
	output io.Writer
}

func runOperatorCommand(ctx context.Context, allArg []string) error {
	if len(allArg) == 0 || strings.HasPrefix(allArg[0], "-") {
		return errors.New(operatorUsage)
	}
	databaseURL := os.Getenv("HISAR_DATABASE_URL")
	if databaseURL == "" {
		return errors.New("HISAR_DATABASE_URL is required")
	}
	database, err := store.Open(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()
	console := operatorConsole{input: bufio.NewReader(os.Stdin), output: os.Stdout}
	return runOperator(ctx, database, console, allArg[0], allArg[1:], time.Now())
}

func runOperator(ctx context.Context, database *store.Store, console operatorConsole, command string, allArg []string, now time.Time) error {
	flagSet := flag.NewFlagSet("operator "+command, flag.ContinueOnError)
	flagSet.SetOutput(console.output)
	email := flagSet.String("email", "", "the account's email address")
	isConfirmed := flagSet.Bool("yes", false, "do not ask for confirmation")
	if err := flagSet.Parse(allArg); err != nil {
		return err
	}
	switch command {
	case "show-plan", "grant-founding", "revoke-founding":
	default:
		return fmt.Errorf("unknown operator command %q\n\n%s", command, operatorUsage)
	}
	if *email == "" {
		*email = console.prompt("Email", "")
	}
	account, err := database.GetAccountByEmail(ctx, strings.ToLower(strings.TrimSpace(*email)))
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("no account has the email %q", *email)
	}
	if err != nil {
		return err
	}
	resolved, err := resolveOperatorPlan(ctx, database, account.ID, now)
	if err != nil {
		return err
	}
	switch command {
	case "show-plan":
		return printAccountPlan(ctx, database, console.output, account, resolved, now)
	case "grant-founding":
		if resolved.IsFoundingMember {
			fmt.Fprintf(console.output, "%s is already a founding member; nothing changed.\n", account.Email)
			return nil
		}
		if !*isConfirmed && !console.confirm(fmt.Sprintf("Make %s (%s) a founding member, with Pro free for life?", account.Email, account.Name)) {
			return errors.New("cancelled; nothing changed")
		}
		if _, err := database.GrantFoundingMember(ctx, account.ID); err != nil {
			return err
		}
		database.RecordAudit(ctx, &account.ID, nil, "founding_member_granted", map[string]string{"by": "operator"}, "")
		fmt.Fprintf(console.output, "%s is now a founding member.\n", account.Email)
	case "revoke-founding":
		if !resolved.IsFoundingMember {
			fmt.Fprintf(console.output, "%s is not a founding member; nothing changed.\n", account.Email)
			return nil
		}
		if !*isConfirmed && !console.confirm(fmt.Sprintf("End the founding membership of %s (%s)? Nothing is deleted; Free's limits apply to new shares and links unless a subscription gives Pro.", account.Email, account.Name)) {
			return errors.New("cancelled; nothing changed")
		}
		if _, err := database.RevokeFoundingMember(ctx, account.ID); err != nil {
			return err
		}
		database.RecordAudit(ctx, &account.ID, nil, "founding_member_revoked", map[string]string{"by": "operator"}, "")
		fmt.Fprintf(console.output, "%s is no longer a founding member.\n", account.Email)
	}
	resolved, err = resolveOperatorPlan(ctx, database, account.ID, now)
	if err != nil {
		return err
	}
	fmt.Fprintf(console.output, "Plan: %s\n", describeOperatorPlan(resolved))
	return nil
}

func resolveOperatorPlan(ctx context.Context, database *store.Store, accountID string, now time.Time) (plan.Resolved, error) {
	entitlements, err := database.GetEntitlements(ctx, accountID)
	if err != nil {
		return plan.Resolved{}, err
	}
	return plan.ResolvePlan(entitlements, now), nil
}

func describeOperatorPlan(resolved plan.Resolved) string {
	switch resolved.Source {
	case proto.PlanSourceFounding:
		return "Pro (founding member)"
	case proto.PlanSourceSubscription:
		return "Pro (subscription)"
	}
	return "Free"
}

func printAccountPlan(ctx context.Context, database *store.Store, output io.Writer, account *store.Account, resolved plan.Resolved, now time.Time) error {
	sharedVaultCount, err := database.CountOwnedSharedVault(ctx, account.ID)
	if err != nil {
		return err
	}
	liveShareLinkCount, err := database.CountLiveShareLink(ctx, account.ID)
	if err != nil {
		return err
	}
	limits := resolved.GetLimits()
	fmt.Fprintf(output, "Account:          %s (%s)\n", account.Email, account.Name)
	fmt.Fprintf(output, "Plan:             %s\n", describeOperatorPlan(resolved))
	fmt.Fprintf(output, "Subscription:     %s\n", describeOperatorSubscription(resolved.Subscription, now))
	fmt.Fprintf(output, "Shared vaults:    %d of %s\n", sharedVaultCount, formatOperatorLimit(limits.MaxSharedVaultCount))
	fmt.Fprintf(output, "People per vault: up to %d\n", limits.MaxVaultPersonCount)
	fmt.Fprintf(output, "Live share links: %d of %s, up to %d days\n", liveShareLinkCount, formatOperatorLimit(limits.MaxLiveShareLinkCount), limits.MaxShareLinkExpirySeconds/(24*60*60))
	return nil
}

func formatOperatorLimit(maxCount int) string {
	if maxCount == 0 {
		return "unlimited"
	}
	return fmt.Sprint(maxCount)
}

func describeOperatorSubscription(subscription *plan.Subscription, now time.Time) string {
	if subscription == nil {
		return "none"
	}
	description := fmt.Sprintf("%s, %s (Lemon Squeezy subscription %s, customer %s)", subscription.Status, subscription.Interval, subscription.ID, subscription.CustomerID)
	switch {
	case subscription.EndsAt != nil && subscription.EndsAt.After(now):
		description += ", active until " + subscription.EndsAt.UTC().Format(time.DateOnly)
	case subscription.EndsAt != nil:
		description += ", ended " + subscription.EndsAt.UTC().Format(time.DateOnly)
	case subscription.RenewsAt != nil:
		description += ", renews " + subscription.RenewsAt.UTC().Format(time.DateOnly)
	}
	return description
}

func (console operatorConsole) prompt(label, defaultValue string) string {
	if defaultValue != "" {
		fmt.Fprintf(console.output, "%s [%s]: ", label, defaultValue)
	} else {
		fmt.Fprintf(console.output, "%s: ", label)
	}
	line, _ := console.input.ReadString('\n')
	if line = strings.TrimSpace(line); line == "" {
		return defaultValue
	}
	return line
}

func (console operatorConsole) confirm(question string) bool {
	answer := strings.ToLower(console.prompt(question+" (y/N)", ""))
	return answer == "y" || answer == "yes"
}
