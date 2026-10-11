package api

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/tryhisar/hisar/server/internal/mail"
	"github.com/tryhisar/hisar/server/internal/store"
)

const emailTimeout = time.Minute

func (s *Server) isEmailConfigured() bool { return s.config.Mailer != nil }

func (s *Server) buildInviteLink(token string) string {
	return s.buildPublicOrigin() + "/invite/" + token
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

func formatExpiry(expiresAt time.Time) string { return expiresAt.UTC().Format("2 January 2006") }

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
`, describeInviter(inviter), link, code, formatExpiry(expiresAt)),
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
`, describeInviter(inviter), describeRole(role), link, code, formatExpiry(expiresAt)),
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

func (s *Server) sendEmail(ctx context.Context, message mail.Message) {
	if err := s.config.Mailer.Send(ctx, message); err != nil {
		log.Printf("email not sent: %v", err)
	}
}

// runInBackground does account-dependent work after the response, so the response cannot reveal whether an account exists.
func (s *Server) runInBackground(work func(ctx context.Context)) {
	s.backgroundWork.Add(1)
	go func() {
		defer s.backgroundWork.Done()
		ctx, cancel := context.WithTimeout(context.Background(), emailTimeout)
		defer cancel()
		work(ctx)
	}()
}

// WaitForBackgroundWork waits for invites and share notifications that were started before it was called.
func (s *Server) WaitForBackgroundWork() { s.backgroundWork.Wait() }
