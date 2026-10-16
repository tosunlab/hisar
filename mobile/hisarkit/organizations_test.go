package hisarkit

import (
	"errors"
	"testing"

	"github.com/tryhisar/hisar/core/app"
)

func TestOrganizationsWhileSignedOut(t *testing.T) {
	hisar, err := New(t.TempDir(), "iPhone", createMemorySecretKeyStore(), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	checkJSON := func(name string, call func() (string, error)) {
		t.Helper()
		encoded, err := call()
		if encoded != "" || !errors.Is(err, app.ErrSignedOut) {
			t.Errorf("%s while signed out: %q %v", name, encoded, err)
		}
	}
	check := func(name string, err error) {
		t.Helper()
		if !errors.Is(err, app.ErrSignedOut) {
			t.Errorf("%s while signed out: %v", name, err)
		}
	}

	checkJSON("ListAllOrganization", hisar.ListAllOrganization)
	checkJSON("GetOrganization", func() (string, error) { return hisar.GetOrganization("organization") })
	checkJSON("CreateOrganization", func() (string, error) { return hisar.CreateOrganization("Acme") })
	check("RenameOrganization", hisar.RenameOrganization("organization", "Acme"))
	check("DeleteOrganization", hisar.DeleteOrganization("organization"))
	check("InviteToOrganization", hisar.InviteToOrganization("organization", "member@example.com", "member"))
	check("CancelOrganizationInvitation", hisar.CancelOrganizationInvitation("organization", "invitation"))
	check("AcceptOrganizationInvitation", hisar.AcceptOrganizationInvitation("invitation"))
	check("DeclineOrganizationInvitation", hisar.DeclineOrganizationInvitation("invitation"))
	check("SetOrganizationMemberRole", hisar.SetOrganizationMemberRole("organization", "account", "admin"))
	check("RemoveOrganizationMember", hisar.RemoveOrganizationMember("organization", "account"))
	check("LeaveOrganization", hisar.LeaveOrganization("organization"))
	checkJSON("CreateTeam", func() (string, error) { return hisar.CreateTeam("organization", "Engineering") })
	check("RenameTeam", hisar.RenameTeam("organization", "team", "Engineering"))
	check("DeleteTeam", hisar.DeleteTeam("organization", "team"))
	check("AddTeamMember", hisar.AddTeamMember("organization", "team", "account"))
	check("RemoveTeamMember", hisar.RemoveTeamMember("organization", "team", "account"))
	check("GiveVaultToTeam", hisar.GiveVaultToTeam("organization", "team", "vault", "reader"))
	check("TakeVaultFromTeam", hisar.TakeVaultFromTeam("organization", "team", "vault"))
	checkJSON("CreateOrganizationVault", func() (string, error) {
		return hisar.CreateOrganizationVault("organization", "team", "Engineering secrets")
	})
	checkJSON("ListAllWaitingVaultKey", hisar.ListAllWaitingVaultKey)
}
