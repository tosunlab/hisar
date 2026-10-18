package service

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/server/internal/mail"
	"github.com/tryhisar/hisar/server/internal/settings"
	"github.com/tryhisar/hisar/server/internal/store"
)

const emailTimeout = time.Minute

// mailer sends with the current SMTP settings; it is nil when email is off.
func (s *Service) mailer() mail.Mailer { return s.NewMailer(s.currentSettings().SMTP) }

func (s *Service) isEmailConfigured() bool { return s.mailer() != nil }

func newSMTPMailer(smtp settings.SMTP) mail.Mailer {
	if !smtp.IsSet() {
		return nil
	}
	mailer, err := mail.NewSMTPMailer(mail.SMTPConfig{Host: smtp.Host, Port: smtp.Port, Username: smtp.Username, Password: smtp.Password, From: smtp.From})
	if err != nil {
		log.Printf("email: %v", err)
		return nil
	}
	return mailer
}

func (s *Service) buildInviteLink(token string) string {
	return BuildPublicOrigin(s.config.PublicURL) + "/invite/" + token
}

func describeInviter(inviter *store.Account) string {
	name := strings.TrimSpace(mail.RemoveControlCharacter(inviter.Name))
	if name == "" || strings.EqualFold(name, inviter.Email) {
		return inviter.Email
	}
	return fmt.Sprintf("%s (%s)", name, inviter.Email)
}

func getInviterName(inviter *store.Account) string {
	if name := strings.TrimSpace(mail.RemoveControlCharacter(inviter.Name)); name != "" {
		return name
	}
	return inviter.Email
}

func describeRole(role string) string {
	switch role {
	case "owner":
		return "an owner"
	case "editor":
		return "an editor"
	}
	return "a reader"
}

// FormatExpiry is how an invite's expiry reads in emails and on the invite page.
func FormatExpiry(expiresAt time.Time) string { return expiresAt.UTC().Format("2 January 2006") }

func composeInviteEmail(inviter *store.Account, recipient, link, code string, expiresAt time.Time) mail.Message {
	return mail.Message{
		To:      recipient,
		Subject: getInviterName(inviter) + " invited you to Hisar",
		Body: fmt.Sprintf(`%s invited you to Hisar, the end-to-end encrypted password, SSH key and secrets manager for developers.

Get Hisar and create your account:
%s

Or paste this invite code under Create account in Hisar:
%s

The invite works once and expires on %s.
If you were not expecting it, ignore this email.
`, describeInviter(inviter), link, code, FormatExpiry(expiresAt)),
	}
}

func composeShareInvitationEmail(inviter *store.Account, recipient, role string) mail.Message {
	return mail.Message{
		To:      recipient,
		Subject: getInviterName(inviter) + " shared a vault with you in Hisar",
		Body: fmt.Sprintf(`%s shared a vault with you in Hisar as %s.
Vault names are end-to-end encrypted, so this email cannot say which vault.

Accept or decline it in Hisar under Settings → People, or with `+"`hisar invitations`"+`.
You get access once you accept and %s's app finishes sharing.

If you were not expecting it, decline it or ignore this email.
`, describeInviter(inviter), describeRole(role), getInviterName(inviter)),
	}
}

func composeShareSignUpEmail(inviter *store.Account, recipient, role, link, code string, expiresAt time.Time) mail.Message {
	return mail.Message{
		To:      recipient,
		Subject: getInviterName(inviter) + " shared a vault with you in Hisar",
		Body: fmt.Sprintf(`%s shared a vault with you in Hisar as %s.
Hisar is the end-to-end encrypted password, SSH key and secrets manager for developers. Vault names are encrypted, so this email cannot say which vault.

Get Hisar and create your account to accept it:
%s

Or paste this invite code under Create account in Hisar:
%s

Then accept the invitation under Settings → People. The invite expires on %s.
If you were not expecting it, ignore this email.
`, describeInviter(inviter), describeRole(role), link, code, FormatExpiry(expiresAt)),
	}
}

func composeShareAcceptedEmail(inviter, recipient *store.Account) mail.Message {
	return mail.Message{
		To:      inviter.Email,
		Subject: getInviterName(recipient) + " accepted your invitation in Hisar",
		Body: fmt.Sprintf(`%s accepted your invitation to share a vault in Hisar.
Vault names are end-to-end encrypted, so this email cannot say which vault.

Open Hisar and unlock it on any of your devices to finish sharing: your app checks their keys
and gives them access. Until then they are waiting and cannot see anything in the vault.

If you no longer want to share it, cancel the invitation under Settings → People.
`, describeInviter(recipient)),
	}
}

func describeOrganizationRole(role string) string {
	if role == apitypes.OrganizationRoleAdmin {
		return "an admin"
	}
	return "a member"
}

// Organization names are not encrypted (§2.3), so the organization invitation emails name the organization.
func composeOrganizationInvitationEmail(inviter *store.Account, recipient, organizationName, role string) mail.Message {
	organizationName = mail.RemoveControlCharacter(organizationName)
	return mail.Message{
		To:      recipient,
		Subject: getInviterName(inviter) + " invited you to " + organizationName + " in Hisar",
		Body: fmt.Sprintf(`%s invited you to join the organization “%s” in Hisar as %s.
Its members share vaults through teams: once an admin adds you to a team, you get the team's vaults.

See the invitation with `+"`hisar orgs`"+`, then accept it with `+"`hisar org accept`"+` or decline it
with `+"`hisar org decline`"+`.

If you were not expecting it, decline it or ignore this email.
`, describeInviter(inviter), organizationName, describeOrganizationRole(role)),
	}
}

func composeOrganizationSignUpEmail(inviter *store.Account, recipient, organizationName, role, link, code string, expiresAt time.Time) mail.Message {
	organizationName = mail.RemoveControlCharacter(organizationName)
	return mail.Message{
		To:      recipient,
		Subject: getInviterName(inviter) + " invited you to " + organizationName + " in Hisar",
		Body: fmt.Sprintf(`%s invited you to join the organization “%s” in Hisar as %s.
Hisar is the end-to-end encrypted password, SSH key and secrets manager for developers; its organizations share vaults through teams.

Get Hisar and create your account to accept it:
%s

Or paste this invite code under Create account in Hisar:
%s

Then accept the invitation with `+"`hisar org accept`"+` (`+"`hisar orgs`"+` lists it). The invite expires on %s.
If you were not expecting it, ignore this email.
`, describeInviter(inviter), organizationName, describeOrganizationRole(role), link, code, FormatExpiry(expiresAt)),
	}
}

// sendEmail sends with the settings of the moment, so a message queued before email was turned off is dropped.
func (s *Service) sendEmail(ctx context.Context, message mail.Message) {
	mailer := s.mailer()
	if mailer == nil {
		return
	}
	if err := mailer.Send(ctx, message); err != nil {
		log.Printf("email not sent: %v", err)
	}
}

// runInBackground does account-dependent work after the response, so the response cannot reveal whether an account exists.
func (s *Service) runInBackground(work func(ctx context.Context)) {
	s.backgroundWork.Add(1)
	go func() {
		defer s.backgroundWork.Done()
		ctx, cancel := context.WithTimeout(context.Background(), emailTimeout)
		defer cancel()
		work(ctx)
	}()
}

// WaitForBackgroundWork waits for invites and share notifications that were started before it was called.
func (s *Service) WaitForBackgroundWork() { s.backgroundWork.Wait() }

// BuildPublicOrigin is the origin of links to this server: https, except http to a loopback host for development.
func BuildPublicOrigin(publicURL string) string {
	parsedURL, err := url.Parse(publicURL)
	if err != nil || parsedURL.Host == "" {
		return strings.TrimRight(publicURL, "/")
	}
	scheme := "https"
	if parsedURL.Scheme == "http" && mail.IsLoopbackHost(parsedURL.Hostname()) {
		scheme = "http"
	}
	return scheme + "://" + parsedURL.Host
}
