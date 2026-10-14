package agent

import (
	"context"
	"slices"
	"testing"

	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/desktop/appinfo"
)

func TestShareLinkApprovalNamesTheItemAndFields(t *testing.T) {
	biometric := &fakeBiometric{isAvailable: true}
	emitter := createFakeEmitter()
	approver := createCommandLineApprover(biometric, emitter, &fakeUnlocker{}, 0)
	request := createCommandLineRequest()
	request.ShareLink = &clisocket.ShareLinkDescription{Title: "GitHub", FieldLabels: []string{"username", "password"}, Detail: "Expires in 7 days · 1 view · passphrase"}
	resultChannel := make(chan error, 1)
	go func() { resultChannel <- approver.ApproveCommandLine(context.Background(), request) }()
	event := waitForApprovalEvent(t, emitter)
	if event.Action != "create a share link for “GitHub”" || event.CommandLine != "Expires in 7 days · 1 view · passphrase" ||
		!slices.Equal(event.SecretNames, []string{"username", "password"}) || event.Warning != shareLinkWarning || event.IsRememberable {
		t.Errorf("event = %+v", event)
	}
	phoneDescription := approver.buildCommandLinePhoneDescription(event)
	if phoneDescription.Purpose != event.Action || phoneDescription.CommandLine != event.CommandLine || !slices.Equal(phoneDescription.Secrets, []string{"username", "password"}) {
		t.Errorf("phone description = %+v", phoneDescription)
	}
	approver.Respond(event.RequestID, true, false)
	if err := waitForCommandLineResult(t, resultChannel); err != nil {
		t.Fatal(err)
	}
	if reasons := biometric.listReasons(); len(reasons) != 1 || reasons[0] != `allow "hisar" in Terminal to create a share link for “GitHub”` {
		t.Errorf("Touch ID reasons = %q", reasons)
	}
}

func TestShareLinkApprovalNamesTheApp(t *testing.T) {
	emitter := createFakeEmitter()
	phone := createFakePhone()
	requester := createITermSigningRequester()
	requester.App.Signature = appinfo.Signature{Status: appinfo.SignatureStatusUnsigned}
	approver := createRequesterApprover(emitter, requesterApproverOption{
		requester: requester, biometric: &fakeBiometric{isAvailable: true}, phone: phone,
		getAppIconPNG: func(string, int) ([]byte, error) { return []byte("png"), nil },
	})
	request := createCommandLineRequest()
	request.ShareLink = &clisocket.ShareLinkDescription{Title: "GitHub", FieldLabels: []string{"password"}, Detail: "Expires in 1 day · 1 view"}
	resultChannel := make(chan error, 1)
	go func() { resultChannel <- approver.ApproveCommandLine(context.Background(), request) }()
	event := waitForApprovalEvent(t, emitter)
	if event.AppName != "iTerm2" || event.AppIconURL == "" || event.AppPath != iTermBundlePath || event.AppSigner != "Not signed" || event.AppWarning == "" || len(event.ChainProcesses) != 3 {
		t.Errorf("event = %+v", event)
	}
	if description := phone.waitForStart(t); description.AppName != "hisar in iTerm2" || string(description.AppIcon) != "png" || description.AppWarning != event.AppWarning || description.Purpose != "create a share link for “GitHub”" {
		t.Errorf("phone description = %+v", description)
	}
	approver.DenyAll()
	waitForCommandLineResult(t, resultChannel)
}
