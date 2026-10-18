package hisarkit

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

type remoteChangeRecorder struct {
	remoteChanges chan struct{}
}

func createRemoteChangeRecorder() *remoteChangeRecorder {
	return &remoteChangeRecorder{remoteChanges: make(chan struct{}, 64)}
}

func (recorder *remoteChangeRecorder) OnLocked() {}

func (recorder *remoteChangeRecorder) OnApprovalEvent(eventName, requestID string) {}

func (recorder *remoteChangeRecorder) OnUpdateRequired() {}

func (recorder *remoteChangeRecorder) OnRemoteChange() {
	select {
	case recorder.remoteChanges <- struct{}{}:
	default:
	}
}

func (recorder *remoteChangeRecorder) drain() {
	for {
		select {
		case <-recorder.remoteChanges:
		default:
			return
		}
	}
}

func (recorder *remoteChangeRecorder) waitUntil(t *testing.T, description string, isDone func() bool) {
	t.Helper()
	deadline := time.After(30 * time.Second)
	for !isDone() {
		select {
		case <-recorder.remoteChanges:
		case <-time.After(time.Second):
		case <-deadline:
			t.Fatalf("timed out waiting until %s", description)
		}
	}
}

type sharingVaultSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Role string `json:"role"`
}

type sharingMember struct {
	AccountID     string `json:"accountId"`
	Email         string `json:"email"`
	Role          string `json:"role"`
	SecurityCode  string `json:"securityCode"`
	IsCurrentUser bool   `json:"isCurrentUser"`
	IsKeyChanged  bool   `json:"isKeyChanged"`
}

type vaultSharing struct {
	Vault       sharingVaultSummary `json:"vault"`
	IsOwner     bool                `json:"isOwner"`
	Members     []sharingMember     `json:"members"`
	Invitations []struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Role  string `json:"role"`
		State string `json:"state"`
	} `json:"invitations"`
	CompletionError string `json:"completionError"`
}

func signUpMobile(t *testing.T, stateDirectory, inviteLink, email, name string, listener EventListener) *Hisar {
	t.Helper()
	hisar, err := New(stateDirectory, "Test iPhone", createMemorySecretKeyStore(), nil, &fakeClipboard{}, listener)
	if err != nil {
		t.Fatal(err)
	}
	signUp := decodeJSON[struct {
		Status status `json:"status"`
	}](t)(hisar.SignUp(inviteLink, email, name, "mobile sharing master password"))
	if signUp.Status.State != "unlocked" {
		t.Fatalf("sign up %s: %+v", email, signUp)
	}
	t.Cleanup(func() { hisar.Lock() })
	return hisar
}

func findVaultByName(t *testing.T, hisar *Hisar, name string) *sharingVaultSummary {
	t.Helper()
	vaults := decodeJSON[[]sharingVaultSummary](t)(hisar.ListAllVault())
	index := slices.IndexFunc(vaults, func(vault sharingVaultSummary) bool { return vault.Name == name })
	if index < 0 {
		return nil
	}
	return &vaults[index]
}

// TestIntegrationMobileSharing runs against a real server from scripts/test-end-to-end.sh: the iPhone owns a vault,
// shares it, and completes the share on the event the recipient's acceptance causes.
func TestIntegrationMobileSharing(t *testing.T) {
	ownerInviteLink := os.Getenv("HISAR_TEST_MOBILE_SHARING_OWNER_INVITE_LINK")
	recipientInviteLink := os.Getenv("HISAR_TEST_MOBILE_SHARING_RECIPIENT_INVITE_LINK")
	if ownerInviteLink == "" || recipientInviteLink == "" {
		t.Skip("HISAR_TEST_MOBILE_SHARING_OWNER_INVITE_LINK and HISAR_TEST_MOBILE_SHARING_RECIPIENT_INVITE_LINK not set")
	}
	const ownerEmail, recipientEmail = "mobile-sharing-owner@example.com", "mobile-sharing-recipient@example.com"
	ownerRecorder := createRemoteChangeRecorder()
	owner := signUpMobile(t, t.TempDir(), ownerInviteLink, ownerEmail, "Mobile Owner", ownerRecorder)
	recipientRecorder := createRemoteChangeRecorder()
	recipient := signUpMobile(t, t.TempDir(), recipientInviteLink, recipientEmail, "Mobile Recipient", recipientRecorder)

	personal := findVaultByName(t, owner, "Personal")
	if personal == nil {
		t.Fatal("the owner has no personal vault")
	}
	if _, err := owner.GetVaultSharing(personal.ID); err == nil || !strings.Contains(err.Error(), "personal vaults are private") {
		t.Errorf("sharing of the personal vault: %v", err)
	}
	deployKeys := decodeJSON[sharingVaultSummary](t)(owner.CreateVault("Deploy keys"))
	saved := decodeJSON[struct {
		ID string `json:"id"`
	}](t)(owner.SaveItem(deployKeys.ID, `{"title":"Staging database","username":"deploy","password":"staging-database-password","urls":[],"fields":[]}`))

	if err := owner.ShareVault(deployKeys.ID, recipientEmail, "reader"); err != nil {
		t.Fatalf("share with an account: %v", err)
	}
	if err := owner.ShareVault(deployKeys.ID, "nobody-mobile-sharing@example.com", "reader"); err != nil {
		t.Errorf("share with an address without an account answers differently: %v", err)
	}
	sharing := decodeJSON[vaultSharing](t)(owner.GetVaultSharing(deployKeys.ID))
	if !sharing.IsOwner || sharing.Vault.Name != "Deploy keys" || len(sharing.Members) != 1 || !sharing.Members[0].IsCurrentUser || len(sharing.Invitations) != 2 {
		t.Fatalf("sharing before anyone accepted: %+v", sharing)
	}
	for _, invitation := range sharing.Invitations {
		if invitation.State != "pending" || invitation.Role != "reader" {
			t.Errorf("invitation = %+v", invitation)
		}
	}

	type received struct {
		ID             string `json:"id"`
		VaultID        string `json:"vaultId"`
		Role           string `json:"role"`
		InvitedByEmail string `json:"invitedByEmail"`
	}
	var invitation received
	recipientRecorder.waitUntil(t, "the recipient sees the invitation", func() bool {
		receivedInvitations := decodeJSON[struct {
			Received []received `json:"received"`
		}](t)(recipient.ListAllShareInvitation()).Received
		if len(receivedInvitations) == 1 {
			invitation = receivedInvitations[0]
		}
		return len(receivedInvitations) == 1
	})
	if invitation.VaultID != deployKeys.ID || invitation.Role != "reader" || invitation.InvitedByEmail != ownerEmail {
		t.Fatalf("received invitation = %+v", invitation)
	}
	if findVaultByName(t, recipient, "Deploy keys") != nil {
		t.Fatal("the recipient sees the vault before accepting")
	}

	ownerRecorder.drain()
	if err := recipient.AcceptShareInvitation(invitation.ID); err != nil {
		t.Fatal(err)
	}
	ownerRecorder.waitUntil(t, "the owner's iPhone completes the share on its own", func() bool {
		members := decodeJSON[[]sharingMember](t)(owner.ListAllVaultMember(deployKeys.ID))
		return slices.ContainsFunc(members, func(member sharingMember) bool { return member.Email == recipientEmail })
	})
	sharing = decodeJSON[vaultSharing](t)(owner.GetVaultSharing(deployKeys.ID))
	if len(sharing.Members) != 2 || sharing.CompletionError != "" || len(sharing.Invitations) != 1 || sharing.Invitations[0].Email != "nobody-mobile-sharing@example.com" {
		t.Fatalf("sharing after completing: %+v", sharing)
	}
	recipientMember := sharing.Members[1]
	if recipientMember.Email != recipientEmail || recipientMember.Role != "reader" || len(recipientMember.SecurityCode) != 19 || recipientMember.IsKeyChanged {
		t.Errorf("recipient as a member = %+v", recipientMember)
	}
	recipientPeople := decodeJSON[[]sharingMember](t)(recipient.ListAllPerson())
	recipientSelf := recipientPeople[slices.IndexFunc(recipientPeople, func(member sharingMember) bool { return member.IsCurrentUser })]
	if recipientSelf.SecurityCode != recipientMember.SecurityCode {
		t.Errorf("security codes differ: the owner sees %s, the recipient %s", recipientMember.SecurityCode, recipientSelf.SecurityCode)
	}

	var shared *sharingVaultSummary
	recipientRecorder.waitUntil(t, "the recipient sees the shared vault", func() bool {
		shared = findVaultByName(t, recipient, "Deploy keys")
		return shared != nil
	})
	if shared.Role != "reader" || shared.Type != "shared" {
		t.Errorf("shared vault for the recipient = %+v", shared)
	}
	item := decodeJSON[struct {
		Password string `json:"password"`
	}](t)(recipient.GetItem(deployKeys.ID, saved.ID))
	if item.Password != "staging-database-password" {
		t.Errorf("the recipient reads %q", item.Password)
	}
	recipientSharing := decodeJSON[vaultSharing](t)(recipient.GetVaultSharing(deployKeys.ID))
	if recipientSharing.IsOwner || len(recipientSharing.Members) != 2 || len(recipientSharing.Invitations) != 0 {
		t.Errorf("sharing for a reader: %+v", recipientSharing)
	}
	if err := recipient.ShareVault(deployKeys.ID, "someone@example.com", "reader"); err == nil {
		t.Error("a reader shared the vault")
	}

	if err := owner.ChangeVaultMemberRole(deployKeys.ID, recipientMember.AccountID, "editor"); err != nil {
		t.Fatal(err)
	}
	recipientRecorder.waitUntil(t, "the recipient becomes an editor", func() bool {
		vault := findVaultByName(t, recipient, "Deploy keys")
		return vault != nil && vault.Role == "editor"
	})
	if err := owner.RemoveVaultMember(deployKeys.ID, recipientMember.AccountID); err != nil {
		t.Fatal(err)
	}
	recipientRecorder.waitUntil(t, "the removed recipient loses the vault", func() bool {
		return findVaultByName(t, recipient, "Deploy keys") == nil
	})
	if members := decodeJSON[vaultSharing](t)(owner.GetVaultSharing(deployKeys.ID)).Members; len(members) != 1 {
		t.Errorf("members after removing: %+v", members)
	}
	if item := decodeJSON[struct {
		Password string `json:"password"`
	}](t)(owner.GetItem(deployKeys.ID, saved.ID)); item.Password != "staging-database-password" {
		t.Errorf("the owner reads %q after the key rotation", item.Password)
	}

	checkShareWaitingForLockedOwner(t, owner, recipient, ownerEmail, recipientEmail)
}

// checkShareWaitingForLockedOwner: a locked owner cannot complete a share, so both sides are told who it waits for.
func checkShareWaitingForLockedOwner(t *testing.T, owner, recipient *Hisar, ownerEmail, recipientEmail string) {
	t.Helper()
	releaseKeys := decodeJSON[sharingVaultSummary](t)(owner.CreateVault("Release keys"))
	if err := owner.ShareVault(releaseKeys.ID, recipientEmail, "editor"); err != nil {
		t.Fatal(err)
	}
	owner.Lock()
	type receivedInvitation struct {
		ID      string `json:"id"`
		VaultID string `json:"vaultId"`
	}
	type waitingList struct {
		Received []receivedInvitation `json:"received"`
		Waiting  []struct {
			VaultID        string `json:"vaultId"`
			InvitedByName  string `json:"invitedByName"`
			InvitedByEmail string `json:"invitedByEmail"`
			AcceptedAt     string `json:"acceptedAt"`
		} `json:"waiting"`
	}
	received := decodeJSON[waitingList](t)(recipient.ListAllShareInvitation()).Received
	index := slices.IndexFunc(received, func(invitation receivedInvitation) bool { return invitation.VaultID == releaseKeys.ID })
	if index < 0 {
		t.Fatalf("no invitation to Release keys: %+v", received)
	}
	if err := recipient.AcceptShareInvitation(received[index].ID); err != nil {
		t.Fatal(err)
	}
	waiting := decodeJSON[waitingList](t)(recipient.ListAllShareInvitation()).Waiting
	if len(waiting) != 1 || waiting[0].VaultID != releaseKeys.ID || waiting[0].InvitedByName != "Mobile Owner" || waiting[0].InvitedByEmail != ownerEmail {
		t.Fatalf("the recipient's waiting share: %+v", waiting)
	}
	if acceptedAt, err := time.Parse(time.RFC3339, waiting[0].AcceptedAt); err != nil || time.Since(acceptedAt) > time.Minute {
		t.Errorf("accepted at %q: %v", waiting[0].AcceptedAt, err)
	}

	type awaitingUnlock struct {
		RecipientName  string `json:"recipientName"`
		RecipientEmail string `json:"recipientEmail"`
		AcceptedAt     string `json:"acceptedAt"`
	}
	awaitingShares := decodeJSON[[]awaitingUnlock](t)(owner.ListAllShareAwaitingUnlock())
	if len(awaitingShares) != 1 || awaitingShares[0].RecipientName != "Mobile Recipient" || awaitingShares[0].RecipientEmail != recipientEmail || awaitingShares[0].AcceptedAt != waiting[0].AcceptedAt {
		t.Fatalf("the locked owner's banner: %+v", awaitingShares)
	}

	if _, err := owner.Unlock("mobile sharing master password"); err != nil {
		t.Fatal(err)
	}
	if awaitingShares := decodeJSON[[]awaitingUnlock](t)(owner.ListAllShareAwaitingUnlock()); len(awaitingShares) != 0 {
		t.Errorf("unlocked owner still has a lock screen banner: %+v", awaitingShares)
	}
	decodeJSON[waitingList](t)(owner.ListAllShareInvitation())
	if waiting := decodeJSON[waitingList](t)(recipient.ListAllShareInvitation()).Waiting; len(waiting) != 0 {
		t.Errorf("the recipient still waits after the owner unlocked: %+v", waiting)
	}
	if findVaultByName(t, recipient, "Release keys") == nil {
		t.Error("the recipient has no access after the owner unlocked")
	}
}
