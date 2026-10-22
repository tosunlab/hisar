package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/server/internal/mail"
	"github.com/tryhisar/hisar/server/internal/ratelimit"
)

func sendRawRequest(t *testing.T, baseURL, token, method, path string, body any) (int, string) {
	t.Helper()
	encoded, _ := json.Marshal(body)
	request, err := http.NewRequest(method, baseURL+path, bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(responseBody)
}

func findEmail(messages []mail.Message, recipient string) (mail.Message, bool) {
	for _, message := range messages {
		if message.To == recipient {
			return message, true
		}
	}
	return mail.Message{}, false
}

func TestInviteByEmail(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleAdmin)
	signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", apitypes.RoleMember)

	if _, err := alice.Client.Invite(ctx, &apitypes.InviteRequest{Email: "carol@example.com"}); !apiclient.IsStatus(err, http.StatusNotImplemented) || !strings.Contains(err.Error(), "email is not configured on this server") {
		t.Fatalf("invite without email: %v", err)
	}

	recorder := attachMailRecorder(server)
	newStatus, newBody := sendRawRequest(t, baseURL, alice.Client.Token, "POST", "/api/v1/invites", apitypes.InviteRequest{Email: " Carol@Example.com "})
	existingStatus, existingBody := sendRawRequest(t, baseURL, alice.Client.Token, "POST", "/api/v1/invites", apitypes.InviteRequest{Email: "bob@example.com"})
	if newStatus != http.StatusAccepted || existingStatus != newStatus || existingBody != newBody || !strings.Contains(newBody, apitypes.InvitationSentMessage) {
		t.Fatalf("responses differ: %d %s / %d %s", newStatus, newBody, existingStatus, existingBody)
	}
	if strings.Contains(newBody, "invite/") || strings.Contains(newBody, "token") {
		t.Errorf("the response carries the invite: %s", newBody)
	}
	server.WaitForBackgroundWork()
	messages := recorder.ListMessages()
	if len(messages) != 1 {
		t.Fatalf("emails: %+v", messages)
	}
	invite, found := findEmail(messages, "carol@example.com")
	token := findInviteToken(t, server, recorder, "carol@example.com")
	if !found || invite.Subject != "alice@example.com invited you to Hisar" ||
		!strings.Contains(invite.Body, baseURL+"/invite/"+token) || !strings.Contains(invite.Body, "\n"+token+"\n") || !strings.Contains(invite.Body, "alice@example.com") {
		t.Errorf("invite email: %+v", invite)
	}
	carol, _, err := session.SignUp(ctx, apiclient.New(baseURL), token, "carol@example.com", "Carol", []byte("carol password"), testDevice)
	if err != nil {
		t.Fatalf("sign up with the emailed code: %v", err)
	}
	if carol.Role != "" && carol.Role != apitypes.RoleMember {
		t.Errorf("Carol's role: %q", carol.Role)
	}
	if account := requireValue[*apitypes.Account](t)(carol.Client.GetAccount(ctx)); account.Role != apitypes.RoleMember {
		t.Errorf("Carol's role: %q", account.Role)
	}

	for _, badEmail := range []string{"", "not an email", "Bob <bob@example.com>", "a@b.c\r\nBcc: eve@example.com"} {
		if _, err := alice.Client.Invite(ctx, &apitypes.InviteRequest{Email: badEmail}); !apiclient.IsStatus(err, http.StatusBadRequest) {
			t.Errorf("invite %q: %v", badEmail, err)
		}
	}

	server.service.Limiters.InviteAccount = ratelimit.NewDaily(2)
	for index := range 2 {
		if _, err := carol.Client.Invite(ctx, &apitypes.InviteRequest{Email: "dave@example.com"}); err != nil {
			t.Fatalf("invite %d: %v", index, err)
		}
	}
	if _, err := carol.Client.Invite(ctx, &apitypes.InviteRequest{Email: "dave@example.com"}); !apiclient.IsStatus(err, http.StatusTooManyRequests) {
		t.Errorf("third invite of the day: %v", err)
	}
	if _, err := alice.Client.Invite(ctx, &apitypes.InviteRequest{Email: "dave@example.com"}); err != nil {
		t.Errorf("another account's invite was limited: %v", err)
	}
	server.service.Limiters.InviteIP = ratelimit.NewDaily(1)
	if _, err := alice.Client.Invite(ctx, &apitypes.InviteRequest{Email: "erin@example.com"}); err != nil {
		t.Fatal(err)
	}
	if _, err := alice.Client.Invite(ctx, &apitypes.InviteRequest{Email: "erin@example.com"}); !apiclient.IsStatus(err, http.StatusTooManyRequests) {
		t.Errorf("invite over the IP limit: %v", err)
	}
	server.WaitForBackgroundWork()
}

func TestShareInvitationWithoutEnumeration(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	recorder := attachMailRecorder(server)
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleAdmin)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", apitypes.RoleMember)
	carol := signUpTestAccount(t, ctx, server, baseURL, "carol@example.com", apitypes.RoleMember)
	family := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Family", apitypes.VaultShared))
	requireValue[*item.Item](t)(alice.SaveItem(ctx, family, &item.Item{Overview: item.Overview{Title: "Netflix"}, Details: item.Details{Password: "hunter2"}}))
	work := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Work", apitypes.VaultShared))
	shareThroughInvitation(t, alice, carol, work, apitypes.VaultReader)
	server.WaitForBackgroundWork()
	emailCountBefore := len(recorder.ListMessages())

	invitationPath := "/api/v1/vaults/" + family.ID + "/share-invitations"
	existingStatus, existingBody := sendRawRequest(t, baseURL, alice.Client.Token, "POST", invitationPath, signedShareRequest(t, alice, family, "Bob@Example.com", apitypes.VaultEditor, nil))
	unknownStatus, unknownBody := sendRawRequest(t, baseURL, alice.Client.Token, "POST", invitationPath, signedShareRequest(t, alice, family, "ghost@example.com", apitypes.VaultEditor, nil))
	if existingStatus != http.StatusAccepted || unknownStatus != existingStatus || unknownBody != existingBody || !strings.Contains(existingBody, apitypes.InvitationSentMessage) {
		t.Fatalf("responses differ: %d %s / %d %s", existingStatus, existingBody, unknownStatus, unknownBody)
	}
	messages := recorder.WaitForMessageCount(emailCountBefore+2, 5*time.Second)[emailCountBefore:]
	toBob, isToBob := findEmail(messages, "bob@example.com")
	if !isToBob || toBob.Subject != "alice@example.com shared a vault with you in Hisar" || !strings.Contains(toBob.Body, "as an editor") ||
		strings.Contains(toBob.Body, "Family") || strings.Contains(toBob.Body, "/invite/") {
		t.Errorf("email to Bob: %+v", toBob)
	}
	toGhost, isToGhost := findEmail(messages, "ghost@example.com")
	if !isToGhost || !strings.Contains(toGhost.Body, baseURL+"/invite/") || strings.Contains(toGhost.Body, "Family") {
		t.Errorf("email to the unknown address: %+v", toGhost)
	}

	invitationList := requireValue[*apitypes.ShareInvitationList](t)(bob.Client.ListShareInvitations(ctx))
	if len(invitationList.Received) != 1 || len(invitationList.Sent) != 0 {
		t.Fatalf("Bob's invitations: %+v", invitationList)
	}
	received := invitationList.Received[0]
	if received.VaultID != family.ID || received.Role != apitypes.VaultEditor || received.InvitedByEmail != "alice@example.com" {
		t.Errorf("received: %+v", received)
	}
	if carolList := requireValue[*apitypes.ShareInvitationList](t)(carol.Client.ListShareInvitations(ctx)); len(carolList.Received) != 0 || len(carolList.Sent) != 0 {
		t.Errorf("Carol sees invitations: %+v", carolList)
	}
	aliceList := requireValue[*apitypes.ShareInvitationList](t)(alice.Client.ListShareInvitations(ctx))
	if len(aliceList.Sent) != 2 {
		t.Fatalf("Alice's sent invitations: %+v", aliceList)
	}
	for _, sent := range aliceList.Sent {
		if sent.State != apitypes.ShareInvitationPending || sent.Recipient != nil {
			t.Errorf("a pending invitation reveals its recipient: %+v", sent)
		}
	}

	bobKeys := requireValue[*apitypes.Account](t)(bob.Client.GetAccount(ctx)).PublicKeys
	wrappedKey := requireValue[[]byte](t)(crypt.WrapVaultKey(bobKeys.KEM, family.ID, bob.AccountID, family.Key))
	if err := alice.Client.SetAccess(ctx, family.ID, bob.AccountID, &apitypes.SetAccessRequest{WrappedKey: wrappedKey, Role: apitypes.VaultEditor}); !apiclient.IsStatus(err, http.StatusConflict) {
		t.Errorf("access before Bob accepted: %v", err)
	}
	if err := carol.Client.AcceptShareInvitation(ctx, received.ID); !apiclient.IsStatus(err, http.StatusNotFound) {
		t.Errorf("Carol accepted Bob's invitation: %v", err)
	}
	if err := bob.Client.AcceptShareInvitation(ctx, received.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := bob.FindVault(ctx, "Family"); !errors.Is(err, session.ErrNotFound) {
		t.Errorf("Bob has access before Alice completed: %v", err)
	}
	aliceList = requireValue[*apitypes.ShareInvitationList](t)(alice.Client.ListShareInvitations(ctx))
	acceptedIndex := slices.IndexFunc(aliceList.Sent, func(sent apitypes.SentShareInvitation) bool { return sent.State == apitypes.ShareInvitationAccepted })
	if acceptedIndex < 0 || aliceList.Sent[acceptedIndex].Recipient == nil || aliceList.Sent[acceptedIndex].Recipient.AccountID != bob.AccountID ||
		aliceList.Sent[acceptedIndex].Recipient.PublicKeys.Verify(bob.AccountID) != nil {
		t.Fatalf("Alice's accepted invitation: %+v", aliceList.Sent)
	}
	if carolList := requireValue[*apitypes.ShareInvitationList](t)(carol.Client.ListShareInvitations(ctx)); len(carolList.Sent) != 0 {
		t.Errorf("a member who does not own the vault sees its invitations: %+v", carolList.Sent)
	}
	completedShares := completeReviewedShares(t, alice)
	if len(completedShares) != 1 || completedShares[0].VaultName != "Family" || completedShares[0].Recipient.Email != "bob@example.com" {
		t.Fatalf("completed: %+v", completedShares)
	}
	bobFamily := requireValue[*session.Vault](t)(bob.FindVault(ctx, "Family"))
	if items := requireValue[[]item.Item](t)(bob.ListItems(ctx, bobFamily)); len(items) != 1 || items[0].Details.Password != "hunter2" {
		t.Errorf("Bob's items: %+v", items)
	}
	if completedShares := requireValue[[]session.CompletedShare](t)(alice.CompleteAcceptedShares(ctx)); len(completedShares) != 0 {
		t.Errorf("completed twice: %+v", completedShares)
	}

	t.Run("people are only those who share a vault", func(t *testing.T) {
		listEmails := func(currentSession *session.Session) []string {
			var emails []string
			for _, member := range requireValue[[]apitypes.Member](t)(currentSession.Client.ListMembers(ctx)) {
				emails = append(emails, member.Email)
			}
			slices.Sort(emails)
			return emails
		}
		if emails := listEmails(bob); !slices.Equal(emails, []string{"alice@example.com", "bob@example.com"}) {
			t.Errorf("Bob sees %v", emails)
		}
		if emails := listEmails(alice); !slices.Equal(emails, []string{"alice@example.com", "bob@example.com", "carol@example.com"}) {
			t.Errorf("Alice sees %v", emails)
		}
	})

	t.Run("refusals that depend only on the owner's data", func(t *testing.T) {
		if err := alice.InviteToVault(ctx, family, "alice@example.com", apitypes.VaultEditor); !apiclient.IsStatus(err, http.StatusBadRequest) {
			t.Errorf("share with yourself: %v", err)
		}
		if err := alice.InviteToVault(ctx, family, "bob@example.com", apitypes.VaultReader); !apiclient.IsStatus(err, http.StatusConflict) {
			t.Errorf("share with a member: %v", err)
		}
		bobRequest := signedShareRequest(t, bob, bobFamily, "carol@example.com", apitypes.VaultReader, nil)
		if err := bob.Client.CreateSignedShareInvitation(ctx, family.ID, &bobRequest); !apiclient.IsStatus(err, http.StatusForbidden) {
			t.Errorf("an editor shared: %v", err)
		}
		alicePersonal := requireValue[*session.Vault](t)(alice.FindVault(ctx, "Personal"))
		personalRequest := signedShareRequest(t, alice, alicePersonal, "carol@example.com", apitypes.VaultReader, nil)
		if err := alice.Client.CreateSignedShareInvitation(ctx, alicePersonal.ID, &personalRequest); !apiclient.IsStatus(err, http.StatusBadRequest) {
			t.Errorf("shared a personal vault: %v", err)
		}
		if err := alice.InviteToVault(ctx, family, "carol@example.com", "admin"); !apiclient.IsStatus(err, http.StatusBadRequest) {
			t.Errorf("invalid role: %v", err)
		}
		if err := alice.InviteToVault(ctx, family, "not an email", apitypes.VaultReader); !apiclient.IsStatus(err, http.StatusBadRequest) {
			t.Errorf("invalid email: %v", err)
		}
		if _, err := carol.Client.CreateShareInvitation(ctx, family.ID, &apitypes.ShareInvitationRequest{Email: "dave@example.com", Role: apitypes.VaultReader}); !apiclient.IsStatus(err, http.StatusNotFound) {
			t.Errorf("a non-member shared: %v", err)
		}
	})

	t.Run("decline and cancel", func(t *testing.T) {
		if err := alice.InviteToVault(ctx, family, "carol@example.com", apitypes.VaultReader); err != nil {
			t.Fatal(err)
		}
		carolList := requireValue[*apitypes.ShareInvitationList](t)(carol.Client.ListShareInvitations(ctx))
		if len(carolList.Received) != 1 {
			t.Fatalf("Carol's invitations: %+v", carolList)
		}
		if err := carol.Client.DeclineShareInvitation(ctx, carolList.Received[0].ID); err != nil {
			t.Fatal(err)
		}
		if err := carol.Client.AcceptShareInvitation(ctx, carolList.Received[0].ID); !apiclient.IsStatus(err, http.StatusNotFound) {
			t.Errorf("accept after declining: %v", err)
		}
		aliceList := requireValue[*apitypes.ShareInvitationList](t)(alice.Client.ListShareInvitations(ctx))
		ghostIndex := slices.IndexFunc(aliceList.Sent, func(sent apitypes.SentShareInvitation) bool { return sent.Email == "ghost@example.com" })
		if len(aliceList.Sent) != 1 || ghostIndex != 0 {
			t.Fatalf("Alice's sent invitations: %+v", aliceList.Sent)
		}
		if err := bob.Client.CancelShareInvitation(ctx, aliceList.Sent[0].ID); !apiclient.IsStatus(err, http.StatusNotFound) {
			t.Errorf("an editor cancelled: %v", err)
		}
		if err := alice.Client.CancelShareInvitation(ctx, aliceList.Sent[0].ID); err != nil {
			t.Fatal(err)
		}
		if err := alice.Client.CancelShareInvitation(ctx, "not-a-uuid"); !apiclient.IsStatus(err, http.StatusNotFound) {
			t.Errorf("cancel an invalid ID: %v", err)
		}
	})

	t.Run("a rotation cannot add someone", func(t *testing.T) {
		carolKeys := requireValue[*apitypes.Account](t)(carol.Client.GetAccount(ctx)).PublicKeys
		request := &apitypes.RotateVaultRequest{EncName: []byte{1}}
		for _, accountID := range []string{alice.AccountID, bob.AccountID} {
			request.Accesses = append(request.Accesses, apitypes.RotatedAccess{AccountID: accountID, Role: apitypes.VaultOwner, WrappedKey: []byte{1}})
		}
		request.Accesses = append(request.Accesses, apitypes.RotatedAccess{AccountID: carol.AccountID, Role: apitypes.VaultReader, WrappedKey: carolKeys.KEM[:32]})
		changes := requireValue[*apitypes.ChangesResponse](t)(alice.Client.ListChanges(ctx, family.ID, 0, 0))
		for _, protoItem := range changes.Items {
			request.Items = append(request.Items, apitypes.RotatedItem{ID: protoItem.ID, BaseRevision: protoItem.Revision, Revision: "01890000-0000-7000-8000-000000000001", Overview: []byte{1}, Details: []byte{1}})
		}
		if err := alice.Client.RotateVault(ctx, family.ID, request); !apiclient.IsStatus(err, http.StatusBadRequest) {
			t.Errorf("rotation adding Carol: %v", err)
		}
	})

	t.Run("an account created from the share email sees the invitation", func(t *testing.T) {
		if err := alice.InviteToVault(ctx, family, "ghost@example.com", apitypes.VaultReader); err != nil {
			t.Fatal(err)
		}
		token := findInviteToken(t, server, recorder, "ghost@example.com")
		ghost, _, err := session.SignUp(ctx, apiclient.New(baseURL), token, "ghost@example.com", "Ghost", []byte("ghost password"), testDevice)
		if err != nil {
			t.Fatal(err)
		}
		shareList := requireValue[*apitypes.ShareInvitationList](t)(ghost.Client.ListShareInvitations(ctx))
		if len(shareList.Received) != 1 || shareList.Received[0].VaultID != family.ID {
			t.Fatalf("Ghost's invitations: %+v", shareList)
		}
		if err := ghost.Client.AcceptShareInvitation(ctx, shareList.Received[0].ID); err != nil {
			t.Fatal(err)
		}
		if completedShares := completeReviewedShares(t, alice); len(completedShares) != 1 || completedShares[0].Role != apitypes.VaultReader {
			t.Fatalf("completed: %+v", completedShares)
		}
		requireValue[*session.Vault](t)(ghost.FindVault(ctx, "Family"))
	})

	t.Run("share invitations are rate limited", func(t *testing.T) {
		server.service.Limiters.ShareInvitationAccount = ratelimit.NewDaily(1)
		if err := alice.InviteToVault(ctx, work, "frank@example.com", apitypes.VaultReader); err != nil {
			t.Fatal(err)
		}
		if err := alice.InviteToVault(ctx, work, "grace@example.com", apitypes.VaultReader); !apiclient.IsStatus(err, http.StatusTooManyRequests) {
			t.Errorf("over the limit: %v", err)
		}
	})
	server.WaitForBackgroundWork()
}

func TestCompleteAcceptedShareRefusesChangedKey(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleAdmin)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", apitypes.RoleMember)
	family := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Family", apitypes.VaultShared))
	pinnedKeys := map[string][]byte{}
	alice.MemberVerifier = func(member *apitypes.Member) error {
		if pinned, found := pinnedKeys[member.AccountID]; found && !bytes.Equal(pinned, member.PublicKeys.Sign) {
			return errors.New("signing key changed")
		}
		pinnedKeys[member.AccountID] = member.PublicKeys.Sign
		return nil
	}
	pinnedKeys[bob.AccountID] = []byte("a different key")
	if err := alice.InviteToVault(ctx, family, "bob@example.com", apitypes.VaultEditor); err != nil {
		t.Fatal(err)
	}
	invitationList := requireValue[*apitypes.ShareInvitationList](t)(bob.Client.ListShareInvitations(ctx))
	if err := bob.Client.AcceptShareInvitation(ctx, invitationList.Received[0].ID); err != nil {
		t.Fatal(err)
	}
	sent := requireValue[*apitypes.ShareInvitationList](t)(alice.Client.ListShareInvitations(ctx)).Sent[0]
	if err := alice.ConfirmShareRecipient(ctx, sent.ID, session.ShareInvitationReviewHash(sent)); err == nil || !strings.Contains(err.Error(), "signing key changed") {
		t.Errorf("confirmed with a changed key: %v", err)
	}
	if _, err := bob.FindVault(ctx, "Family"); !errors.Is(err, session.ErrNotFound) {
		t.Errorf("Bob got access: %v", err)
	}
	server.WaitForBackgroundWork()
}

func TestAcceptedShareTellsInviterAndRecipient(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	recorder := attachMailRecorder(server)
	server.service.ShareAcceptedEmailDelay = 200 * time.Millisecond
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleAdmin)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", apitypes.RoleMember)
	carol := signUpTestAccount(t, ctx, server, baseURL, "carol@example.com", apitypes.RoleMember)
	work := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Work", apitypes.VaultShared))
	server.WaitForBackgroundWork()

	acceptInvitation := func(recipient *session.Session) (emailCountBefore int) {
		t.Helper()
		if err := alice.InviteToVault(ctx, work, recipient.Email, apitypes.VaultEditor); err != nil {
			t.Fatal(err)
		}
		server.WaitForBackgroundWork()
		invitationList := requireValue[*apitypes.ShareInvitationList](t)(recipient.Client.ListShareInvitations(ctx))
		if len(invitationList.Received) != 1 || len(invitationList.Waiting) != 0 {
			t.Fatalf("%s before accepting: %+v", recipient.Email, invitationList)
		}
		emailCountBefore = len(recorder.ListMessages())
		if err := recipient.Client.AcceptShareInvitation(ctx, invitationList.Received[0].ID); err != nil {
			t.Fatal(err)
		}
		return emailCountBefore
	}

	emailCountBefore := acceptInvitation(bob)
	bobList := requireValue[*apitypes.ShareInvitationList](t)(bob.Client.ListShareInvitations(ctx))
	if len(bobList.Received) != 0 || len(bobList.Waiting) != 1 {
		t.Fatalf("Bob after accepting: %+v", bobList)
	}
	waiting := bobList.Waiting[0]
	if waiting.VaultID != work.ID || waiting.InvitedByEmail != "alice@example.com" || waiting.InvitedByName == "" || waiting.Role != apitypes.VaultEditor ||
		time.Since(waiting.AcceptedAt) > time.Minute {
		t.Errorf("Bob's waiting invitation: %+v", waiting)
	}
	aliceList := requireValue[*apitypes.ShareInvitationList](t)(alice.Client.ListShareInvitations(ctx))
	if len(aliceList.Sent) != 1 || aliceList.Sent[0].AcceptedAt == nil || !aliceList.Sent[0].AcceptedAt.Equal(waiting.AcceptedAt) || len(aliceList.Waiting) != 0 {
		t.Errorf("Alice's sent invitations: %+v", aliceList)
	}
	if carolList := requireValue[*apitypes.ShareInvitationList](t)(carol.Client.ListShareInvitations(ctx)); len(carolList.Waiting) != 0 || len(carolList.Sent) != 0 {
		t.Errorf("Carol sees someone else's share: %+v", carolList)
	}

	messages := recorder.WaitForMessageCount(emailCountBefore+1, 5*time.Second)[emailCountBefore:]
	if len(messages) != 1 || messages[0].To != "alice@example.com" || !strings.HasSuffix(messages[0].Subject, "accepted your invitation in Hisar") ||
		!strings.Contains(messages[0].Body, "bob@example.com") || !strings.Contains(messages[0].Body, "unlock it") || strings.Contains(messages[0].Body, "Work") {
		t.Fatalf("accepted email: %+v", messages)
	}

	if completedShares := completeReviewedShares(t, alice); len(completedShares) != 1 {
		t.Fatalf("completed: %+v", completedShares)
	}
	if bobList := requireValue[*apitypes.ShareInvitationList](t)(bob.Client.ListShareInvitations(ctx)); len(bobList.Waiting) != 0 {
		t.Errorf("Bob still waits after completion: %+v", bobList.Waiting)
	}

	t.Run("no email when the owner's app completes in time", func(t *testing.T) {
		server.service.ShareAcceptedEmailDelay = time.Second
		emailCountBefore := acceptInvitation(carol)
		if completedShares := completeReviewedShares(t, alice); len(completedShares) != 1 {
			t.Fatalf("completed: %+v", completedShares)
		}
		time.Sleep(1500 * time.Millisecond)
		server.WaitForBackgroundWork()
		if messages := recorder.ListMessages()[emailCountBefore:]; len(messages) != 0 {
			t.Errorf("emailed after completion: %+v", messages)
		}
	})

	t.Run("accepted emails are rate limited per inviter", func(t *testing.T) {
		server.service.ShareAcceptedEmailDelay = 50 * time.Millisecond
		server.service.Limiters.ShareAcceptedEmail = ratelimit.NewDaily(1)
		server.service.Limiters.ShareAcceptedEmail.Allow(alice.AccountID)
		dave := signUpTestAccount(t, ctx, server, baseURL, "dave@example.com", apitypes.RoleMember)
		emailCountBefore := acceptInvitation(dave)
		time.Sleep(500 * time.Millisecond)
		server.WaitForBackgroundWork()
		if messages := recorder.ListMessages()[emailCountBefore:]; len(messages) != 0 {
			t.Errorf("emailed over the limit: %+v", messages)
		}
	})
}

func TestInvitationsOfARemovedOwnerGrantNothing(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleMember)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", apitypes.RoleMember)
	carol := signUpTestAccount(t, ctx, server, baseURL, "carol@example.com", apitypes.RoleMember)
	dave := signUpTestAccount(t, ctx, server, baseURL, "dave@example.com", apitypes.RoleMember)
	team := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Team", apitypes.VaultShared))
	shareThroughInvitation(t, alice, bob, team, apitypes.VaultOwner)

	for _, recipient := range []*session.Session{carol, dave} {
		if err := alice.InviteToVault(ctx, team, recipient.Email, apitypes.VaultOwner); err != nil {
			t.Fatal(err)
		}
	}
	carolInvitations := requireValue[*apitypes.ShareInvitationList](t)(carol.Client.ListShareInvitations(ctx))
	daveInvitations := requireValue[*apitypes.ShareInvitationList](t)(dave.Client.ListShareInvitations(ctx))
	if len(carolInvitations.Received) != 1 || len(daveInvitations.Received) != 1 {
		t.Fatalf("received: %+v %+v", carolInvitations, daveInvitations)
	}
	if err := dave.Client.AcceptShareInvitation(ctx, daveInvitations.Received[0].ID); err != nil {
		t.Fatal(err)
	}

	bobTeam := requireValue[*session.Vault](t)(bob.FindVault(ctx, "Team"))
	if err := bob.RemoveVaultMember(ctx, bobTeam, alice.AccountID); err != nil {
		t.Fatal(err)
	}

	if received := requireValue[*apitypes.ShareInvitationList](t)(carol.Client.ListShareInvitations(ctx)).Received; len(received) != 0 {
		t.Errorf("the removed owner's invitation is still offered: %+v", received)
	}
	if err := carol.Client.AcceptShareInvitation(ctx, carolInvitations.Received[0].ID); !apiclient.IsStatus(err, http.StatusNotFound) {
		t.Errorf("accepting the removed owner's invitation: %v", err)
	}
	if waiting := requireValue[*apitypes.ShareInvitationList](t)(dave.Client.ListShareInvitations(ctx)).Waiting; len(waiting) != 0 {
		t.Errorf("an acceptance of the removed owner's invitation still waits: %+v", waiting)
	}
	completedShares, err := bob.CompleteAcceptedShares(ctx)
	if err != nil || len(completedShares) != 0 {
		t.Errorf("the remaining owner's sync completed: %+v %v", completedShares, err)
	}
	for _, recipient := range []*session.Session{carol, dave} {
		vaults := requireValue[[]apitypes.Vault](t)(recipient.Client.ListVaults(ctx))
		if slices.ContainsFunc(vaults, func(vault apitypes.Vault) bool { return vault.ID == team.ID }) {
			t.Errorf("%s got the vault through the removed owner's invitation", recipient.Email)
		}
	}
	if err := bob.Client.SetAccess(ctx, team.ID, dave.AccountID, &apitypes.SetAccessRequest{WrappedKey: []byte{1}, Role: apitypes.VaultOwner, KeyGeneration: bobTeam.KeyGeneration}); !apiclient.IsStatus(err, http.StatusConflict) || !strings.Contains(err.Error(), "has not accepted an invitation") {
		t.Errorf("granting the vault to the removed owner's invitee: %v", err)
	}
}
