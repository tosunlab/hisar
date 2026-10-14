package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
)

func TestGetVaultSharingCompletesAcceptedSharesAndListsTheVaultOnly(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	bob, _ := test.apiServer.addMember(t, "Bob", "b@example.com")
	travelID := createSharedTestVault(t, service, "Travel")

	if err := service.ShareVault(test.familyID, "b@example.com", apitypes.VaultEditor); err != nil {
		t.Fatal(err)
	}
	if err := service.ShareVault(test.familyID, "nobody@example.com", apitypes.VaultReader); err != nil {
		t.Fatal(err)
	}
	if err := service.ShareVault(travelID, "carol@example.com", apitypes.VaultReader); err != nil {
		t.Fatal(err)
	}
	sharing, err := GetVaultSharing(service, test.familyID)
	if err != nil {
		t.Fatal(err)
	}
	if sharing.Vault.ID != test.familyID || sharing.Vault.Name != "Family" || !sharing.IsOwner || len(sharing.AllMember) != 1 || !sharing.AllMember[0].IsCurrentUser {
		t.Fatalf("before anyone accepted: %+v", sharing)
	}
	if len(sharing.AllInvitation) != 2 || sharing.AllInvitation[0].VaultName != "Family" || sharing.AllInvitation[0].State != apitypes.ShareInvitationPending {
		t.Fatalf("Family's invitations: %+v", sharing.AllInvitation)
	}

	test.apiServer.acceptShareInvitation(t, bob)
	sharing, err = GetVaultSharing(service, test.familyID)
	if err != nil || sharing.CompletionError != "" {
		t.Fatalf("after Bob accepted: %+v %v", sharing, err)
	}
	if access, found := test.apiServer.getAccess(test.familyID, bob.AccountID); !found || access.Role != apitypes.VaultEditor {
		t.Fatalf("Bob's access after opening the sharing screen: %+v %v", access, found)
	}
	if len(sharing.AllMember) != 2 || sharing.AllMember[1].Email != "b@example.com" || sharing.AllMember[1].Role != apitypes.VaultEditor ||
		sharing.AllMember[1].SecurityCode != crypt.FormatKeyFingerprint(bob.PublicKeys.Sign) {
		t.Fatalf("members after completing: %+v", sharing.AllMember)
	}
	if len(sharing.AllInvitation) != 1 || sharing.AllInvitation[0].Email != "nobody@example.com" {
		t.Errorf("invitations after completing: %+v", sharing.AllInvitation)
	}
}

func TestGetVaultSharingShowsARefusedChangedKeyOnlyForItsVault(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	bob, _ := test.apiServer.addMember(t, "Bob", "b@example.com")
	travelID := createSharedTestVault(t, service, "Travel")
	shareThroughInvitation(t, test, test.familyID, bob, apitypes.VaultReader)

	test.apiServer.replaceMemberKey(t, bob.AccountID)
	if err := service.ShareVault(travelID, "b@example.com", apitypes.VaultReader); err != nil {
		t.Fatal(err)
	}
	test.apiServer.acceptShareInvitation(t, *test.apiServer.findMember(bob.AccountID))
	travel, err := GetVaultSharing(service, travelID)
	if err != nil || !strings.Contains(travel.CompletionError, "signing key of b@example.com changed") {
		t.Fatalf("Travel after a key change: %+v %v", travel, err)
	}
	if len(travel.AllInvitation) != 1 || travel.AllInvitation[0].State != apitypes.ShareInvitationAccepted {
		t.Errorf("the refused invitation should stay accepted: %+v", travel.AllInvitation)
	}
	if _, found := test.apiServer.getAccess(travelID, bob.AccountID); found {
		t.Error("the vault key was wrapped to a substituted key")
	}
	family, err := GetVaultSharing(service, test.familyID)
	if err != nil || family.CompletionError != "" {
		t.Errorf("Family shows Travel's error: %+v %v", family, err)
	}
	if !family.AllMember[1].IsKeyChanged {
		t.Errorf("Bob's changed key is not marked: %+v", family.AllMember)
	}
}

func TestGetVaultSharingRefusesPersonalVaultAndHidesInvitationsFromNonOwners(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	if _, err := GetVaultSharing(service, test.personalID); !errors.Is(err, errPersonalVault) {
		t.Errorf("personal vault: %v", err)
	}
	if _, err := GetVaultSharing(service, "no-such-vault"); err == nil {
		t.Error("an unknown vault")
	}
	if err := service.ShareVault(test.familyID, "b@example.com", apitypes.VaultReader); err != nil {
		t.Fatal(err)
	}
	test.apiServer.setVaultRole(test.familyID, apitypes.VaultEditor)
	if _, err := service.ListAllVault(); err != nil {
		t.Fatal(err)
	}
	sharing, err := GetVaultSharing(service, test.familyID)
	if err != nil || sharing.IsOwner || len(sharing.AllInvitation) != 0 || len(sharing.AllMember) != 1 {
		t.Errorf("as an editor: %+v %v", sharing, err)
	}
	service.Lock()
	if _, err := GetVaultSharing(service, test.familyID); err == nil {
		t.Error("sharing read while locked")
	}
}

func TestSelectCompletionError(t *testing.T) {
	completionError := "b@example.com: the signing key of b@example.com changed since you last shared with them; stopping\nc@example.com: invalid signature"
	if got := selectCompletionError(completionError, []string{"C@example.com"}); got != "c@example.com: invalid signature" {
		t.Errorf("one recipient: %q", got)
	}
	if got := selectCompletionError(completionError, nil); got != "" {
		t.Errorf("no accepted recipients: %q", got)
	}
	if got := selectCompletionError("", []string{"b@example.com"}); got != "" {
		t.Errorf("no error: %q", got)
	}
}
