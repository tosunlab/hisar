package main

import (
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/cliprotocol"
)

// answerAccountRequest adds people, sharing, invitations, devices and codes to the vault data the fake app answers.
func answerAccountRequest(request cliprotocol.Request) (any, *cliprotocol.ResponseError) {
	switch request.Type {
	case cliprotocol.TypeListPeople:
		return cliprotocol.PeopleListResult{People: []cliprotocol.Person{
			{AccountID: "alice-id", Name: "Alice", Email: "alice@example.com", Role: "admin", SecurityCode: "1111 2222", IsCurrentUser: true},
			{AccountID: "ada-id", Name: "Ada", Email: "ada@example.com", Role: "member", SecurityCode: "3333 4444"},
		}}, nil
	case cliprotocol.TypeShareVault, cliprotocol.TypeUnshareVault, cliprotocol.TypeInvite, cliprotocol.TypeAcceptInvitation,
		cliprotocol.TypeDeleteItem, cliprotocol.TypeRevokeDevice:
		return struct{}{}, nil
	case cliprotocol.TypeListInvitation:
		return cliprotocol.InvitationListResult{Received: []cliprotocol.ReceivedInvitation{{ID: "invitation-1234567890", VaultID: "family-id", Role: "editor", InvitedByName: "Bob", InvitedByEmail: "bob@example.com"}}}, nil
	case cliprotocol.TypeListDevice:
		return cliprotocol.DeviceListResult{Devices: []cliprotocol.Device{{ID: "device-id", Name: "MacBook", Platform: "darwin/desktop", LastSeenAt: "2026-01-02T03:04:05Z", IsCurrent: true}}}, nil
	case cliprotocol.TypeGetItem:
		if request.ItemID == "github-id" {
			return cliprotocol.ItemResult{Item: cliprotocol.Item{ID: "github-id", Revision: "r1", Title: "GitHub", Username: "alice"}}, nil
		}
	case cliprotocol.TypeGetCode:
		if request.ItemID == "github-id" {
			return cliprotocol.CodeResult{Code: "123456"}, nil
		}
		return nil, cliprotocol.NewError(cliprotocol.ErrorNotFound, "no one-time password")
	}
	return answerVaultDataRequest(request)
}

func startAccountApp(t *testing.T) *fakeApp {
	t.Helper()
	app := startFakeApp(t)
	app.mutex.Lock()
	app.answer = answerAccountRequest
	app.mutex.Unlock()
	return app
}

func TestItemCommandsThroughApp(t *testing.T) {
	app := startAccountApp(t)
	if stdout, _, err := runCapturingOutput(t, false, "", "get", "github", "-f", "code"); err != nil || strings.TrimSpace(stdout) != "123456" {
		t.Errorf("code: %q %v", stdout, err)
	}
	if _, _, err := runCapturingOutput(t, false, "", "get", "shop", "-v", "Work", "-f", "code"); err == nil || !strings.Contains(err.Error(), `item has no field "code"`) {
		t.Errorf("code of an item without one: %v", err)
	}
	if _, stderr, err := runCapturingOutput(t, false, "y\n", "rm", "github"); err != nil || !strings.Contains(stderr, `Deleted "GitHub".`) {
		t.Fatalf("rm: %v %s", err, stderr)
	}
	if deletions := app.findRequest(cliprotocol.TypeDeleteItem); len(deletions) != 1 || deletions[0].VaultID != "personal-id" || deletions[0].ItemID != "github-id" {
		t.Errorf("delete requests: %+v", deletions)
	}
	if stdout, _, err := runCapturingOutput(t, false, "", "vaults"); err != nil || !strings.Contains(stdout, "work-id") || !strings.HasPrefix(stdout, "NAME") {
		t.Errorf("vaults: %v\n%s", err, stdout)
	}
}

func TestSharingCommandsThroughApp(t *testing.T) {
	app := startAccountApp(t)
	if stdout, _, err := runCapturingOutput(t, false, "", "people"); err != nil || !strings.Contains(stdout, "ada@example.com") || !strings.Contains(stdout, "3333 4444") {
		t.Errorf("people: %v\n%s", err, stdout)
	}
	if _, stderr, err := runCapturingOutput(t, false, "", "share", "Work", "ada@example.com", "-r", "reader"); err != nil || !strings.Contains(stderr, "the Hisar app then finishes sharing.") {
		t.Errorf("share: %v %s", err, stderr)
	}
	if shares := app.findRequest(cliprotocol.TypeShareVault); len(shares) != 1 || shares[0].VaultID != "work-id" || shares[0].Email != "ada@example.com" || shares[0].Role != "reader" {
		t.Errorf("share requests: %+v", shares)
	}
	if _, _, err := runCapturingOutput(t, false, "", "share", "Personal", "ada@example.com"); err != errPersonalVaultShare {
		t.Errorf("sharing the personal vault: %v", err)
	}
	if _, stderr, err := runCapturingOutput(t, false, "", "unshare", "Work", "ADA@example.com"); err != nil || !strings.Contains(stderr, `Removed ada@example.com from "Work" and rotated the vault key.`) {
		t.Errorf("unshare: %v %s", err, stderr)
	}
	if unshares := app.findRequest(cliprotocol.TypeUnshareVault); len(unshares) != 1 || unshares[0].AccountID != "ada-id" {
		t.Errorf("unshare requests: %+v", unshares)
	}
	if _, _, err := runCapturingOutput(t, false, "", "unshare", "Work", "nobody@example.com"); err == nil || !strings.Contains(err.Error(), `member "nobody@example.com": not found`) {
		t.Errorf("unshare of a stranger: %v", err)
	}
	if _, stderr, err := runCapturingOutput(t, false, "", "invite", " carol@example.com "); err != nil || !strings.Contains(stderr, "Invitation sent to carol@example.com.") {
		t.Errorf("invite: %v %s", err, stderr)
	}
	if stdout, _, err := runCapturingOutput(t, false, "", "invitations"); err != nil || !strings.Contains(stdout, "bob@example.com") {
		t.Errorf("invitations: %v\n%s", err, stdout)
	}
	if _, _, err := runCapturingOutput(t, false, "", "invitations", "accept", "invitation-1234567890"); err != nil {
		t.Errorf("accept: %v", err)
	}
	if accepts := app.findRequest(cliprotocol.TypeAcceptInvitation); len(accepts) != 1 || accepts[0].InvitationID != "invitation-1234567890" {
		t.Errorf("accept requests: %+v", accepts)
	}
}

func TestDeviceCommandsThroughApp(t *testing.T) {
	app := startAccountApp(t)
	if stdout, _, err := runCapturingOutput(t, false, "", "devices"); err != nil || !strings.Contains(stdout, "MacBook (this device)") || !strings.Contains(stdout, "device-id") {
		t.Errorf("devices: %v\n%s", err, stdout)
	}
	if _, stderr, err := runCapturingOutput(t, false, "", "revoke-device", "device-id"); err != nil || !strings.Contains(stderr, "Device signed out.") {
		t.Errorf("revoke-device: %v %s", err, stderr)
	}
	if revokes := app.findRequest(cliprotocol.TypeRevokeDevice); len(revokes) != 1 || revokes[0].DeviceID != "device-id" {
		t.Errorf("revoke requests: %+v", revokes)
	}
}
