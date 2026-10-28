// Command hisar-ui-test-partner plays the partner of the iPhone and Android UI tests once scripts/ui-test-partner.sh has
// set the partner up on the standalone CLI. It unlocks the partner's account once, as the partner's own Hisar would, and
// then answers every 2 seconds: it accepts every share invitation the partner gets, confirms and completes every share
// the partner sent, accepts every organization invitation, and gives team members the keys of the partner's vaults and
// those vaults a new key once someone lost access. For the organization tests it adds the joining account to Northwind's
// team Platform once it joined Northwind, removes it again once it saved "Remove me" in Deploy, and writes "Seen by
// partner" into Contoso's Runbooks once it reads Pager there.
//
// Unlocking derives the account's keys with Argon2id (64 MiB, 24 passes, 4 lanes), seconds of CPU on a CI runner, so a
// partner that unlocked on every pass kept the runner's CPUs busy under the simulator for the whole run.
package main

import (
	"context"
	"flag"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/session"
)

type partner struct {
	session      *session.Session
	joinEmail    string
	isInPlatform bool
	hasLeft      bool
	hasAnswered  bool
}

func main() {
	statePath := flag.String("state", "", "the partner's CLI state file (required)")
	password := flag.String("password", "", "the partner's master password (required)")
	secretKeyText := flag.String("secret-key", "", "the partner's Secret Key (required)")
	joinEmail := flag.String("join-email", "", "the account that joins Northwind (required)")
	flag.Parse()
	if *statePath == "" || *password == "" || *secretKeyText == "" || *joinEmail == "" {
		log.Fatal("-state, -password, -secret-key and -join-email are required")
	}
	state, err := localstate.Load(*statePath)
	if err != nil {
		log.Fatal(err)
	}
	secretKey, err := crypt.ParseSecretKey(*secretKeyText)
	if err != nil {
		log.Fatal(err)
	}
	client := apiclient.New(state.Server)
	client.Token = state.Token
	ctx := context.Background()
	unlocked, err := session.Unlock(ctx, client, nil, []byte(*password), secretKey)
	if err != nil {
		log.Fatalf("unlocking the partner: %v", err)
	}
	unlocked.SeqStore = state
	unlocked.MemberVerifier = state.CheckMemberPin
	log.Print("unlocked; answering as the partner")
	answerer := &partner{session: unlocked, joinEmail: *joinEmail}
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
	if northwind := findOrganization(organizations, "Northwind"); northwind != nil && !p.hasLeft {
		p.answerNorthwind(ctx, northwind)
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

// answerNorthwind adds the joining account to Platform once it joined Northwind, and removes it once it saved "Remove me"
// in Deploy; both vaults then get a new key on the next pass.
func (p *partner) answerNorthwind(ctx context.Context, northwind *apitypes.OrganizationSummary) {
	if !p.isInPlatform && northwind.MemberCount < 2 {
		return
	}
	if p.isInPlatform {
		deploy := p.findVault(ctx, "Deploy")
		if deploy == nil || !p.hasItem(ctx, deploy, func(entry item.Item) bool { return entry.Overview.Title == "Remove me" }) {
			return
		}
	}
	organization, err := p.session.Client.GetOrganization(ctx, northwind.ID)
	if err != nil {
		log.Printf("reading Northwind: %v", err)
		return
	}
	memberIndex := slices.IndexFunc(organization.Members, func(member apitypes.OrganizationMember) bool { return strings.EqualFold(member.Email, p.joinEmail) })
	teamIndex := slices.IndexFunc(organization.Teams, func(team apitypes.Team) bool { return team.Name == "Platform" })
	if memberIndex < 0 || teamIndex < 0 {
		return
	}
	accountID, teamID := organization.Members[memberIndex].AccountID, organization.Teams[teamIndex].ID
	if !p.isInPlatform {
		if err := p.session.Client.AddTeamMember(ctx, northwind.ID, teamID, accountID); err != nil {
			log.Printf("adding %s to Platform: %v", p.joinEmail, err)
			return
		}
		p.isInPlatform = true
		log.Printf("added %s to Platform", p.joinEmail)
	} else {
		if err := p.session.Client.RemoveTeamMember(ctx, northwind.ID, teamID, accountID); err != nil {
			log.Printf("removing %s from Platform: %v", p.joinEmail, err)
			return
		}
		p.hasLeft = true
		log.Printf("removed %s from Platform", p.joinEmail)
	}
	p.completeOrganizationKeys(ctx)
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
