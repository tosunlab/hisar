package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

func TestShareInvitationLifecycle(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	createTestAccount(t, database, "BBBBBB", "b@example.com")
	createTestAccount(t, database, "CCCCCC", "c@example.com")
	vaultID := createTestVault(t, database, "AAAAAA")

	invitationID, err := database.UpsertShareInvitation(ctx, &ShareInvitation{VaultID: vaultID, Email: "b@example.com", Role: "editor", InvitedBy: "AAAAAA"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	renewedID, err := database.UpsertShareInvitation(ctx, &ShareInvitation{VaultID: vaultID, Email: "b@example.com", Role: "reader", InvitedBy: "AAAAAA"}, time.Hour)
	if err != nil || renewedID != invitationID {
		t.Fatalf("renewed invitation %s, %v", renewedID, err)
	}

	receivedInvitations, err := database.ListReceivedShareInvitations(ctx, "b@example.com")
	if err != nil || len(receivedInvitations) != 1 || receivedInvitations[0].Role != "reader" || receivedInvitations[0].InvitedByEmail != "a@example.com" || receivedInvitations[0].InvitedByName != "AAAAAA" {
		t.Fatalf("received: %+v %v", receivedInvitations, err)
	}
	if receivedInvitations, _ := database.ListReceivedShareInvitations(ctx, "c@example.com"); len(receivedInvitations) != 0 {
		t.Errorf("Carol received Bob's invitation: %+v", receivedInvitations)
	}
	sent, err := database.ListSentShareInvitations(ctx, "AAAAAA")
	if err != nil || len(sent) != 1 || sent[0].State != ShareInvitationPending || sent[0].Recipient != nil {
		t.Fatalf("sent: %+v %v", sent, err)
	}
	if sent, _ := database.ListSentShareInvitations(ctx, "BBBBBB"); len(sent) != 0 {
		t.Errorf("a non-owner sees sent invitations: %+v", sent)
	}

	if _, err := database.GrantAccess(ctx, vaultID, Member{AccountID: "BBBBBB", Role: "reader", WrappedKey: []byte{1}, GrantedBy: "AAAAAA"}, 0); !errors.Is(err, ErrNoInvitation) {
		t.Errorf("grant before acceptance: %v", err)
	}
	if _, _, err := database.AcceptShareInvitation(ctx, invitationID, "CCCCCC", "c@example.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("accept with another email: %v", err)
	}
	if waiting, err := database.ListWaitingShareInvitations(ctx, "BBBBBB"); err != nil || len(waiting) != 0 {
		t.Errorf("waiting before accepting: %+v %v", waiting, err)
	}
	acceptedVaultID, acceptedAt, err := database.AcceptShareInvitation(ctx, invitationID, "BBBBBB", "b@example.com")
	if err != nil || acceptedVaultID != vaultID || time.Since(acceptedAt) > time.Minute {
		t.Fatalf("accept: %s %v %v", acceptedVaultID, acceptedAt, err)
	}
	if _, _, err := database.AcceptShareInvitation(ctx, invitationID, "BBBBBB", "b@example.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("accept twice: %v", err)
	}
	if receivedInvitations, _ := database.ListReceivedShareInvitations(ctx, "b@example.com"); len(receivedInvitations) != 0 {
		t.Errorf("accepted invitation still received: %+v", receivedInvitations)
	}
	sent, _ = database.ListSentShareInvitations(ctx, "AAAAAA")
	if len(sent) != 1 || sent[0].State != ShareInvitationAccepted || sent[0].Recipient == nil || sent[0].Recipient.ID != "BBBBBB" || sent[0].Recipient.Email != "b@example.com" ||
		sent[0].AcceptedAt == nil || !sent[0].AcceptedAt.Equal(acceptedAt) {
		t.Fatalf("sent after accept: %+v", sent)
	}
	waiting, err := database.ListWaitingShareInvitations(ctx, "BBBBBB")
	if err != nil || len(waiting) != 1 || waiting[0].ID != invitationID || waiting[0].InvitedByEmail != "a@example.com" ||
		waiting[0].AcceptedAt == nil || !waiting[0].AcceptedAt.Equal(acceptedAt) {
		t.Fatalf("waiting after accept: %+v %v", waiting, err)
	}
	if waiting, _ := database.ListWaitingShareInvitations(ctx, "AAAAAA"); len(waiting) != 0 {
		t.Errorf("the owner waits for their own invitation: %+v", waiting)
	}
	acceptance, err := database.GetShareAcceptance(ctx, invitationID, acceptedAt)
	if err != nil || acceptance.VaultID != vaultID || acceptance.Inviter.Email != "a@example.com" || acceptance.Recipient.Email != "b@example.com" {
		t.Fatalf("acceptance: %+v %v", acceptance, err)
	}
	if _, err := database.GetShareAcceptance(ctx, invitationID, acceptedAt.Add(time.Second)); !errors.Is(err, ErrNotFound) {
		t.Errorf("acceptance from another accept: %v", err)
	}

	if _, err := database.GrantAccess(ctx, vaultID, Member{AccountID: "CCCCCC", Role: "reader", WrappedKey: []byte{1}, GrantedBy: "AAAAAA"}, 0); !errors.Is(err, ErrNoInvitation) {
		t.Errorf("grant to an account without an invitation: %v", err)
	}
	isNewMember, err := database.GrantAccess(ctx, vaultID, Member{AccountID: "BBBBBB", Role: "reader", WrappedKey: []byte{1}, GrantedBy: "AAAAAA"}, 0)
	if err != nil || !isNewMember {
		t.Fatalf("grant after acceptance: %v %v", isNewMember, err)
	}
	if sent, _ := database.ListSentShareInvitations(ctx, "AAAAAA"); len(sent) != 0 {
		t.Errorf("completed invitation still listed: %+v", sent)
	}
	if waiting, _ := database.ListWaitingShareInvitations(ctx, "BBBBBB"); len(waiting) != 0 {
		t.Errorf("completed invitation still waiting: %+v", waiting)
	}
	if _, err := database.GetShareAcceptance(ctx, invitationID, acceptedAt); !errors.Is(err, ErrNotFound) {
		t.Errorf("acceptance after completion: %v", err)
	}
	if isNewMember, err := database.GrantAccess(ctx, vaultID, Member{AccountID: "BBBBBB", Role: "editor", WrappedKey: []byte{2}, GrantedBy: "AAAAAA"}, 0); err != nil || isNewMember {
		t.Errorf("role change: %v %v", isNewMember, err)
	}
	if role, _ := database.GetVaultRole(ctx, vaultID, "BBBBBB"); role != "editor" {
		t.Errorf("role = %q", role)
	}
	if isMember, err := database.IsVaultMemberEmail(ctx, vaultID, "b@example.com"); err != nil || !isMember {
		t.Errorf("Bob is a member: %v %v", isMember, err)
	}
	if isMember, _ := database.IsVaultMemberEmail(ctx, vaultID, "c@example.com"); isMember {
		t.Error("Carol is not a member")
	}
}

func TestShareInvitationDeclineCancelAndExpiry(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	createTestAccount(t, database, "BBBBBB", "b@example.com")
	vaultID := createTestVault(t, database, "AAAAAA")

	invitationID, err := database.UpsertShareInvitation(ctx, &ShareInvitation{VaultID: vaultID, Email: "b@example.com", Role: "editor", InvitedBy: "AAAAAA"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.DeclineShareInvitation(ctx, invitationID, "a@example.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("decline by someone else: %v", err)
	}
	if declinedVaultID, err := database.DeclineShareInvitation(ctx, invitationID, "b@example.com"); err != nil || declinedVaultID != vaultID {
		t.Fatalf("decline: %s %v", declinedVaultID, err)
	}
	if sent, _ := database.ListSentShareInvitations(ctx, "AAAAAA"); len(sent) != 0 {
		t.Errorf("declined invitation still listed: %+v", sent)
	}

	invitationID, _ = database.UpsertShareInvitation(ctx, &ShareInvitation{VaultID: vaultID, Email: "nobody@example.com", Role: "reader", InvitedBy: "AAAAAA"}, time.Hour)
	if _, _, err := database.CancelShareInvitation(ctx, invitationID, "BBBBBB"); !errors.Is(err, ErrNotFound) {
		t.Errorf("cancel by a non-owner: %v", err)
	}
	cancelledVaultID, email, err := database.CancelShareInvitation(ctx, invitationID, "AAAAAA")
	if err != nil || cancelledVaultID != vaultID || email != "nobody@example.com" {
		t.Fatalf("cancel: %s %s %v", cancelledVaultID, email, err)
	}

	expiredID, _ := database.UpsertShareInvitation(ctx, &ShareInvitation{VaultID: vaultID, Email: "b@example.com", Role: "reader", InvitedBy: "AAAAAA"}, -time.Minute)
	if receivedInvitations, _ := database.ListReceivedShareInvitations(ctx, "b@example.com"); len(receivedInvitations) != 0 {
		t.Errorf("expired invitation received: %+v", receivedInvitations)
	}
	if sent, _ := database.ListSentShareInvitations(ctx, "AAAAAA"); len(sent) != 0 {
		t.Errorf("expired invitation sent: %+v", sent)
	}
	if _, _, err := database.AcceptShareInvitation(ctx, expiredID, "BBBBBB", "b@example.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("accept expired: %v", err)
	}
}

func TestListAllPersonOnlySharesVaults(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	createTestAccount(t, database, "BBBBBB", "b@example.com")
	createTestAccount(t, database, "CCCCCC", "c@example.com")
	createTestAccount(t, database, "DDDDDD", "d@example.com")
	firstVaultID := createTestVault(t, database, "AAAAAA")
	secondVaultID := createTestVault(t, database, "AAAAAA")
	if err := database.SetAccess(ctx, firstVaultID, Member{AccountID: "BBBBBB", Role: "reader", WrappedKey: []byte{1}, GrantedBy: "AAAAAA"}); err != nil {
		t.Fatal(err)
	}
	if err := database.SetAccess(ctx, secondVaultID, Member{AccountID: "CCCCCC", Role: "reader", WrappedKey: []byte{1}, GrantedBy: "AAAAAA"}); err != nil {
		t.Fatal(err)
	}

	listPersonIDs := func(accountID string) []string {
		people, err := database.ListPeople(ctx, accountID)
		if err != nil {
			t.Fatal(err)
		}
		var iDs []string
		for _, person := range people {
			iDs = append(iDs, person.ID)
		}
		slices.Sort(iDs)
		return iDs
	}
	if iDs := listPersonIDs("AAAAAA"); !slices.Equal(iDs, []string{"AAAAAA", "BBBBBB", "CCCCCC"}) {
		t.Errorf("Alice sees %v", iDs)
	}
	if iDs := listPersonIDs("BBBBBB"); !slices.Equal(iDs, []string{"AAAAAA", "BBBBBB"}) {
		t.Errorf("Bob sees %v", iDs)
	}
	if iDs := listPersonIDs("DDDDDD"); !slices.Equal(iDs, []string{"DDDDDD"}) {
		t.Errorf("Dave sees %v", iDs)
	}
}

func TestRotateVaultCannotAddMember(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	createTestAccount(t, database, "BBBBBB", "b@example.com")
	vaultID := createTestVault(t, database, "AAAAAA")
	rotation := VaultRotation{
		VaultID:   vaultID,
		EncName:   []byte("new-name"),
		Accesses:  []Member{{AccountID: "AAAAAA", Role: "owner", WrappedKey: []byte{9}}, {AccountID: "BBBBBB", Role: "editor", WrappedKey: []byte{9}}},
		RotatedBy: "AAAAAA",
	}
	if err := database.RotateVault(ctx, rotation); !errors.Is(err, ErrNotMember) {
		t.Fatalf("rotation adding Bob: %v", err)
	}
	if _, err := database.GetVaultRole(ctx, vaultID, "BBBBBB"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Bob got access: %v", err)
	}
	rotation.Accesses = rotation.Accesses[:1]
	if err := database.RotateVault(ctx, rotation); err != nil {
		t.Fatal(err)
	}
}
