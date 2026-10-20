package app

import (
	"context"
	"errors"
	"testing"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/clisocket"
)

func TestStaleCommandMutationsAreDeniedByTheOperation(t *testing.T) {
	s := createUnlockedTestService(t).service
	bridge := NewCommandLineBridge(s, clisocket.NewSessionApprovals(0, nil, nil))
	ctx := bridge.BindSession(context.Background())
	epoch := ctx.Value(sessionEpochKey{}).(uint64)
	signInOtherAccount(t, s, createUnlockedTestService(t))
	requests := []cliprotocol.Request{
		{Type: cliprotocol.TypeCreateVault, Name: "stale"},
		{Type: cliprotocol.TypeRenameVault, VaultID: "vault", Name: "stale"},
		{Type: cliprotocol.TypeDeleteVault, VaultID: "vault", VaultDeletion: &cliprotocol.VaultDeletion{}},
		{Type: cliprotocol.TypeLeaveVault, VaultID: "vault"},
		{Type: cliprotocol.TypeSaveItem, VaultID: "vault", Item: &cliprotocol.ItemChange{Title: "stale", Category: "login"}},
		{Type: cliprotocol.TypeDeleteItem, VaultID: "vault", ItemID: "item"},
		{Type: cliprotocol.TypeImportItem, VaultID: "vault"},
		{Type: cliprotocol.TypeAddFavorite, VaultID: "vault", ItemID: "item"},
		{Type: cliprotocol.TypeRemoveFavorite, VaultID: "vault", ItemID: "item"},
		{Type: cliprotocol.TypeShareVault, VaultID: "vault", Email: "a@example.com", Role: "reader"},
		{Type: cliprotocol.TypeUnshareVault, VaultID: "vault", AccountID: "account"},
		{Type: cliprotocol.TypeInvite, Email: "a@example.com"},
		{Type: cliprotocol.TypeAcceptInvitation, InvitationID: "invitation"},
		{Type: cliprotocol.TypeDeclineInvitation, InvitationID: "invitation"},
		{Type: cliprotocol.TypeCancelInvitation, InvitationID: "invitation"},
		{Type: cliprotocol.TypeRevokeDevice, DeviceID: "device"},
		{Type: cliprotocol.TypeCreateShareLink, ShareLink: &cliprotocol.ShareLinkChange{Text: "stale", Title: "stale", ExpiresInSeconds: 3600, MaxViews: 1}},
		{Type: cliprotocol.TypeRevokeShareLink, ShareLinkID: "link"},
		{Type: cliprotocol.TypeCreateOrganization, Name: "stale"},
		{Type: cliprotocol.TypeRenameOrganization, OrganizationID: "org", Name: "stale"},
		{Type: cliprotocol.TypeDeleteOrganization, OrganizationID: "org"},
		{Type: cliprotocol.TypeInviteToOrganization, OrganizationID: "org", Email: "a@example.com", Role: "member"},
		{Type: cliprotocol.TypeCancelOrganizationInvitation, OrganizationID: "org", InvitationID: "invitation"},
		{Type: cliprotocol.TypeAcceptOrganizationInvitation, InvitationID: "invitation"},
		{Type: cliprotocol.TypeDeclineOrganizationInvitation, InvitationID: "invitation"},
		{Type: cliprotocol.TypeSetOrganizationRole, OrganizationID: "org", AccountID: "account", Role: "member"},
		{Type: cliprotocol.TypeRemoveOrganizationMember, OrganizationID: "org", AccountID: "account"},
		{Type: cliprotocol.TypeLeaveOrganization, OrganizationID: "org"},
		{Type: cliprotocol.TypeCompleteOrganizationKeys},
		{Type: cliprotocol.TypeCreateTeam, OrganizationID: "org", Name: "stale"},
		{Type: cliprotocol.TypeRenameTeam, OrganizationID: "org", TeamID: "team", Name: "stale"},
		{Type: cliprotocol.TypeDeleteTeam, OrganizationID: "org", TeamID: "team"},
		{Type: cliprotocol.TypeAddTeamMember, OrganizationID: "org", TeamID: "team", AccountID: "account"},
		{Type: cliprotocol.TypeRemoveTeamMember, OrganizationID: "org", TeamID: "team", AccountID: "account"},
		{Type: cliprotocol.TypeGiveVaultToTeam, OrganizationID: "org", TeamID: "team", VaultID: "vault", Role: "reader"},
		{Type: cliprotocol.TypeTakeVaultFromTeam, OrganizationID: "org", TeamID: "team", VaultID: "vault"},
		{Type: cliprotocol.TypeCreateOrganizationVault, OrganizationID: "org", TeamID: "team", Name: "stale"},
		{Type: cliprotocol.TypeLauncherLock},
	}
	for _, request := range requests {
		t.Run(request.Type, func(t *testing.T) {
			if _, err := bridge.answer(ctx, epoch, &request); !errors.Is(err, errCommandLineSessionEnded) {
				t.Fatalf("stale operation error = %v, want session denial", err)
			}
		})
	}
	if s.GetStatus().State != StateUnlocked {
		t.Fatal("stale launcher lock locked the replacement session")
	}
}
