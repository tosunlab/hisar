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

	allReceived, err := database.ListAllReceivedShareInvitation(ctx, "b@example.com")
	if err != nil || len(allReceived) != 1 || allReceived[0].Role != "reader" || allReceived[0].InvitedByEmail != "a@example.com" || allReceived[0].InvitedByName != "AAAAAA" {
		t.Fatalf("received: %+v %v", allReceived, err)
	}
	if allReceived, _ := database.ListAllReceivedShareInvitation(ctx, "c@example.com"); len(allReceived) != 0 {
		t.Errorf("Carol received Bob's invitation: %+v", allReceived)
	}
	allSent, err := database.ListAllSentShareInvitation(ctx, "AAAAAA")
	if err != nil || len(allSent) != 1 || allSent[0].State != ShareInvitationPending || allSent[0].Recipient != nil {
		t.Fatalf("sent: %+v %v", allSent, err)
	}
	if allSent, _ := database.ListAllSentShareInvitation(ctx, "BBBBBB"); len(allSent) != 0 {
		t.Errorf("a non-owner sees sent invitations: %+v", allSent)
	}

	if _, err := database.GrantAccess(ctx, vaultID, Member{AccountID: "BBBBBB", Role: "reader", WrappedKey: []byte{1}, GrantedBy: "AAAAAA"}); !errors.Is(err, ErrNoInvitation) {
		t.Errorf("grant before acceptance: %v", err)
	}
	if _, err := database.AcceptShareInvitation(ctx, invitationID, "CCCCCC", "c@example.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("accept with another email: %v", err)
	}
	acceptedVaultID, err := database.AcceptShareInvitation(ctx, invitationID, "BBBBBB", "b@example.com")
	if err != nil || acceptedVaultID != vaultID {
		t.Fatalf("accept: %s %v", acceptedVaultID, err)
	}
	if _, err := database.AcceptShareInvitation(ctx, invitationID, "BBBBBB", "b@example.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("accept twice: %v", err)
	}
	if allReceived, _ := database.ListAllReceivedShareInvitation(ctx, "b@example.com"); len(allReceived) != 0 {
		t.Errorf("accepted invitation still received: %+v", allReceived)
	}
	allSent, _ = database.ListAllSentShareInvitation(ctx, "AAAAAA")
	if len(allSent) != 1 || allSent[0].State != ShareInvitationAccepted || allSent[0].Recipient == nil || allSent[0].Recipient.ID != "BBBBBB" || allSent[0].Recipient.Email != "b@example.com" {
		t.Fatalf("sent after accept: %+v", allSent)
	}

	if _, err := database.GrantAccess(ctx, vaultID, Member{AccountID: "CCCCCC", Role: "reader", WrappedKey: []byte{1}, GrantedBy: "AAAAAA"}); !errors.Is(err, ErrNoInvitation) {
		t.Errorf("grant to an account without an invitation: %v", err)
	}
	isNewMember, err := database.GrantAccess(ctx, vaultID, Member{AccountID: "BBBBBB", Role: "reader", WrappedKey: []byte{1}, GrantedBy: "AAAAAA"})
	if err != nil || !isNewMember {
		t.Fatalf("grant after acceptance: %v %v", isNewMember, err)
	}
	if allSent, _ := database.ListAllSentShareInvitation(ctx, "AAAAAA"); len(allSent) != 0 {
		t.Errorf("completed invitation still listed: %+v", allSent)
	}
	if isNewMember, err := database.GrantAccess(ctx, vaultID, Member{AccountID: "BBBBBB", Role: "editor", WrappedKey: []byte{2}, GrantedBy: "AAAAAA"}); err != nil || isNewMember {
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
	if allSent, _ := database.ListAllSentShareInvitation(ctx, "AAAAAA"); len(allSent) != 0 {
		t.Errorf("declined invitation still listed: %+v", allSent)
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
	if allReceived, _ := database.ListAllReceivedShareInvitation(ctx, "b@example.com"); len(allReceived) != 0 {
		t.Errorf("expired invitation received: %+v", allReceived)
	}
	if allSent, _ := database.ListAllSentShareInvitation(ctx, "AAAAAA"); len(allSent) != 0 {
		t.Errorf("expired invitation sent: %+v", allSent)
	}
	if _, err := database.AcceptShareInvitation(ctx, expiredID, "BBBBBB", "b@example.com"); !errors.Is(err, ErrNotFound) {
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

	listAllPersonID := func(accountID string) []string {
		allPerson, err := database.ListAllPerson(ctx, accountID)
		if err != nil {
			t.Fatal(err)
		}
		var allID []string
		for _, person := range allPerson {
			allID = append(allID, person.ID)
		}
		slices.Sort(allID)
		return allID
	}
	if allID := listAllPersonID("AAAAAA"); !slices.Equal(allID, []string{"AAAAAA", "BBBBBB", "CCCCCC"}) {
		t.Errorf("Alice sees %v", allID)
	}
	if allID := listAllPersonID("BBBBBB"); !slices.Equal(allID, []string{"AAAAAA", "BBBBBB"}) {
		t.Errorf("Bob sees %v", allID)
	}
	if allID := listAllPersonID("DDDDDD"); !slices.Equal(allID, []string{"DDDDDD"}) {
		t.Errorf("Dave sees %v", allID)
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
		AllAccess: []Member{{AccountID: "AAAAAA", Role: "owner", WrappedKey: []byte{9}}, {AccountID: "BBBBBB", Role: "editor", WrappedKey: []byte{9}}},
		RotatedBy: "AAAAAA",
	}
	if err := database.RotateVault(ctx, rotation); !errors.Is(err, ErrNotMember) {
		t.Fatalf("rotation adding Bob: %v", err)
	}
	if _, err := database.GetVaultRole(ctx, vaultID, "BBBBBB"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Bob got access: %v", err)
	}
	rotation.AllAccess = rotation.AllAccess[:1]
	if err := database.RotateVault(ctx, rotation); err != nil {
		t.Fatal(err)
	}
}
