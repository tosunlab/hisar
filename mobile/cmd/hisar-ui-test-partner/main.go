// Command hisar-ui-test-partner plays the partner of the iPhone and Android UI tests for scripts/ui-test-partner.sh. It
// signs up from an invite, writes the ready file and answers every 2 seconds, unlocked as the partner's own Hisar would
// be: it accepts every share invitation the partner gets, confirms and completes every share the partner sent, accepts
// every organization invitation, gives team members the keys of the partner's vaults and those vaults a new key once
// someone lost access, and writes "Seen by partner" into Contoso's Runbooks once it reads Pager there.
//
// The account's keys are derived once, at sign-up, with Argon2id (64 MiB, 24 passes, 4 lanes), about a second of CPU. A
// partner on the command line derived them on every pass, every 5 to 6 seconds, on the runner the simulator shares.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"slices"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/invitelink"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/session"
)

type partner struct {
	session     *session.Session
	hasAnswered bool
}

func main() {
	invite := flag.String("invite", "", "the invite link the partner signs up with (required)")
	email := flag.String("email", "", "the partner's email (required)")
	name := flag.String("name", "", "the partner's name (required)")
	password := flag.String("password", "", "the partner's master password (required)")
	readyPath := flag.String("ready-file", "", "the file written once the partner signed up (required)")
	flag.Parse()
	if *invite == "" || *email == "" || *name == "" || *password == "" || *readyPath == "" {
		log.Fatal("-invite, -email, -name, -password and -ready-file are required")
	}
	server, token, ok := invitelink.Parse(*invite)
	if !ok {
		log.Fatalf("invalid invite link %q", *invite)
	}
	ctx := context.Background()
	signedUp, _, err := session.SignUp(ctx, apiclient.New(server), token, *email, *name, []byte(*password), apitypes.DeviceInfo{Name: "UI test partner", Platform: "go"})
	if err != nil {
		log.Fatalf("signing up the partner: %v", err)
	}
	state := &localstate.State{Pins: map[string][]byte{}, Seqs: map[string]int64{}}
	signedUp.SeqStore = state
	signedUp.MemberVerifier = state.CheckMemberPin
	if err := os.WriteFile(*readyPath, nil, 0o600); err != nil {
		log.Fatal(err)
	}
	log.Print("signed up; answering as the partner")
	answerer := &partner{session: signedUp}
	for {
		answerer.answer(ctx)
		time.Sleep(2 * time.Second)
	}
}

func (p *partner) answer(ctx context.Context) {
	p.answerShareInvitations(ctx)
	p.completeOrganizationKeys(ctx)
	organizations, err := p.session.Client.ListOrganizations(ctx)
	if err != nil {
		log.Printf("listing organizations: %v", err)
		return
	}
	for _, invitation := range organizations.Invitations {
		if err := p.session.Client.AcceptOrganizationInvitation(ctx, invitation.ID); err != nil {
			log.Printf("accepting the invitation to %s: %v", invitation.OrganizationName, err)
		} else {
			log.Printf("joined %s", invitation.OrganizationName)
		}
	}
	if findOrganization(organizations, "Contoso") != nil && !p.hasAnswered {
		p.answerRunbooks(ctx)
	}
}

func (p *partner) answerShareInvitations(ctx context.Context) {
	invitations, err := p.session.Client.ListShareInvitations(ctx)
	if err != nil {
		log.Printf("listing share invitations: %v", err)
		return
	}
	for _, received := range invitations.Received {
		if err := p.session.Client.AcceptShareInvitation(ctx, received.ID); err != nil {
			log.Printf("accepting share invitation %s: %v", received.ID, err)
		}
	}
	isAnyAccepted := false
	for _, sent := range invitations.Sent {
		if sent.State != apitypes.ShareInvitationAccepted {
			continue
		}
		isAnyAccepted = true
		if sent.Recipient != nil && (sent.Intent == nil || sent.Intent.Context.RecipientID == "") {
			if err := p.session.ConfirmShareRecipient(ctx, sent.ID, session.ShareInvitationReviewHash(sent)); err != nil {
				log.Printf("confirming %s: %v", sent.Email, err)
			}
		}
	}
	if !isAnyAccepted {
		return
	}
	completed, err := p.session.CompleteAcceptedShares(ctx)
	for _, share := range completed {
		log.Printf("shared %q with %s as %s", share.VaultName, share.Recipient.Email, share.Role)
	}
	if err != nil {
		log.Printf("completing shares: %v", err)
	}
}

func (p *partner) completeOrganizationKeys(ctx context.Context) {
	delivered, err := p.session.CompleteVaultKeyRequests(ctx)
	if len(delivered) > 0 {
		log.Printf("gave %d vault keys to team members", len(delivered))
	}
	if err != nil {
		log.Printf("giving vault keys: %v", err)
	}
	rotated, err := p.session.RotatePendingVaults(ctx)
	if len(rotated) > 0 {
		log.Printf("rotated the keys of %d vaults people lost access to", len(rotated))
	}
	if err != nil {
		log.Printf("rotating vaults: %v", err)
	}
}

// answerRunbooks writes "Seen by partner" into Runbooks once the admin account put Pager there for the partner's team.
func (p *partner) answerRunbooks(ctx context.Context) {
	runbooks := p.findVault(ctx, "Runbooks")
	if runbooks == nil || !p.hasItem(ctx, runbooks, func(entry item.Item) bool {
		return entry.Overview.Title == "Pager" && entry.Overview.Username == "pager-oncall"
	}) {
		return
	}
	answer := &item.Item{
		Overview: item.Overview{Category: item.CategoryLogin, Title: "Seen by partner", Username: "partner", URLs: []string{"https://runbooks.example.com"}},
		Details:  item.Details{Password: "seen-by-partner-password"},
	}
	if _, err := p.session.SaveItem(ctx, runbooks, answer); err != nil {
		log.Printf("writing into Runbooks: %v", err)
		return
	}
	p.hasAnswered = true
	log.Print(`saved "Seen by partner" to Runbooks`)
}

func findOrganization(organizations *apitypes.OrganizationList, name string) *apitypes.OrganizationSummary {
	for index := range organizations.Organizations {
		if organizations.Organizations[index].Name == name {
			return &organizations.Organizations[index]
		}
	}
	return nil
}

func (p *partner) findVault(ctx context.Context, name string) *session.Vault {
	vaults, err := p.session.ListVaults(ctx)
	if err != nil {
		log.Printf("listing vaults: %v", err)
		return nil
	}
	for index := range vaults {
		if vaults[index].Name == name {
			return &vaults[index]
		}
	}
	return nil
}

func (p *partner) hasItem(ctx context.Context, vault *session.Vault, isWanted func(item.Item) bool) bool {
	items, err := p.session.ListItems(ctx, vault)
	if err != nil {
		log.Printf("listing %s: %v", vault.Name, err)
		return false
	}
	return slices.ContainsFunc(items, isWanted)
}
