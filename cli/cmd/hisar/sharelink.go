package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
	"unicode/utf8"

	"golang.org/x/term"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/core/sharelink"
)

const maxPassphraseAttempt = 3

type repeatedFlag []string

func (values *repeatedFlag) String() string { return strings.Join(*values, ",") }

func (values *repeatedFlag) Set(value string) error {
	*values = append(*values, value)
	return nil
}

type shareLinkOption struct {
	vaultName      string
	fieldNames     repeatedFlag
	expiry         string
	views          string
	isPassphrase   bool
	isText         bool
	title          string
	positionals    []string
	expiryDuration time.Duration
	maxViews       int
}

func parseShareLinkOption(flags *flag.FlagSet, args []string) (*shareLinkOption, error) {
	option := &shareLinkOption{}
	flags.StringVar(&option.vaultName, "v", "Personal", "vault name or ID")
	flags.Var(&option.fieldNames, "field", "a field to share: username, password, url, notes or a custom field's label (repeat for more; default username and password)")
	flags.StringVar(&option.expiry, "expires", "7d", "1h, 1d, 7d or 30d")
	flags.StringVar(&option.views, "views", "1", "how many times the link can be opened: 1-100 or unlimited")
	flags.BoolVar(&option.isPassphrase, "passphrase", false, "also require a passphrase (asked for; send it to the recipient another way)")
	flags.BoolVar(&option.isText, "text", false, "share text read from stdin instead of an item")
	flags.StringVar(&option.title, "title", "", "title of a --text share")
	var err error
	if option.positionals, err = parse(flags, args); err != nil {
		return nil, err
	}
	return option, nil
}

func (option *shareLinkOption) validateCreation() error {
	if option.isText != (len(option.positionals) == 0) || len(option.positionals) > 1 {
		return errUsage
	}
	if option.isText && len(option.fieldNames) > 0 {
		return errors.New("--field is for items; --text shares one text")
	}
	var err error
	if option.expiryDuration, err = parseExpiry(option.expiry); err != nil {
		return err
	}
	option.maxViews, err = parseViews(option.views)
	return err
}

func runShareLink(ctx context.Context, flags *flag.FlagSet, args []string) error {
	option, err := parseShareLinkOption(flags, args)
	if err != nil {
		return err
	}
	if len(option.positionals) > 0 && !option.isText {
		switch option.positionals[0] {
		case "open":
			if len(option.positionals) != 2 {
				return errUsage
			}
			return openShareLink(ctx, option.positionals[1])
		case "revoke":
			if len(option.positionals) != 2 {
				return errUsage
			}
			return revokeShareLink(ctx, option.positionals[1])
		}
	}
	if err := option.validateCreation(); err != nil {
		return err
	}
	mode := chooseMode(isStandaloneForcedByOption)
	selectedMode = mode
	if mode.client != nil {
		defer mode.client.Close()
		return createShareLinkThroughApp(ctx, mode.client, option)
	}
	return createShareLinkStandalone(ctx, option)
}

func promptShareLinkPassphrase(option *shareLinkOption) ([]byte, error) {
	if !option.isPassphrase {
		return nil, nil
	}
	for {
		passphrase, err := promptSecret("Passphrase for the link")
		if err != nil {
			return nil, err
		}
		if utf8.RuneCount(passphrase) < sharelink.MinPassphraseLength {
			if !term.IsTerminal(int(os.Stdin.Fd())) {
				return nil, sharelink.ErrPassphraseTooShort
			}
			fmt.Fprintf(os.Stderr, "Use at least %d characters.\n", sharelink.MinPassphraseLength)
			continue
		}
		repeated, err := promptSecret("Repeat the passphrase")
		if err != nil {
			return nil, err
		}
		if string(repeated) == string(passphrase) {
			clear(repeated)
			return passphrase, nil
		}
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return nil, errors.New("the passphrases do not match")
		}
		fmt.Fprintln(os.Stderr, "The passphrases do not match.")
	}
}

func readShareText() (string, error) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintln(os.Stderr, "Type or paste the text to share, then press Ctrl-D:")
	}
	text, err := io.ReadAll(io.LimitReader(stdinReader, sharelink.MaxPayloadSize+1))
	if err != nil {
		return "", err
	}
	if len(text) > sharelink.MaxPayloadSize {
		return "", sharelink.ErrTooLarge
	}
	return strings.TrimRight(string(text), "\r\n"), nil
}

func createShareLinkThroughApp(ctx context.Context, client *appClient, option *shareLinkOption) error {
	passphrase, err := promptShareLinkPassphrase(option)
	if err != nil {
		return err
	}
	defer clear(passphrase)
	change := &cliprotocol.ShareLinkChange{
		Fields: option.fieldNames, ExpiresInSeconds: int64(option.expiryDuration / time.Second), MaxViews: option.maxViews, Passphrase: string(passphrase),
	}
	request := cliprotocol.Request{Type: cliprotocol.TypeCreateShareLink, ShareLink: change}
	if option.isText {
		if change.Text, err = readShareText(); err != nil {
			return err
		}
		change.Title = option.title
	} else {
		request.Vault, request.Query = option.vaultName, option.positionals[0]
	}
	fmt.Fprintln(os.Stderr, "Approve the share link in Hisar's approval window…")
	var result cliprotocol.ShareLinkResult
	if err := client.callWithTimeout(ctx, request, &result, appRequestTimeout); err != nil {
		return err
	}
	printCreatedShareLink(result.ShareLink)
	return nil
}

func createShareLinkStandalone(ctx context.Context, option *shareLinkOption) error {
	options := session.ShareLinkOptions{Expiry: option.expiryDuration, MaxViews: option.maxViews}
	if option.isText {
		currentState, unlockedSession, err := unlock(ctx)
		if err != nil {
			return err
		}
		err = func() error {
			if options.Passphrase, err = promptShareLinkPassphrase(option); err != nil {
				return err
			}
			defer clear(options.Passphrase)
			text, err := readShareText()
			if err != nil {
				return err
			}
			created, err := unlockedSession.ShareText(ctx, option.title, text, options)
			if err != nil {
				return err
			}
			printCreatedShareLink(convertToCommandLineShareLink(created, true))
			return nil
		}()
		return finishSession(currentState, unlockedSession, err)
	}
	return runWithItem(ctx, option.vaultName, option.positionals[0], func(unlockedSession *session.Session, vault *session.Vault, vaultItem *item.Item) error {
		fieldKeys, err := sharelink.FindFieldKeys(vaultItem, option.fieldNames)
		if err != nil {
			return err
		}
		if options.Passphrase, err = promptShareLinkPassphrase(option); err != nil {
			return err
		}
		defer clear(options.Passphrase)
		created, err := unlockedSession.ShareItem(ctx, vault, vaultItem, fieldKeys, options)
		if err != nil {
			return err
		}
		printCreatedShareLink(convertToCommandLineShareLink(created, true))
		return nil
	})
}

func convertToCommandLineShareLink(link *session.ShareLink, isURLIncluded bool) cliprotocol.ShareLink {
	shareLink := cliprotocol.ShareLink{
		ID: link.ID, Title: link.Title, FieldLabels: append([]string{}, link.FieldLabels...), IsText: link.IsText,
		CreatedAt: link.CreatedAt.UTC().Format(time.RFC3339), ExpiresAt: link.ExpiresAt.UTC().Format(time.RFC3339),
		MaxViews: link.MaxViews, ViewCount: link.ViewCount, HasPassphrase: link.HasPassphrase,
	}
	if isURLIncluded {
		shareLink.URL = link.URL
	}
	return shareLink
}

func describeViews(link cliprotocol.ShareLink) string {
	if link.MaxViews == 0 {
		return fmt.Sprintf("%d/unlimited", link.ViewCount)
	}
	return fmt.Sprintf("%d/%d", link.ViewCount, link.MaxViews)
}

func formatDateTime(timestamp string) string {
	if parsed, err := time.Parse(time.RFC3339, timestamp); err == nil {
		return parsed.Local().Format("2006-01-02 15:04")
	}
	return timestamp
}

func printCreatedShareLink(link cliprotocol.ShareLink) {
	views := "unlimited views"
	if link.MaxViews == 1 {
		views = "1 view"
	} else if link.MaxViews > 1 {
		views = fmt.Sprintf("%d views", link.MaxViews)
	}
	passphrase := ""
	if link.HasPassphrase {
		passphrase = "; send the passphrase another way"
	}
	fmt.Fprintf(os.Stderr, "Share link for %q (%s): expires %s, %s%s.\n", link.Title, strings.Join(link.FieldLabels, ", "), formatDateTime(link.ExpiresAt), views, passphrase)
	fmt.Println(link.URL)
}

func printShareLinks(shareLinks []cliprotocol.ShareLink) error {
	if len(shareLinks) == 0 {
		fmt.Fprintln(os.Stderr, "No live share links.")
		return nil
	}
	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "ID\tTITLE\tSHARES\tVIEWS\tEXPIRES\tPASSPHRASE")
	for _, link := range shareLinks {
		passphrase := "no"
		if link.HasPassphrase {
			passphrase = "yes"
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n", link.ID, link.Title, strings.Join(link.FieldLabels, ", "), describeViews(link), formatDateTime(link.ExpiresAt), passphrase)
	}
	return writer.Flush()
}

func parseWithoutArgument(flags *flag.FlagSet, args []string) error {
	positionals, err := parse(flags, args)
	if err == nil && len(positionals) != 0 {
		return errUsage
	}
	return err
}

func revokeShareLink(ctx context.Context, shareLinkID string) error {
	mode := chooseMode(isStandaloneForcedByOption)
	selectedMode = mode
	if mode.client != nil {
		defer mode.client.Close()
		if err := mode.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeRevokeShareLink, ShareLinkID: shareLinkID}, nil); err != nil {
			return err
		}
	} else {
		currentState, unlockedSession, err := unlock(ctx)
		if err != nil {
			return err
		}
		err = unlockedSession.RevokeShareLink(ctx, shareLinkID)
		if err := finishSession(currentState, unlockedSession, err); err != nil {
			return err
		}
	}
	fmt.Fprintln(os.Stderr, "Revoked the share link; it no longer opens.")
	return nil
}

func openShareLink(ctx context.Context, link string) error {
	fetched, err := sharelink.Fetch(ctx, nil, link)
	if err != nil {
		return err
	}
	var payload *sharelink.Payload
	for attempt := 1; ; attempt++ {
		var passphrase []byte
		if fetched.GetPassphraseParams() != nil {
			if passphrase, err = promptSecret("Passphrase"); err != nil {
				return err
			}
		}
		payload, err = fetched.Open(passphrase)
		clear(passphrase)
		if err == nil {
			break
		}
		if !errors.Is(err, sharelink.ErrWrongKey) || fetched.GetPassphraseParams() == nil || attempt >= maxPassphraseAttempt || !term.IsTerminal(int(os.Stdin.Fd())) {
			return err
		}
		fmt.Fprintln(os.Stderr, "That passphrase doesn't open this link.")
	}
	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(writer, "title:\t%s\n", payload.Title)
	for _, field := range payload.Fields {
		fmt.Fprintf(writer, "%s:\t%s\n", strings.ToLower(field.Label), field.Value)
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	switch viewsLeft := fetched.Link.MaxViews - fetched.Link.ViewCount; {
	case fetched.Link.MaxViews == 0:
		fmt.Fprintf(os.Stderr, "The link works until %s.\n", fetched.Link.ExpiresAt.Local().Format("2006-01-02 15:04"))
	case viewsLeft <= 0:
		fmt.Fprintln(os.Stderr, "That was the last view: this link no longer works.")
	case viewsLeft == 1:
		fmt.Fprintf(os.Stderr, "1 more view until %s.\n", fetched.Link.ExpiresAt.Local().Format("2006-01-02 15:04"))
	default:
		fmt.Fprintf(os.Stderr, "%d more views until %s.\n", viewsLeft, fetched.Link.ExpiresAt.Local().Format("2006-01-02 15:04"))
	}
	return nil
}

func parseExpiry(text string) (time.Duration, error) {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "1h", "1hour", "1 hour":
		return sharelink.ExpiryHour, nil
	case "1d", "1day", "1 day", "24h":
		return sharelink.ExpiryDay, nil
	case "7d", "7days", "7 days", "1w":
		return sharelink.ExpiryWeek, nil
	case "30d", "30days", "30 days":
		return sharelink.ExpiryMonth, nil
	}
	return 0, fmt.Errorf("expiry %q: use 1h, 1d, 7d or 30d", text)
}

// parseViews returns 0 for unlimited.
func parseViews(text string) (int, error) {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "unlimited" || text == "0" {
		return 0, nil
	}
	views, err := strconv.Atoi(text)
	if err != nil || views < 1 || views > sharelink.MaxViews {
		return 0, fmt.Errorf("views %q: use a number from 1 to %d, or unlimited", text, sharelink.MaxViews)
	}
	return views, nil
}

func runListShareLinks(ctx context.Context, access backend, flags *flag.FlagSet, args []string) error {
	if err := parseWithoutArgument(flags, args); err != nil {
		return err
	}
	shareLinks, err := access.listShareLinks(ctx)
	if err != nil {
		return err
	}
	return printShareLinks(shareLinks)
}

// runWithItem unlocks, finds the vault and item, and runs action; share-link chooses its mode itself and needs the session.
func runWithItem(ctx context.Context, vaultName, query string, action func(*session.Session, *session.Vault, *item.Item) error) error {
	currentState, unlockedSession, err := unlock(ctx)
	if err != nil {
		return err
	}
	err = func() error {
		vault, err := unlockedSession.FindVault(ctx, vaultName)
		if err != nil {
			return err
		}
		items, err := unlockedSession.ListItems(ctx, vault)
		if err != nil {
			return err
		}
		vaultItem, err := findItem(items, query)
		if err != nil {
			return err
		}
		return action(unlockedSession, vault, vaultItem)
	}()
	return finishSession(currentState, unlockedSession, err)
}
