// Command hisar-ui-test-partner plays the partner of the iPhone and Android UI tests for scripts/ui-test-partner.sh. It
// signs up from an invite and sets up what the tests expect: the vault Infrastructure with one login, shared with the
// banner account before it exists, and the organization Northwind, whose team Owners (the partner alone) owns the vaults
// Deploy and Audit, given to the team Platform as editor and reader, with the joining account invited. It then writes the
// ready file and answers every 2 seconds, unlocked as the partner's own Hisar would be: it accepts every share invitation
// the partner gets, confirms and completes every share the partner sent, accepts every organization invitation, and gives
// team members the keys of the partner's vaults and those vaults a new key once someone lost access. For the organization
// tests it adds the joining account to Northwind's team Platform once it joined Northwind, removes it again once it saved
// "Remove me" in Deploy, and writes "Seen by partner" into Contoso's Runbooks once it reads Pager there.
//
// The account's keys are derived once, at sign-up, with Argon2id (64 MiB, 24 passes, 4 lanes), about a second of CPU. A
// partner on the command line derived them on every pass, every 5 to 6 seconds, on the runner the simulator shares.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/invitelink"
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
	invite := flag.String("invite", "", "the invite link the partner signs up with (required)")
	email := flag.String("email", "", "the partner's email (required)")
	name := flag.String("name", "", "the partner's name (required)")
	password := flag.String("password", "", "the partner's master password (required)")
	shareEmail := flag.String("share-email", "", "the account Infrastructure is shared with (required)")
	joinEmail := flag.String("join-email", "", "the account invited to Northwind (required)")
	readyPath := flag.String("ready-file", "", "the file written once the partner is set up (required)")
	flag.Parse()
	if *invite == "" || *email == "" || *name == "" || *password == "" || *shareEmail == "" || *joinEmail == "" || *readyPath == "" {
		log.Fatal("-invite, -email, -name, -password, -share-email, -join-email and -ready-file are required")
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
	if err := setUp(ctx, signedUp, *shareEmail, *joinEmail); err != nil {
		log.Fatalf("setting up the partner: %v", err)
	}
	if err := os.WriteFile(*readyPath, nil, 0o600); err != nil {
		log.Fatal(err)
	}
	log.Print("set up; answering as the partner")
	answerer := &partner{session: signedUp, joinEmail: *joinEmail}
	for {
		answerer.answer(ctx)
		time.Sleep(2 * time.Second)
	}
}

func setUp(ctx context.Context, s *session.Session, shareEmail, joinEmail string) error {
	infrastructure, err := s.CreateVault(ctx, "Infrastructure", apitypes.VaultShared)
	if err != nil {
		return fmt.Errorf("creating Infrastructure: %w", err)
	}
	if err := saveLogin(ctx, s, infrastructure, "Staging database", "deploy", "https://staging.example.com", "staging-database-password"); err != nil {
		return err
	}
	if err := s.InviteToVault(ctx, infrastructure, shareEmail, apitypes.VaultEditor); err != nil {
		return fmt.Errorf("sharing Infrastructure with %s: %w", shareEmail, err)
	}
	northwind, err := s.Client.CreateOrganization(ctx, &apitypes.CreateOrganizationRequest{Name: "Northwind"})
	if err != nil {
		return fmt.Errorf("creating Northwind: %w", err)
	}
	owners, err := s.Client.CreateTeam(ctx, northwind.ID, &apitypes.CreateTeamRequest{Name: "Owners"})
	if err != nil {
		return fmt.Errorf("creating the team Owners: %w", err)
	}
	platform, err := s.Client.CreateTeam(ctx, northwind.ID, &apitypes.CreateTeamRequest{Name: "Platform"})
	if err != nil {
		return fmt.Errorf("creating the team Platform: %w", err)
	}
	if err := s.Client.AddTeamMember(ctx, northwind.ID, owners.ID, s.AccountID); err != nil {
		return fmt.Errorf("joining the team Owners: %w", err)
	}
	for _, organizationVault := range []struct{ name, title, username, url, password, role string }{
		{"Deploy", "Release signing", "release", "https://deploy.example.com", "release-signing-password", apitypes.VaultEditor},
		{"Audit", "Audit log", "auditor", "https://audit.example.com", "audit-log-password", apitypes.VaultReader},
	} {
		vault, err := s.CreateOrganizationVault(ctx, organizationVault.name, northwind.ID, owners.ID)
		if err != nil {
			return fmt.Errorf("creating %s: %w", organizationVault.name, err)
		}
		if err := saveLogin(ctx, s, vault, organizationVault.title, organizationVault.username, organizationVault.url, organizationVault.password); err != nil {
			return err
		}
		if err := s.Client.GiveVaultToTeam(ctx, northwind.ID, platform.ID, vault.ID, &apitypes.GrantTeamVaultRequest{Role: organizationVault.role}); err != nil {
			return fmt.Errorf("giving %s to Platform: %w", organizationVault.name, err)
		}
	}
	if _, err := s.Client.InviteToOrganization(ctx, northwind.ID, &apitypes.OrganizationInvitationRequest{Email: joinEmail, Role: apitypes.OrganizationRoleMember}); err != nil {
		return fmt.Errorf("inviting %s to Northwind: %w", joinEmail, err)
	}
	return nil
}

func saveLogin(ctx context.Context, s *session.Session, vault *session.Vault, title, username, url, password string) error {
	login := &item.Item{
		Overview: item.Overview{Category: item.CategoryLogin, Title: title, Username: username, URLs: []string{url}},
		Details:  item.Details{Password: password},
	}
	if _, err := s.SaveItem(ctx, vault, login); err != nil {
		return fmt.Errorf("saving %q to %s: %w", title, vault.Name, err)
	}
	return nil
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
