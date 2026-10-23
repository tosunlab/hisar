package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/cliprotocol"
)

const hostileDisplayText = "Eve\x1b]52;c;ZWNobyBoaQ==\x07\u009b"
const escapedDisplayText = `Eve\x1b]52;c;ZWNobyBoaQ==\x07\x9b`

func TestCommandMessagesEscapeBackendTextOnStandardError(t *testing.T) {
	app := startFakeApp(t)
	app.mutex.Lock()
	app.answer = func(request cliprotocol.Request) (any, *cliprotocol.ResponseError) {
		switch request.Type {
		case cliprotocol.TypeListOrganization:
			result, refusal := answerOrganizationRequest(request)
			list := result.(cliprotocol.OrganizationListResult)
			list.Invitations[0].OrganizationName = hostileDisplayText
			return list, refusal
		case cliprotocol.TypeGetOrganization:
			result, refusal := answerOrganizationRequest(request)
			organization := result.(cliprotocol.OrganizationResult)
			organization.Organization.Name = hostileDisplayText
			return organization, refusal
		case cliprotocol.TypeCompleteOrganizationKeys:
			return cliprotocol.OrganizationKeysResult{
				DeliveryError: hostileDisplayText, RotationError: hostileDisplayText,
				Waiting: []cliprotocol.WaitingVaultKey{{OrganizationName: hostileDisplayText}},
			}, nil
		case cliprotocol.TypeListVault, cliprotocol.TypeReadVault:
			return cliprotocol.VaultListResult{Vaults: []cliprotocol.Vault{
				{ID: "personal-id", Name: "Personal", Type: "personal", Role: "owner"},
				{ID: "work-id", Name: hostileDisplayText, Type: "shared", Role: "owner"},
			}}, nil
		case cliprotocol.TypeGetVaultActions:
			return cliprotocol.VaultActionsResult{
				ImportScope: "reviewed-display-scope",
				Vault:       cliprotocol.Vault{ID: "work-id", Name: hostileDisplayText}, CanDelete: true,
				MembersLosingAccess: []cliprotocol.VaultMember{{Name: hostileDisplayText, Email: "eve@example.com", Role: "editor"}},
			}, nil
		case cliprotocol.TypeImportItem:
			return cliprotocol.ImportResult{Failures: []cliprotocol.ImportFailure{{Title: "New item", Message: hostileDisplayText}}}, nil
		}
		if result, refusal := answerOrganizationRequest(request); refusal == nil {
			return result, nil
		}
		return answerVaultDataRequest(request)
	}
	app.mutex.Unlock()
	setInteractiveTerminal(t, true)
	path := filepath.Join(t.TempDir(), hostileDisplayText+".csv")
	if err := os.WriteFile(path, []byte("name,url,username,password,note\nNew item,,alice,password,\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name        string
		args        []string
		input       string
		requestType string
		messages    []string
	}{
		{"organization deletion", []string{"org", "delete", "Acme", "-y"}, "", cliprotocol.TypeDeleteOrganization,
			[]string{"Deleted the organization " + escapedDisplayText + ".\n"}},
		{"organization invitation", []string{"org", "invite", "Acme", "new@example.com"}, "", cliprotocol.TypeInviteToOrganization,
			[]string{"Invitation sent to new@example.com to join " + escapedDisplayText + " as member."}},
		{"invitation acceptance", []string{"org", "accept", "f0e1"}, "", cliprotocol.TypeAcceptOrganizationInvitation,
			[]string{"You joined " + escapedDisplayText + " as a member."}},
		{"team creation", []string{"team", "create", "Acme", "Ops"}, "", cliprotocol.TypeCreateTeam,
			[]string{`Created team "Ops" in ` + escapedDisplayText + "."}},
		{"organization key notices", []string{"orgs"}, "", cliprotocol.TypeCompleteOrganizationKeys,
			[]string{"Could not give every vault key: " + escapedDisplayText, "Could not rotate every vault key: " + escapedDisplayText,
				"Waiting for an owner of a vault in " + escapedDisplayText + " to give you its key."}},
		{"vault deletion and member preview", []string{"vault", "delete", "work-id", "-y"}, "", cliprotocol.TypeDeleteVault,
			[]string{escapedDisplayText + " has 0 items.", "These people lose access:\n  " + escapedDisplayText + " <eve@example.com>, editor", "Deleted " + escapedDisplayText + "."}},
		{"prompt default", []string{"vault", "rename", "work-id"}, "New name\n", cliprotocol.TypeRenameVault,
			[]string{"New name [" + escapedDisplayText + "]: "}},
		{"import preview and failure", []string{"import", path, "-v", "work-id", "-y"}, "", cliprotocol.TypeImportItem,
			[]string{"File:            " + escapedDisplayText + ".csv", "Vault:           " + escapedDisplayText,
				`could not import "New item": ` + escapedDisplayText, "Imported 0 items into " + escapedDisplayText + "."}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			before := len(app.findRequest(testCase.requestType))
			_, stderr, err := runCapturingOutput(t, false, testCase.input, testCase.args...)
			if err != nil {
				t.Fatalf("command failed: %v; stderr %q", err, stderr)
			}
			if len(app.findRequest(testCase.requestType)) != before+1 {
				t.Fatalf("command did not complete its backend request %s", testCase.requestType)
			}
			if strings.ContainsAny(stderr, "\x1b\x07\u009b") {
				t.Errorf("stderr contains raw terminal controls: %q", stderr)
			}
			for _, message := range testCase.messages {
				if !strings.Contains(stderr, message) {
					t.Errorf("stderr %q does not contain %q", stderr, message)
				}
			}
		})
	}
}

func TestHumanReadableStatusEscapesAccountText(t *testing.T) {
	app := startFakeApp(t)
	app.mutex.Lock()
	app.hello.Email, app.hello.Server = hostileDisplayText, hostileDisplayText
	app.mutex.Unlock()
	currentState, err := loadState()
	if err != nil {
		t.Fatal(err)
	}
	currentState.AccountID, currentState.Token = "ABC123", "token"
	currentState.Email, currentState.Server = hostileDisplayText, hostileDisplayText
	if err := currentState.Save(); err != nil {
		t.Fatal(err)
	}
	for _, standalone := range []bool{false, true} {
		stdout, _, err := runCapturingOutput(t, standalone, "", "status")
		if err != nil {
			t.Fatal(err)
		}
		if strings.ContainsAny(stdout, "\x1b\x07\u009b") || !strings.Contains(stdout, "account: "+escapedDisplayText+" on "+escapedDisplayText) {
			t.Errorf("standalone=%v: unsafe or missing status account: %q", standalone, stdout)
		}
	}
}
