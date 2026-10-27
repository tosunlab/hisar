package agent

import (
	"testing"

	"github.com/tryhisar/hisar/core/clisocket"
)

func TestCommandLineApprovalDisclosesReusablePermissions(t *testing.T) {
	approver := NewApprover(ApproverConfig{})
	want := "Read secrets and one-time codes\nAdd, change and delete items\nManage vaults, sharing, invitations, organizations and devices\nShare links and exports still ask each time."
	for _, request := range []clisocket.ApprovalRequest{
		{},
		{IsSecretRequest: true, SecretNames: []string{"Personal › GitHub › password"}},
		{IsSecretRequest: true, SecretNames: []string{"Personal › GitHub › password"}, CommandLine: []string{"git", "push"}},
		{ShareLink: &clisocket.ShareLinkDescription{Title: "GitHub"}},
		{Export: &clisocket.ExportDescription{IsPlain: true, Path: "/tmp/export.json"}},
	} {
		event := approver.buildCommandLineEvent("request", request)
		if event.Allowance != want {
			t.Errorf("Mac allowance = %q, want %q", event.Allowance, want)
		}
		if got := approver.buildCommandLinePhoneDescription(event).Allowance; got != want {
			t.Errorf("phone allowance = %q, want %q", got, want)
		}
		if got := approver.buildAgentPhoneDescription(event, clisocket.AgentApprovalRequest{}).Allowance; got != "" {
			t.Errorf("agent inherited command-line permissions: %q", got)
		}
	}
}
