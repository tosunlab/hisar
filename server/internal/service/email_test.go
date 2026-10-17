package service

import (
	"strings"
	"testing"
	"time"

	"github.com/tryhisar/hisar/server/internal/store"
)

func TestBuildInviteLink(t *testing.T) {
	for publicURL, expectedLink := range map[string]string{
		"https://api.tryhisar.com":      "https://api.tryhisar.com/invite/code",
		"https://api.tryhisar.com/":     "https://api.tryhisar.com/invite/code",
		"http://api.tryhisar.com":       "https://api.tryhisar.com/invite/code",
		"https://api.tryhisar.com/base": "https://api.tryhisar.com/invite/code",
		"http://127.0.0.1:18550":        "http://127.0.0.1:18550/invite/code",
		"http://localhost:8080":         "http://localhost:8080/invite/code",
	} {
		server := &Service{config: &Config{PublicURL: publicURL}}
		if link := server.buildInviteLink("code"); link != expectedLink {
			t.Errorf("%s: %s", publicURL, link)
		}
	}
}

func TestComposeEmails(t *testing.T) {
	inviter := &store.Account{Name: "Alice\r\nBcc: eve@example.com", Email: "alice@example.com"}
	expiresAt := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

	invite := composeInviteEmail(inviter, "bob@example.com", "https://api.tryhisar.com/invite/abc", "abc", expiresAt)
	if invite.To != "bob@example.com" || invite.Subject != "AliceBcc: eve@example.com invited you to Hisar" ||
		!strings.Contains(invite.Body, "AliceBcc: eve@example.com (alice@example.com) invited you") ||
		!strings.Contains(invite.Body, "https://api.tryhisar.com/invite/abc\n") || !strings.Contains(invite.Body, "\nabc\n") ||
		!strings.Contains(invite.Body, "18 October 2026") {
		t.Errorf("invite: %+v", invite)
	}

	shared := composeShareInvitationEmail(&store.Account{Name: "Alice", Email: "alice@example.com"}, "bob@example.com", "reader")
	if shared.Subject != "Alice shared a vault with you in Hisar" || !strings.Contains(shared.Body, "as a reader") ||
		!strings.Contains(shared.Body, "Settings → People") || !strings.Contains(shared.Body, "hisar invitations") || strings.Contains(shared.Body, "http") {
		t.Errorf("share invitation: %+v", shared)
	}

	signUp := composeShareSignUpEmail(&store.Account{Name: "", Email: "alice@example.com"}, "ghost@example.com", "owner", "https://api.tryhisar.com/invite/xyz", "xyz", expiresAt)
	if signUp.Subject != "alice@example.com shared a vault with you in Hisar" || !strings.Contains(signUp.Body, "as an owner") ||
		!strings.Contains(signUp.Body, "https://api.tryhisar.com/invite/xyz") || !strings.Contains(signUp.Body, "\nxyz\n") {
		t.Errorf("share sign-up: %+v", signUp)
	}
	accepted := composeShareAcceptedEmail(&store.Account{Name: "Alice", Email: "alice@example.com"}, &store.Account{Name: "Ada\nBcc: eve@example.com", Email: "ada@example.com"})
	if accepted.To != "alice@example.com" || accepted.Subject != "AdaBcc: eve@example.com accepted your invitation in Hisar" ||
		!strings.Contains(accepted.Body, "(ada@example.com) accepted your invitation") || !strings.Contains(accepted.Body, "unlock it") ||
		!strings.Contains(accepted.Body, "cannot say which vault") || strings.Contains(accepted.Body, "http") {
		t.Errorf("share accepted: %+v", accepted)
	}
	for _, body := range []string{invite.Body, shared.Body, signUp.Body, accepted.Body} {
		if strings.Contains(body, "<") || strings.Contains(body, "family") || strings.Contains(body, "Family") {
			t.Errorf("unexpected content: %s", body)
		}
	}
}

func TestComposeOrganizationInvitationEmails(t *testing.T) {
	inviter := &store.Account{Name: "Alice", Email: "alice@example.com"}
	expiresAt := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

	invitation := composeOrganizationInvitationEmail(inviter, "bob@example.com", "Acme\r\nBcc: eve@example.com", "admin")
	if invitation.To != "bob@example.com" || invitation.Subject != "Alice invited you to AcmeBcc: eve@example.com in Hisar" ||
		!strings.Contains(invitation.Body, "Alice (alice@example.com) invited you to join the organization “AcmeBcc: eve@example.com” in Hisar as an admin") ||
		!strings.Contains(invitation.Body, "`hisar orgs`") || !strings.Contains(invitation.Body, "`hisar org accept`") || strings.Contains(invitation.Body, "http") {
		t.Errorf("organization invitation: %+v", invitation)
	}

	signUp := composeOrganizationSignUpEmail(inviter, "ghost@example.com", "Acme", "member", "https://api.tryhisar.com/invite/xyz", "xyz", expiresAt)
	if signUp.To != "ghost@example.com" || signUp.Subject != "Alice invited you to Acme in Hisar" || !strings.Contains(signUp.Body, "“Acme” in Hisar as a member") ||
		!strings.Contains(signUp.Body, "https://api.tryhisar.com/invite/xyz\n") || !strings.Contains(signUp.Body, "\nxyz\n") ||
		!strings.Contains(signUp.Body, "`hisar org accept`") || !strings.Contains(signUp.Body, "18 October 2026") {
		t.Errorf("organization sign-up: %+v", signUp)
	}
}
