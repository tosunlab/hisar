package agent

import (
	"context"
	"slices"
	"testing"

	"github.com/tryhisar/hisar/desktop/commandline"
)

func TestShareLinkApprovalNamesTheItemAndFields(t *testing.T) {
	biometric := &fakeBiometric{isAvailable: true}
	emitter := createFakeEmitter()
	approver := createCommandLineApprover(biometric, emitter, &fakeUnlocker{}, 0)
	request := createCommandLineRequest()
	request.ShareLink = &commandline.ShareLinkDescription{Title: "GitHub", AllFieldLabel: []string{"username", "password"}, Detail: "Expires in 7 days · 1 view · passphrase"}
	resultChannel := make(chan error, 1)
	go func() { resultChannel <- approver.ApproveCommandLine(context.Background(), request) }()
	event := waitForApprovalEvent(t, emitter)
	if event.Action != "create a share link for “GitHub”" || event.CommandLine != "Expires in 7 days · 1 view · passphrase" ||
		!slices.Equal(event.AllSecretName, []string{"username", "password"}) || event.Warning != shareLinkWarning || event.IsRememberable {
		t.Errorf("event = %+v", event)
	}
	phoneDescription := buildCommandLinePhoneDescription(event)
	if phoneDescription.Purpose != event.Action || phoneDescription.CommandLine != event.CommandLine || !slices.Equal(phoneDescription.Secrets, []string{"username", "password"}) {
		t.Errorf("phone description = %+v", phoneDescription)
	}
	approver.Respond(event.RequestID, true, false)
	if err := waitForCommandLineResult(t, resultChannel); err != nil {
		t.Fatal(err)
	}
	if allReason := biometric.listAllReason(); len(allReason) != 1 || allReason[0] != `allow "hisar" in Terminal to create a share link for “GitHub”` {
		t.Errorf("Touch ID reasons = %q", allReason)
	}
}
