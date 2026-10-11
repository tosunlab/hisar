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

	"github.com/tryhisar/hisar/core/client"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/server/internal/mail"
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

func findEmail(allMessage []mail.Message, recipient string) (mail.Message, bool) {
	for _, message := range allMessage {
		if message.To == recipient {
			return message, true
		}
	}
	return mail.Message{}, false
}

func TestInviteByEmail(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", proto.RoleAdmin)
	signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", proto.RoleMember)

	if _, err := alice.Client.Invite(ctx, &proto.InviteRequest{Email: "carol@example.com"}); !client.IsStatus(err, http.StatusNotImplemented) || !strings.Contains(err.Error(), "email is not configured on this server") {
		t.Fatalf("invite without email: %v", err)
	}

	recorder := attachMailRecorder(server)
	newStatus, newBody := sendRawRequest(t, baseURL, alice.Client.Token, "POST", "/api/v1/invites", proto.InviteRequest{Email: " Carol@Example.com "})
	existingStatus, existingBody := sendRawRequest(t, baseURL, alice.Client.Token, "POST", "/api/v1/invites", proto.InviteRequest{Email: "bob@example.com"})
	if newStatus != http.StatusAccepted || existingStatus != newStatus || existingBody != newBody || !strings.Contains(newBody, proto.InvitationSentMessage) {
		t.Fatalf("responses differ: %d %s / %d %s", newStatus, newBody, existingStatus, existingBody)
	}
	if strings.Contains(newBody, "invite/") || strings.Contains(newBody, "token") {
		t.Errorf("the response carries the invite: %s", newBody)
	}
	server.WaitForBackgroundWork()
	allMessage := recorder.ListAllMessage()
	if len(allMessage) != 1 {
		t.Fatalf("emails: %+v", allMessage)
	}
	invite, found := findEmail(allMessage, "carol@example.com")
	token := findInviteToken(t, server, recorder, "carol@example.com")
	if !found || invite.Subject != "alice@example.com invited you to Hisar" ||
		!strings.Contains(invite.Body, "https://hisar.test/invite/"+token) || !strings.Contains(invite.Body, "\n"+token+"\n") || !strings.Contains(invite.Body, "alice@example.com") {
		t.Errorf("invite email: %+v", invite)
	}
	carol, _, err := session.SignUp(ctx, client.Create(baseURL), token, "carol@example.com", "Carol", []byte("carol password"), testDevice)
	if err != nil {
		t.Fatalf("sign up with the emailed code: %v", err)
	}
	if carol.Role != "" && carol.Role != proto.RoleMember {
		t.Errorf("Carol's role: %q", carol.Role)
	}
	if account := requireValue[*proto.Account](t)(carol.Client.GetAccount(ctx)); account.Role != proto.RoleMember {
		t.Errorf("Carol's role: %q", account.Role)
	}

	for _, badEmail := range []string{"", "not an email", "Bob <bob@example.com>", "a@b.c\r\nBcc: eve@example.com"} {
		if _, err := alice.Client.Invite(ctx, &proto.InviteRequest{Email: badEmail}); !client.IsStatus(err, http.StatusBadRequest) {
			t.Errorf("invite %q: %v", badEmail, err)
		}
	}

	server.inviteAccountLimiter = createDailyLimiter(2)
	for index := range 2 {
		if _, err := carol.Client.Invite(ctx, &proto.InviteRequest{Email: "dave@example.com"}); err != nil {
			t.Fatalf("invite %d: %v", index, err)
		}
	}
	if _, err := carol.Client.Invite(ctx, &proto.InviteRequest{Email: "dave@example.com"}); !client.IsStatus(err, http.StatusTooManyRequests) {
		t.Errorf("third invite of the day: %v", err)
	}
	if _, err := alice.Client.Invite(ctx, &proto.InviteRequest{Email: "dave@example.com"}); err != nil {
		t.Errorf("another account's invite was limited: %v", err)
	}
	server.inviteIPLimiter = createDailyLimiter(1)
	if _, err := alice.Client.Invite(ctx, &proto.InviteRequest{Email: "erin@example.com"}); err != nil {
		t.Fatal(err)
	}
	if _, err := alice.Client.Invite(ctx, &proto.InviteRequest{Email: "erin@example.com"}); !client.IsStatus(err, http.StatusTooManyRequests) {
		t.Errorf("invite over the IP limit: %v", err)
	}
	server.WaitForBackgroundWork()
}

func TestShareInvitationWithoutEnumeration(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	recorder := attachMailRecorder(server)
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", proto.RoleAdmin)
	grantFoundingMember(t, server, alice.AccountID)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", proto.RoleMember)
	carol := signUpTestAccount(t, ctx, server, baseURL, "carol@example.com", proto.RoleMember)
	family := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Family", proto.VaultShared))
	requireValue[*item.Item](t)(alice.SaveItem(ctx, family, &item.Item{Overview: item.Overview{Title: "Netflix"}, Details: item.Details{Password: "hunter2"}}))
	work := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Work", proto.VaultShared))
	shareThroughInvitation(t, alice, carol, work, proto.VaultReader)
	server.WaitForBackgroundWork()
	emailCountBefore := len(recorder.ListAllMessage())

	invitationPath := "/api/v1/vaults/" + family.ID + "/share-invitations"
	existingStatus, existingBody := sendRawRequest(t, baseURL, alice.Client.Token, "POST", invitationPath, proto.ShareInvitationRequest{Email: "Bob@Example.com", Role: proto.VaultEditor})
	unknownStatus, unknownBody := sendRawRequest(t, baseURL, alice.Client.Token, "POST", invitationPath, proto.ShareInvitationRequest{Email: "ghost@example.com", Role: proto.VaultEditor})
	if existingStatus != http.StatusAccepted || unknownStatus != existingStatus || unknownBody != existingBody || !strings.Contains(existingBody, proto.InvitationSentMessage) {
		t.Fatalf("responses differ: %d %s / %d %s", existingStatus, existingBody, unknownStatus, unknownBody)
	}
	allMessage := recorder.WaitForMessageCount(emailCountBefore+2, 5*time.Second)[emailCountBefore:]
	toBob, isToBob := findEmail(allMessage, "bob@example.com")
	if !isToBob || toBob.Subject != "alice@example.com shared a vault with you in Hisar" || !strings.Contains(toBob.Body, "as an editor") ||
		strings.Contains(toBob.Body, "Family") || strings.Contains(toBob.Body, "/invite/") {
		t.Errorf("email to Bob: %+v", toBob)
	}
	toGhost, isToGhost := findEmail(allMessage, "ghost@example.com")
	if !isToGhost || !strings.Contains(toGhost.Body, "https://hisar.test/invite/") || strings.Contains(toGhost.Body, "Family") {
		t.Errorf("email to the unknown address: %+v", toGhost)
	}

	invitationList := requireValue[*proto.ShareInvitationList](t)(bob.Client.ListAllShareInvitation(ctx))
	if len(invitationList.AllReceived) != 1 || len(invitationList.AllSent) != 0 {
		t.Fatalf("Bob's invitations: %+v", invitationList)
	}
	received := invitationList.AllReceived[0]
	if received.VaultID != family.ID || received.Role != proto.VaultEditor || received.InvitedByEmail != "alice@example.com" {
		t.Errorf("received: %+v", received)
	}
	if carolList := requireValue[*proto.ShareInvitationList](t)(carol.Client.ListAllShareInvitation(ctx)); len(carolList.AllReceived) != 0 || len(carolList.AllSent) != 0 {
		t.Errorf("Carol sees invitations: %+v", carolList)
	}
	aliceList := requireValue[*proto.ShareInvitationList](t)(alice.Client.ListAllShareInvitation(ctx))
	if len(aliceList.AllSent) != 2 {
		t.Fatalf("Alice's sent invitations: %+v", aliceList)
	}
	for _, sent := range aliceList.AllSent {
		if sent.State != proto.ShareInvitationPending || sent.Recipient != nil {
			t.Errorf("a pending invitation reveals its recipient: %+v", sent)
		}
	}

	bobKeys := requireValue[*proto.Account](t)(bob.Client.GetAccount(ctx)).PublicKeys
	wrappedKey := requireValue[[]byte](t)(crypt.WrapVaultKey(bobKeys.KEM, family.ID, bob.AccountID, family.Key))
	if err := alice.Client.SetAccess(ctx, family.ID, bob.AccountID, &proto.SetAccessRequest{WrappedKey: wrappedKey, Role: proto.VaultEditor}); !client.IsStatus(err, http.StatusConflict) {
		t.Errorf("access before Bob accepted: %v", err)
	}
	if err := carol.Client.AcceptShareInvitation(ctx, received.ID); !client.IsStatus(err, http.StatusNotFound) {
		t.Errorf("Carol accepted Bob's invitation: %v", err)
	}
	if err := bob.Client.AcceptShareInvitation(ctx, received.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := bob.FindVault(ctx, "Family"); !errors.Is(err, session.ErrNotFound) {
		t.Errorf("Bob has access before Alice completed: %v", err)
	}
	aliceList = requireValue[*proto.ShareInvitationList](t)(alice.Client.ListAllShareInvitation(ctx))
	acceptedIndex := slices.IndexFunc(aliceList.AllSent, func(sent proto.SentShareInvitation) bool { return sent.State == proto.ShareInvitationAccepted })
	if acceptedIndex < 0 || aliceList.AllSent[acceptedIndex].Recipient == nil || aliceList.AllSent[acceptedIndex].Recipient.AccountID != bob.AccountID ||
		aliceList.AllSent[acceptedIndex].Recipient.PublicKeys.Verify(bob.AccountID) != nil {
		t.Fatalf("Alice's accepted invitation: %+v", aliceList.AllSent)
	}
	if carolList := requireValue[*proto.ShareInvitationList](t)(carol.Client.ListAllShareInvitation(ctx)); len(carolList.AllSent) != 0 {
		t.Errorf("a member who does not own the vault sees its invitations: %+v", carolList.AllSent)
	}
	allCompleted := requireValue[[]session.CompletedShare](t)(alice.CompleteAllAcceptedShare(ctx))
	if len(allCompleted) != 1 || allCompleted[0].VaultName != "Family" || allCompleted[0].Recipient.Email != "bob@example.com" {
		t.Fatalf("completed: %+v", allCompleted)
	}
	bobFamily := requireValue[*session.Vault](t)(bob.FindVault(ctx, "Family"))
	if allItem := requireValue[[]item.Item](t)(bob.ListAllItem(ctx, bobFamily)); len(allItem) != 1 || allItem[0].Details.Password != "hunter2" {
		t.Errorf("Bob's items: %+v", allItem)
	}
	if allCompleted := requireValue[[]session.CompletedShare](t)(alice.CompleteAllAcceptedShare(ctx)); len(allCompleted) != 0 {
		t.Errorf("completed twice: %+v", allCompleted)
	}

	t.Run("people are only those who share a vault", func(t *testing.T) {
		listAllEmail := func(currentSession *session.Session) []string {
			var allEmail []string
			for _, member := range requireValue[[]proto.Member](t)(currentSession.Client.ListAllMember(ctx)) {
				allEmail = append(allEmail, member.Email)
			}
			slices.Sort(allEmail)
			return allEmail
		}
		if allEmail := listAllEmail(bob); !slices.Equal(allEmail, []string{"alice@example.com", "bob@example.com"}) {
			t.Errorf("Bob sees %v", allEmail)
		}
		if allEmail := listAllEmail(alice); !slices.Equal(allEmail, []string{"alice@example.com", "bob@example.com", "carol@example.com"}) {
			t.Errorf("Alice sees %v", allEmail)
		}
	})

	t.Run("refusals that depend only on the owner's data", func(t *testing.T) {
		if err := alice.InviteToVault(ctx, family, "alice@example.com", proto.VaultEditor); !client.IsStatus(err, http.StatusBadRequest) {
			t.Errorf("share with yourself: %v", err)
		}
		if err := alice.InviteToVault(ctx, family, "bob@example.com", proto.VaultReader); !client.IsStatus(err, http.StatusConflict) {
			t.Errorf("share with a member: %v", err)
		}
		if err := bob.InviteToVault(ctx, bobFamily, "carol@example.com", proto.VaultReader); !client.IsStatus(err, http.StatusForbidden) {
			t.Errorf("an editor shared: %v", err)
		}
		alicePersonal := requireValue[*session.Vault](t)(alice.FindVault(ctx, "Personal"))
		if err := alice.InviteToVault(ctx, alicePersonal, "carol@example.com", proto.VaultReader); !client.IsStatus(err, http.StatusBadRequest) {
			t.Errorf("shared a personal vault: %v", err)
		}
		if err := alice.InviteToVault(ctx, family, "carol@example.com", "admin"); !client.IsStatus(err, http.StatusBadRequest) {
			t.Errorf("invalid role: %v", err)
		}
		if err := alice.InviteToVault(ctx, family, "not an email", proto.VaultReader); !client.IsStatus(err, http.StatusBadRequest) {
			t.Errorf("invalid email: %v", err)
		}
		if err := carol.InviteToVault(ctx, family, "dave@example.com", proto.VaultReader); !client.IsStatus(err, http.StatusNotFound) {
			t.Errorf("a non-member shared: %v", err)
		}
	})

	t.Run("decline and cancel", func(t *testing.T) {
		if err := alice.InviteToVault(ctx, family, "carol@example.com", proto.VaultReader); err != nil {
			t.Fatal(err)
		}
		carolList := requireValue[*proto.ShareInvitationList](t)(carol.Client.ListAllShareInvitation(ctx))
		if len(carolList.AllReceived) != 1 {
			t.Fatalf("Carol's invitations: %+v", carolList)
		}
		if err := carol.Client.DeclineShareInvitation(ctx, carolList.AllReceived[0].ID); err != nil {
			t.Fatal(err)
		}
		if err := carol.Client.AcceptShareInvitation(ctx, carolList.AllReceived[0].ID); !client.IsStatus(err, http.StatusNotFound) {
			t.Errorf("accept after declining: %v", err)
		}
		aliceList := requireValue[*proto.ShareInvitationList](t)(alice.Client.ListAllShareInvitation(ctx))
		ghostIndex := slices.IndexFunc(aliceList.AllSent, func(sent proto.SentShareInvitation) bool { return sent.Email == "ghost@example.com" })
		if len(aliceList.AllSent) != 1 || ghostIndex != 0 {
			t.Fatalf("Alice's sent invitations: %+v", aliceList.AllSent)
		}
		if err := bob.Client.CancelShareInvitation(ctx, aliceList.AllSent[0].ID); !client.IsStatus(err, http.StatusNotFound) {
			t.Errorf("an editor cancelled: %v", err)
		}
		if err := alice.Client.CancelShareInvitation(ctx, aliceList.AllSent[0].ID); err != nil {
			t.Fatal(err)
		}
		if err := alice.Client.CancelShareInvitation(ctx, "not-a-uuid"); !client.IsStatus(err, http.StatusNotFound) {
			t.Errorf("cancel an invalid ID: %v", err)
		}
	})

	t.Run("a rotation cannot add someone", func(t *testing.T) {
		carolKeys := requireValue[*proto.Account](t)(carol.Client.GetAccount(ctx)).PublicKeys
		request := &proto.RotateVaultRequest{EncName: []byte{1}}
		for _, accountID := range []string{alice.AccountID, bob.AccountID} {
			request.AllAccess = append(request.AllAccess, proto.RotatedAccess{AccountID: accountID, Role: proto.VaultOwner, WrappedKey: []byte{1}})
		}
		request.AllAccess = append(request.AllAccess, proto.RotatedAccess{AccountID: carol.AccountID, Role: proto.VaultReader, WrappedKey: carolKeys.KEM[:32]})
		allChange := requireValue[*proto.ChangesResponse](t)(alice.Client.ListAllChange(ctx, family.ID, 0, 0))
		for _, protoItem := range allChange.Items {
			request.AllItem = append(request.AllItem, proto.RotatedItem{ID: protoItem.ID, BaseRevision: protoItem.Revision, Revision: "01890000-0000-7000-8000-000000000001", Overview: []byte{1}, Details: []byte{1}})
		}
		if err := alice.Client.RotateVault(ctx, family.ID, request); !client.IsStatus(err, http.StatusBadRequest) {
			t.Errorf("rotation adding Carol: %v", err)
		}
	})

	t.Run("an account created from the share email sees the invitation", func(t *testing.T) {
		token := findInviteToken(t, server, recorder, "ghost@example.com")
		if err := alice.InviteToVault(ctx, family, "ghost@example.com", proto.VaultReader); err != nil {
			t.Fatal(err)
		}
		ghost, _, err := session.SignUp(ctx, client.Create(baseURL), token, "ghost@example.com", "Ghost", []byte("ghost password"), testDevice)
		if err != nil {
			t.Fatal(err)
		}
		shareList := requireValue[*proto.ShareInvitationList](t)(ghost.Client.ListAllShareInvitation(ctx))
		if len(shareList.AllReceived) != 1 || shareList.AllReceived[0].VaultID != family.ID {
			t.Fatalf("Ghost's invitations: %+v", shareList)
		}
		if err := ghost.Client.AcceptShareInvitation(ctx, shareList.AllReceived[0].ID); err != nil {
			t.Fatal(err)
		}
		if allCompleted := requireValue[[]session.CompletedShare](t)(alice.CompleteAllAcceptedShare(ctx)); len(allCompleted) != 1 || allCompleted[0].Role != proto.VaultReader {
			t.Fatalf("completed: %+v", allCompleted)
		}
		requireValue[*session.Vault](t)(ghost.FindVault(ctx, "Family"))
	})

	t.Run("share invitations are rate limited", func(t *testing.T) {
		server.shareInvitationAccountLimiter = createDailyLimiter(1)
		if err := alice.InviteToVault(ctx, work, "frank@example.com", proto.VaultReader); err != nil {
			t.Fatal(err)
		}
		if err := alice.InviteToVault(ctx, work, "grace@example.com", proto.VaultReader); !client.IsStatus(err, http.StatusTooManyRequests) {
			t.Errorf("over the limit: %v", err)
		}
	})
	server.WaitForBackgroundWork()
}

func TestCompleteAcceptedShareRefusesChangedKey(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", proto.RoleAdmin)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", proto.RoleMember)
	family := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Family", proto.VaultShared))
	allPinnedKey := map[string][]byte{}
	alice.MemberVerifier = func(member *proto.Member) error {
		if pinned, found := allPinnedKey[member.AccountID]; found && !bytes.Equal(pinned, member.PublicKeys.Sign) {
			return errors.New("signing key changed")
		}
		allPinnedKey[member.AccountID] = member.PublicKeys.Sign
		return nil
	}
	allPinnedKey[bob.AccountID] = []byte("a different key")
	if err := alice.InviteToVault(ctx, family, "bob@example.com", proto.VaultEditor); err != nil {
		t.Fatal(err)
	}
	invitationList := requireValue[*proto.ShareInvitationList](t)(bob.Client.ListAllShareInvitation(ctx))
	if err := bob.Client.AcceptShareInvitation(ctx, invitationList.AllReceived[0].ID); err != nil {
		t.Fatal(err)
	}
	if allCompleted, err := alice.CompleteAllAcceptedShare(ctx); len(allCompleted) != 0 || err == nil || !strings.Contains(err.Error(), "signing key changed") {
		t.Errorf("completed with a changed key: %+v %v", allCompleted, err)
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
	server.config.ShareAcceptedEmailDelay = 200 * time.Millisecond
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", proto.RoleAdmin)
	grantFoundingMember(t, server, alice.AccountID)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", proto.RoleMember)
	carol := signUpTestAccount(t, ctx, server, baseURL, "carol@example.com", proto.RoleMember)
	work := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Work", proto.VaultShared))
	server.WaitForBackgroundWork()

	acceptInvitation := func(recipient *session.Session) (emailCountBefore int) {
		t.Helper()
		if err := alice.InviteToVault(ctx, work, recipient.Email, proto.VaultEditor); err != nil {
			t.Fatal(err)
		}
		server.WaitForBackgroundWork()
		invitationList := requireValue[*proto.ShareInvitationList](t)(recipient.Client.ListAllShareInvitation(ctx))
		if len(invitationList.AllReceived) != 1 || len(invitationList.AllWaiting) != 0 {
			t.Fatalf("%s before accepting: %+v", recipient.Email, invitationList)
		}
		emailCountBefore = len(recorder.ListAllMessage())
		if err := recipient.Client.AcceptShareInvitation(ctx, invitationList.AllReceived[0].ID); err != nil {
			t.Fatal(err)
		}
		return emailCountBefore
	}

	emailCountBefore := acceptInvitation(bob)
	bobList := requireValue[*proto.ShareInvitationList](t)(bob.Client.ListAllShareInvitation(ctx))
	if len(bobList.AllReceived) != 0 || len(bobList.AllWaiting) != 1 {
		t.Fatalf("Bob after accepting: %+v", bobList)
	}
	waiting := bobList.AllWaiting[0]
	if waiting.VaultID != work.ID || waiting.InvitedByEmail != "alice@example.com" || waiting.InvitedByName == "" || waiting.Role != proto.VaultEditor ||
		time.Since(waiting.AcceptedAt) > time.Minute {
		t.Errorf("Bob's waiting invitation: %+v", waiting)
	}
	aliceList := requireValue[*proto.ShareInvitationList](t)(alice.Client.ListAllShareInvitation(ctx))
	if len(aliceList.AllSent) != 1 || aliceList.AllSent[0].AcceptedAt == nil || !aliceList.AllSent[0].AcceptedAt.Equal(waiting.AcceptedAt) || len(aliceList.AllWaiting) != 0 {
		t.Errorf("Alice's sent invitations: %+v", aliceList)
	}
	if carolList := requireValue[*proto.ShareInvitationList](t)(carol.Client.ListAllShareInvitation(ctx)); len(carolList.AllWaiting) != 0 || len(carolList.AllSent) != 0 {
		t.Errorf("Carol sees someone else's share: %+v", carolList)
	}

	allMessage := recorder.WaitForMessageCount(emailCountBefore+1, 5*time.Second)[emailCountBefore:]
	if len(allMessage) != 1 || allMessage[0].To != "alice@example.com" || !strings.HasSuffix(allMessage[0].Subject, "accepted your invitation in Hisar") ||
		!strings.Contains(allMessage[0].Body, "bob@example.com") || !strings.Contains(allMessage[0].Body, "unlock it") || strings.Contains(allMessage[0].Body, "Work") {
		t.Fatalf("accepted email: %+v", allMessage)
	}

	if allCompleted := requireValue[[]session.CompletedShare](t)(alice.CompleteAllAcceptedShare(ctx)); len(allCompleted) != 1 {
		t.Fatalf("completed: %+v", allCompleted)
	}
	if bobList := requireValue[*proto.ShareInvitationList](t)(bob.Client.ListAllShareInvitation(ctx)); len(bobList.AllWaiting) != 0 {
		t.Errorf("Bob still waits after completion: %+v", bobList.AllWaiting)
	}

	t.Run("no email when the owner's app completes in time", func(t *testing.T) {
		server.config.ShareAcceptedEmailDelay = time.Second
		emailCountBefore := acceptInvitation(carol)
		if allCompleted := requireValue[[]session.CompletedShare](t)(alice.CompleteAllAcceptedShare(ctx)); len(allCompleted) != 1 {
			t.Fatalf("completed: %+v", allCompleted)
		}
		time.Sleep(1500 * time.Millisecond)
		server.WaitForBackgroundWork()
		if allMessage := recorder.ListAllMessage()[emailCountBefore:]; len(allMessage) != 0 {
			t.Errorf("emailed after completion: %+v", allMessage)
		}
	})

	t.Run("accepted emails are rate limited per inviter", func(t *testing.T) {
		server.config.ShareAcceptedEmailDelay = 50 * time.Millisecond
		server.shareAcceptedEmailLimiter = createDailyLimiter(1)
		server.shareAcceptedEmailLimiter.allow(alice.AccountID)
		dave := signUpTestAccount(t, ctx, server, baseURL, "dave@example.com", proto.RoleMember)
		emailCountBefore := acceptInvitation(dave)
		time.Sleep(500 * time.Millisecond)
		server.WaitForBackgroundWork()
		if allMessage := recorder.ListAllMessage()[emailCountBefore:]; len(allMessage) != 0 {
			t.Errorf("emailed over the limit: %+v", allMessage)
		}
	})
}
