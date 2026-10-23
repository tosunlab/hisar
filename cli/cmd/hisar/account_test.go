package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/session"
)

func runAccountWithInput(t *testing.T, input string, args ...string) error {
	t.Helper()
	originalReader := stdinReader
	defer func() { stdinReader = originalReader }()
	stdinReader = bufio.NewReader(strings.NewReader(input))
	flags := flag.NewFlagSet("account", flag.ContinueOnError)
	flags.SetOutput(&bytes.Buffer{})
	return runAccount(context.Background(), flags, args)
}

func TestAccountCommandChecksItsArgumentsBeforeUnlocking(t *testing.T) {
	t.Setenv("HISAR_STATE", filepath.Join(t.TempDir(), "state.json"))
	existingKitPath := filepath.Join(t.TempDir(), "kit.pdf")
	os.WriteFile(existingKitPath, []byte("old kit"), 0o600)
	cases := []struct {
		name    string
		input   string
		args    []string
		wantErr string
	}{
		{"unknown action", "", []string{"rename"}, "unknown account action"},
		{"prompted action", "change-password\n", nil, "not logged in"},
		{"change password extra argument", "", []string{"change-password", "extra"}, "usage"},
		{"replace keeps an existing kit", "", []string{"replace-secret-key", "-o", existingKitPath}, "already exists"},
		{"replace needs a login", "", []string{"replace-secret-key", "-o", filepath.Join(t.TempDir(), "new.pdf")}, "not logged in"},
		{"delete needs a login", "", []string{"delete", "--email", "a@example.com"}, "not logged in"},
	}
	for _, testCase := range cases {
		err := runAccountWithInput(t, testCase.input, testCase.args...)
		if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
			t.Errorf("%s: err = %v, want %q", testCase.name, err, testCase.wantErr)
		}
	}
	if content, _ := os.ReadFile(existingKitPath); string(content) != "old kit" {
		t.Error("an existing Emergency Kit was overwritten")
	}
}

func TestPromptChangedPasswordRefusesWeakAndUnchangedPasswords(t *testing.T) {
	originalReader := stdinReader
	defer func() { stdinReader = originalReader }()
	stdinReader = bufio.NewReader(strings.NewReader("short\ncurrent password\nnew master password\ntypo master password\nnew master password\nnew master password\n"))
	newPassword, err := promptChangedPassword([]byte("current password"))
	if err != nil || string(newPassword) != "new master password" {
		t.Errorf("new password: %q %v", newPassword, err)
	}
}

func TestPrintAccountDeletionPreview(t *testing.T) {
	var output bytes.Buffer
	printAccountDeletionPreview(&output, &session.AccountDeletionPreview{
		OwnedVaults: []session.DeletionVault{
			{Name: "Personal", Type: "personal", Role: "owner"},
			{Name: "Family", Type: "shared", Role: "owner", OtherMemberCount: 2},
		},
		MemberVaults:        []session.DeletionVault{{Name: "Work", Type: "shared", Role: "editor"}},
		ShareLinkCount:      3,
		SentInvitationCount: 1,
		DeviceCount:         2,
		Organizations: []apitypes.DeletionOrganization{
			{OrganizationID: "acme-id", Name: "Acme", NewAdminName: "Bob"},
			{OrganizationID: "solo-id", Name: "Solo", IsDeleted: true},
			{OrganizationID: "globex-id", Name: "Globex"},
		},
	})
	for _, expected := range []string{
		`Deleted with its items: "Personal"` + "\n",
		`Deleted with its items: "Family" (2 other people lose access)`,
		`Your access is removed: "Work" (editor; it stays with its other owners)`,
		"Share links revoked: 3",
		"Invitations you sent withdrawn: 1",
		"Devices signed out: 2, including this one",
		`You leave the organization "Acme"; Bob becomes its admin`,
		`Organization deleted with its teams, as you are its last member: "Solo"`,
		`You leave the organization "Globex"` + "\n",
		"Export your items first with `hisar export`",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("preview misses %q:\n%s", expected, output.String())
		}
	}
	if strings.Contains(output.String(), "Invitations for you") {
		t.Errorf("preview lists zero counts:\n%s", output.String())
	}
}

func TestAccountDeletionPreviewEscapesTheNextAdminsName(t *testing.T) {
	var output bytes.Buffer
	printAccountDeletionPreview(&output, &session.AccountDeletionPreview{
		Organizations: []apitypes.DeletionOrganization{{Name: "Work", NewAdminName: hostileDisplayText}},
	})
	if text := output.String(); strings.ContainsAny(text, "\x1b\x07\u009b") ||
		!strings.Contains(text, `You leave the organization "Work"; `+escapedDisplayText+" becomes its admin") {
		t.Errorf("unsafe or missing next-admin preview: %q", text)
	}
}

func TestConvertAccountError(t *testing.T) {
	cases := map[string]struct {
		err      error
		expected string
	}{
		"wrong password": {session.ErrBadCredentials, "the master password is not correct"},
		"signed out":     {&apiclient.APIError{Status: http.StatusUnauthorized}, "sign in again with `hisar login`"},
		"changed":        {&apiclient.APIError{Status: http.StatusConflict}, "changed on another device"},
		"email":          {session.ErrEmailMismatch, "nothing was deleted"},
	}
	for name, testCase := range cases {
		if err := convertAccountError(testCase.err); !strings.Contains(err.Error(), testCase.expected) {
			t.Errorf("%s: %v", name, err)
		}
	}
	other := errors.New("other")
	if convertAccountError(other) != other {
		t.Error("an unrelated error was changed")
	}
	if describeSignedOutDevice(1) != "1 other device was signed out." || describeSignedOutDevice(3) != "3 other devices were signed out." {
		t.Error("signed-out device count wording")
	}
	if describeSignedOutDevice(-1) != "Any other devices were signed out; their count could not be recovered." {
		t.Error("an unknown count was reported as zero")
	}
}

func TestAccountDeletionAsksForReviewOnlyWhenThePlanChanged(t *testing.T) {
	for _, answer := range []struct {
		name   string
		body   any
		reason string
	}{
		{"plan changed", apitypes.DeletionPlanChangedResponse{Error: "The deletion plan changed. Review the deletion again before confirming.", IsDeletionPlanChanged: true}, "review the deletion again"},
		{"memberships busy", apitypes.Error{Error: "The account's memberships are changing. Try deleting the account again."}, "memberships are changing"},
		{"credentials changed", apitypes.Error{Error: "the master password or Secret Key was changed on another device; try again"}, "changed on another device"},
	} {
		t.Run(answer.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(writer).Encode(answer.body)
			}))
			defer server.Close()
			err := convertAccountDeletionError(apiclient.New(server.URL).DeleteAccount(context.Background(), &apitypes.DeleteAccountRequest{PlanVersion: "1:reviewed"}))
			if err == nil || !strings.Contains(err.Error(), answer.reason) {
				t.Fatalf("deletion conflict lost its reason: %v", err)
			}
			if isReviewAsked := strings.Contains(err.Error(), "deletion plan changed"); isReviewAsked != (answer.name == "plan changed") {
				t.Errorf("asks for a new review %v: %v", isReviewAsked, err)
			}
		})
	}
}

func TestAccountDeletionKeepsOtherErrors(t *testing.T) {
	if err := convertAccountDeletionError(session.ErrEmailMismatch); !strings.Contains(err.Error(), "nothing was deleted") {
		t.Errorf("email mismatch: %v", err)
	}
	other := errors.New("other")
	if convertAccountDeletionError(other) != other {
		t.Error("an unrelated error was changed")
	}
}
