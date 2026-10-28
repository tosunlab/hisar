package agent

import (
	"testing"

	"github.com/tryhisar/hisar/core/clisocket"
)

func TestPhoneDescriptionsIncludeOperationWarnings(t *testing.T) {
	approver := NewApprover(ApproverConfig{})
	for _, test := range []struct {
		name    string
		warning string
	}{
		{"share link", shareLinkWarning},
		{"plain export", plainExportWarning},
		{"agent reveal", "The value will be visible to omp and can end up in its transcript and logs."},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, appWarning := range []string{"", "The app's signature could not be verified."} {
				event := ApprovalEvent{AppWarning: appWarning, Warning: test.warning}
				want := test.warning
				if appWarning != "" {
					want = appWarning + " " + want
				}
				if got := approver.buildCommandLinePhoneDescription(event).AppWarning; got != want {
					t.Errorf("command-line warning = %q, want %q", got, want)
				}
				if got := approver.buildAgentPhoneDescription(event, clisocket.AgentApprovalRequest{}).AppWarning; got != want {
					t.Errorf("agent warning = %q, want %q", got, want)
				}
			}
		})
	}
}
