package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/tryhisar/hisar/core/client"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/proto"
)

const (
	planUsage        = "[upgrade [--monthly|--yearly]|manage]"
	yearlyPriceLabel = "€19.99 a year"
	monthlyPrice     = "€2.49 a month"
)

type planOption struct {
	isMonthly bool
	isYearly  bool
}

func addPlanFlag(flags *flag.FlagSet) *planOption {
	option := &planOption{}
	flags.BoolVar(&option.isMonthly, "monthly", false, "upgrade: pay "+monthlyPrice)
	flags.BoolVar(&option.isYearly, "yearly", false, "upgrade: pay "+yearlyPriceLabel+" (the default)")
	return option
}

// choosePlanInterval asks on a terminal and takes yearly when nothing is said, as the pricing page does.
func choosePlanInterval(option *planOption) (string, error) {
	switch {
	case option.isMonthly && option.isYearly:
		return "", errors.New("choose --monthly or --yearly, not both")
	case option.isMonthly:
		return proto.BillingIntervalMonthly, nil
	case option.isYearly:
		return proto.BillingIntervalYearly, nil
	}
	answer, err := prompt(fmt.Sprintf("Yearly (%s) or monthly (%s)?", yearlyPriceLabel, monthlyPrice), "yearly")
	if errors.Is(err, io.EOF) {
		return proto.BillingIntervalYearly, nil
	}
	if err != nil {
		return "", err
	}
	switch strings.ToLower(answer) {
	case "yearly", "year", "y":
		return proto.BillingIntervalYearly, nil
	case "monthly", "month", "m":
		return proto.BillingIntervalMonthly, nil
	}
	return "", fmt.Errorf("answer yearly or monthly, not %q", answer)
}

func runPlan(ctx context.Context, flags *flag.FlagSet, allArg []string) error {
	option := addPlanFlag(flags)
	allPositional, err := parse(flags, allArg)
	if err != nil {
		return err
	}
	currentState, err := loadState()
	if err != nil {
		return err
	}
	if err := requireLogin(currentState); err != nil {
		return err
	}
	apiClient := createAuthedClient(currentState)
	switch getPlanAction(allPositional) {
	case "":
		protoPlan, err := apiClient.GetPlan(ctx)
		if err != nil {
			return describeBillingError(err)
		}
		return writePlan(os.Stdout, cliprotocol.ConvertToPlanResult(protoPlan), time.Now())
	case "upgrade":
		interval, err := choosePlanInterval(option)
		if err != nil {
			return err
		}
		checkoutURL, err := apiClient.CreateCheckout(ctx, interval)
		if err != nil {
			return describeBillingError(err)
		}
		return printBillingLink(checkoutURL, "Open this link to pay with Lemon Squeezy; Hisar switches to Pro once the payment is done.")
	case "manage":
		manageURL, err := apiClient.CreateManageLink(ctx)
		if err != nil {
			return describeBillingError(err)
		}
		return printBillingLink(manageURL, "Open this link to change the payment method, see invoices or cancel. It works for a day.")
	}
	return errUsage
}

func runPlanThroughApp(ctx context.Context, appConnection *appClient, flags *flag.FlagSet, allArg []string) error {
	option := addPlanFlag(flags)
	allPositional, err := parse(flags, allArg)
	if err != nil {
		return err
	}
	switch getPlanAction(allPositional) {
	case "":
		var plan cliprotocol.PlanResult
		if err := appConnection.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeGetPlan}, &plan); err != nil {
			return err
		}
		return writePlan(os.Stdout, plan, time.Now())
	case "upgrade":
		interval, err := choosePlanInterval(option)
		if err != nil {
			return err
		}
		var link cliprotocol.LinkResult
		if err := appConnection.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeCreateCheckout, Interval: interval}, &link); err != nil {
			return err
		}
		return printBillingLink(link.URL, "Open this link to pay with Lemon Squeezy; Hisar switches to Pro once the payment is done.")
	case "manage":
		var link cliprotocol.LinkResult
		if err := appConnection.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeManagePlan}, &link); err != nil {
			return err
		}
		return printBillingLink(link.URL, "Open this link to change the payment method, see invoices or cancel. It works for a day.")
	}
	return errUsage
}

// getPlanAction returns "", "upgrade" or "manage", and "?" for anything else.
func getPlanAction(allPositional []string) string {
	switch {
	case len(allPositional) == 0:
		return ""
	case len(allPositional) == 1 && (allPositional[0] == "upgrade" || allPositional[0] == "manage"):
		return allPositional[0]
	}
	return "?"
}

func printBillingLink(link, explanation string) error {
	parsedURL, err := url.Parse(link)
	if err != nil || parsedURL.Scheme != "https" {
		return errors.New("the server returned a link that is not https")
	}
	fmt.Fprintln(os.Stderr, explanation)
	fmt.Println(link)
	return nil
}

func describeBillingError(err error) error {
	switch {
	case client.IsStatus(err, http.StatusNotImplemented):
		return errors.New("plans are not sold on this server")
	case client.IsStatus(err, http.StatusConflict):
		return errors.New("you already have Pro")
	case client.IsStatus(err, http.StatusNotFound):
		return errors.New("there is no subscription to manage")
	case client.IsStatus(err, http.StatusFailedDependency):
		return errors.New("the payment provider did not answer; try again in a minute")
	}
	return err
}

// describePlanLimitError turns a refused create into the apps' sentence, with the upgrade hint on Free.
func describePlanLimitError(err error) error {
	if limit, ok := client.GetPlanLimit(err); ok {
		return errors.New(cliprotocol.FormatPlanLimitMessage(proto.DescribePlanLimit(*limit), limit.Plan))
	}
	return err
}

func formatPlanDate(text string) string {
	parsedTime, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return text
	}
	return parsedTime.Local().Format("2 January 2006")
}

func formatPlanMaximum(maxCount int) string {
	if maxCount == 0 {
		return "unlimited"
	}
	return fmt.Sprint(maxCount)
}

func describeSubscription(plan cliprotocol.PlanResult, now time.Time) string {
	interval := plan.SubscriptionInterval
	switch plan.SubscriptionStatus {
	case "":
		return ""
	case proto.SubscriptionActive:
		return fmt.Sprintf("%s, renews on %s", interval, formatPlanDate(plan.RenewsAt))
	case proto.SubscriptionOnTrial:
		return fmt.Sprintf("%s, trial ends on %s", interval, formatPlanDate(plan.TrialEndsAt))
	case proto.SubscriptionPastDue:
		return interval + ", the last payment didn't go through; Pro stays on while Lemon Squeezy retries"
	case proto.SubscriptionCancelled:
		if endsAt, err := time.Parse(time.RFC3339, plan.EndsAt); err == nil && endsAt.After(now) {
			return fmt.Sprintf("%s, cancelled: Pro until %s", interval, formatPlanDate(plan.EndsAt))
		}
	}
	if plan.EndsAt != "" {
		return fmt.Sprintf("%s, ended on %s", interval, formatPlanDate(plan.EndsAt))
	}
	return interval + ", " + strings.ReplaceAll(plan.SubscriptionStatus, "_", " ")
}

func writePlan(output io.Writer, plan cliprotocol.PlanResult, now time.Time) error {
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	planTitle := proto.GetPlanTitle(plan.Plan)
	if plan.IsFoundingMember {
		planTitle += " · Founding member, free for life"
	}
	fmt.Fprintf(writer, "Plan:\t%s\n", planTitle)
	if subscription := describeSubscription(plan, now); subscription != "" {
		fmt.Fprintf(writer, "Subscription:\t%s\n", subscription)
	}
	fmt.Fprintf(writer, "Shared vaults:\t%d of %s, up to %d people each\n", plan.SharedVaultCount, formatPlanMaximum(plan.MaxSharedVaultCount), plan.MaxVaultPersonCount)
	fmt.Fprintf(writer, "Share links:\t%d of %s live, up to %d days and %d views\n", plan.LiveShareLinkCount, formatPlanMaximum(plan.MaxLiveShareLinkCount),
		plan.MaxShareLinkExpirySeconds/(24*60*60), plan.MaxShareLinkViewCount)
	if err := writer.Flush(); err != nil {
		return err
	}
	switch {
	case plan.IsCheckoutAvailable:
		fmt.Fprintln(output, "\n"+cliprotocol.UpgradeHint)
	case plan.IsManageAvailable:
		fmt.Fprintln(output, "\nManage the subscription: hisar plan manage")
	}
	return nil
}
