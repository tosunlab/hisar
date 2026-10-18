package hisarkit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/session"
)

type organizationList struct {
	Organizations []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Role        string `json:"role"`
		MemberCount int    `json:"memberCount"`
	} `json:"organizations"`
	Invitations []struct {
		ID               string `json:"id"`
		OrganizationID   string `json:"organizationId"`
		OrganizationName string `json:"organizationName"`
		Role             string `json:"role"`
		InvitedByEmail   string `json:"invitedByEmail"`
	} `json:"invitations"`
}

type organizationPerson struct {
	AccountID string `json:"accountId"`
	Email     string `json:"email"`
	Role      string `json:"role"`
}

type organizationDetail struct {
	Members []organizationPerson `json:"members"`
	Teams   []struct {
		ID      string               `json:"id"`
		Name    string               `json:"name"`
		Members []organizationPerson `json:"members"`
		Vaults  []struct {
			VaultID string `json:"vaultId"`
			Name    string `json:"name"`
			Role    string `json:"role"`
			CanOpen bool   `json:"canOpen"`
		} `json:"vaults"`
	} `json:"teams"`
	Invitations []struct {
		Email string `json:"email"`
	} `json:"invitations"`
}

func findOrganizationPerson(t *testing.T, people []organizationPerson, email string) organizationPerson {
	t.Helper()
	index := slices.IndexFunc(people, func(person organizationPerson) bool { return person.Email == email })
	if index < 0 {
		t.Fatalf("no %s in %+v", email, people)
	}
	return people[index]
}

// getServerVault reads a vault as the server lists it to the account signed in from stateDirectory.
func getServerVault(t *testing.T, stateDirectory, vaultID string) apitypes.Vault {
	t.Helper()
	state, err := localstate.Load(filepath.Join(stateDirectory, "hisar.json"))
	if err != nil {
		t.Fatal(err)
	}
	client := apiclient.New(state.Server)
	client.Token = state.Token
	vaults, err := client.ListVaults(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	index := slices.IndexFunc(vaults, func(vault apitypes.Vault) bool { return vault.ID == vaultID })
	if index < 0 {
		t.Fatalf("the server does not list vault %s", vaultID)
	}
	return vaults[index]
}

// TestIntegrationMobileOrganizations runs against a real server from scripts/test-end-to-end.sh: an admin iPhone
// creates an organization and a team vault, a member iPhone reads it through the team and loses it when removed, and
// the admin's app rotates the key at once.
func TestIntegrationMobileOrganizations(t *testing.T) {
	adminInviteLink := os.Getenv("HISAR_TEST_MOBILE_ORGANIZATIONS_ADMIN_INVITE_LINK")
	memberInviteLink := os.Getenv("HISAR_TEST_MOBILE_ORGANIZATIONS_MEMBER_INVITE_LINK")
	if adminInviteLink == "" || memberInviteLink == "" {
		t.Skip("HISAR_TEST_MOBILE_ORGANIZATIONS_ADMIN_INVITE_LINK and HISAR_TEST_MOBILE_ORGANIZATIONS_MEMBER_INVITE_LINK not set")
	}
	const adminEmail, memberEmail = "mobile-organizations-admin@example.com", "mobile-organizations-member@example.com"
	const withoutAccountEmail = "nobody-mobile-organizations@example.com"
	adminDirectory := t.TempDir()
	admin := signUpMobile(t, adminDirectory, adminInviteLink, adminEmail, "Mobile Admin", createRemoteChangeRecorder())
	memberRecorder := createRemoteChangeRecorder()
	member := signUpMobile(t, t.TempDir(), memberInviteLink, memberEmail, "Mobile Member", memberRecorder)

	created := decodeJSON[struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Role        string `json:"role"`
		MemberCount int    `json:"memberCount"`
	}](t)(admin.CreateOrganization("Acme"))
	if created.ID == "" || created.Name != "Acme" || created.Role != "admin" || created.MemberCount != 1 {
		t.Fatalf("created organization: %+v", created)
	}
	if err := admin.InviteToOrganization(created.ID, memberEmail, "member"); err != nil {
		t.Fatalf("invite an account: %v", err)
	}
	if err := admin.InviteToOrganization(created.ID, withoutAccountEmail, "member"); err != nil {
		t.Fatalf("invite an address without an account: %v", err)
	}

	var invitationID string
	memberRecorder.waitUntil(t, "the member sees the organization invitation", func() bool {
		invitations := decodeJSON[organizationList](t)(member.ListAllOrganization()).Invitations
		if len(invitations) != 1 {
			return false
		}
		if invitation := invitations[0]; invitation.OrganizationID != created.ID || invitation.OrganizationName != "Acme" || invitation.Role != "member" || invitation.InvitedByEmail != adminEmail {
			t.Fatalf("invitation = %+v", invitation)
		}
		invitationID = invitations[0].ID
		return true
	})
	if err := member.AcceptOrganizationInvitation(invitationID); err != nil {
		t.Fatal(err)
	}
	listed := decodeJSON[organizationList](t)(member.ListAllOrganization())
	if len(listed.Invitations) != 0 || len(listed.Organizations) != 1 {
		t.Fatalf("organizations after accepting: %+v", listed)
	}
	if organization := listed.Organizations[0]; organization.ID != created.ID || organization.Role != "member" || organization.MemberCount != 2 {
		t.Fatalf("organization after accepting: %+v", organization)
	}

	team := decodeJSON[struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}](t)(admin.CreateTeam(created.ID, "Engineering"))
	if team.ID == "" || team.Name != "Engineering" {
		t.Fatalf("team: %+v", team)
	}
	people := decodeJSON[organizationDetail](t)(admin.GetOrganization(created.ID)).Members
	adminAccountID := findOrganizationPerson(t, people, adminEmail).AccountID
	memberAccountID := findOrganizationPerson(t, people, memberEmail).AccountID
	if err := admin.AddTeamMember(created.ID, team.ID, adminAccountID); err != nil {
		t.Fatal(err)
	}
	vault := decodeJSON[sharingVaultSummary](t)(admin.CreateOrganizationVault(created.ID, team.ID, "Engineering secrets"))
	if vault.ID == "" || vault.Name != "Engineering secrets" || vault.Type != "organization" || vault.Role != "owner" {
		t.Fatalf("organization vault: %+v", vault)
	}
	saved := decodeJSON[struct {
		ID string `json:"id"`
	}](t)(admin.SaveItem(vault.ID, `{"title":"Staging database","username":"deploy","password":"staging-database-password","urls":[],"fields":[]}`))

	memberRecorder.drain()
	if err := admin.AddTeamMember(created.ID, team.ID, memberAccountID); err != nil {
		t.Fatal(err)
	}
	var memberVault *sharingVaultSummary
	memberRecorder.waitUntil(t, "the member sees the organization vault", func() bool {
		memberVault = findVaultByName(t, member, "Engineering secrets")
		return memberVault != nil
	})
	if memberVault.ID != vault.ID || memberVault.Type != "organization" {
		t.Fatalf("the member's vault = %+v", memberVault)
	}
	if item := decodeJSON[struct {
		Password string `json:"password"`
	}](t)(member.GetItem(vault.ID, saved.ID)); item.Password != "staging-database-password" {
		t.Errorf("the member reads %q", item.Password)
	}

	for _, side := range []struct {
		name  string
		hisar *Hisar
	}{{"admin", admin}, {"member", member}} {
		organization := decodeJSON[organizationDetail](t)(side.hisar.GetOrganization(created.ID))
		if len(organization.Teams) != 1 || organization.Teams[0].ID != team.ID || organization.Teams[0].Name != "Engineering" {
			t.Fatalf("the %s's teams: %+v", side.name, organization.Teams)
		}
		teamMembers := organization.Teams[0].Members
		if len(teamMembers) != 2 || findOrganizationPerson(t, teamMembers, adminEmail).AccountID != adminAccountID || findOrganizationPerson(t, teamMembers, memberEmail).AccountID != memberAccountID {
			t.Errorf("the %s's team members: %+v", side.name, teamMembers)
		}
		teamVaults := organization.Teams[0].Vaults
		if len(teamVaults) != 1 || teamVaults[0].VaultID != vault.ID || teamVaults[0].Name != "Engineering secrets" || teamVaults[0].Role != "owner" || !teamVaults[0].CanOpen {
			t.Errorf("the %s's team vaults: %+v", side.name, teamVaults)
		}
	}
	if invitations := decodeJSON[organizationDetail](t)(admin.GetOrganization(created.ID)).Invitations; len(invitations) != 1 || invitations[0].Email != withoutAccountEmail {
		t.Errorf("the admin's pending invitations: %+v", invitations)
	}

	if actions := decodeJSON[struct {
		CanLeave           bool   `json:"canLeave"`
		LeaveBlockedReason string `json:"leaveBlockedReason"`
	}](t)(member.GetVaultActions(vault.ID)); actions.CanLeave || actions.LeaveBlockedReason != session.OrganizationVaultLeaveReason {
		t.Errorf("the member's vault actions: %+v", actions)
	}
	if err := member.LeaveVault(vault.ID); !errors.Is(err, session.ErrOrganizationVaultAccess) {
		t.Fatalf("leave the organization vault: %v", err)
	}

	memberRecorder.drain()
	if err := admin.RemoveTeamMember(created.ID, team.ID, memberAccountID); err != nil {
		t.Fatal(err)
	}
	if serverVault := getServerVault(t, adminDirectory, vault.ID); serverVault.KeyGeneration != 2 || serverVault.NeedsRotation {
		t.Fatalf("the admin's app did not rotate the key: generation %d, needs rotation %t", serverVault.KeyGeneration, serverVault.NeedsRotation)
	}
	if keys := decodeJSON[struct {
		Waiting       []struct{} `json:"waiting"`
		DeliveryError string     `json:"deliveryError"`
		RotationError string     `json:"rotationError"`
	}](t)(admin.ListAllWaitingVaultKey()); len(keys.Waiting) != 0 || keys.DeliveryError != "" || keys.RotationError != "" {
		t.Errorf("the admin's waiting vault keys: %+v", keys)
	}
	memberRecorder.waitUntil(t, "the member loses the organization vault", func() bool {
		return findVaultByName(t, member, "Engineering secrets") == nil
	})
	if item := decodeJSON[struct {
		Password string `json:"password"`
	}](t)(admin.GetItem(vault.ID, saved.ID)); item.Password != "staging-database-password" {
		t.Errorf("the admin reads %q after the key rotation", item.Password)
	}
}
