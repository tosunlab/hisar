package agent

import (
	"context"
	"testing"

	"github.com/tryhisar/hisar/core/clisocket"
)

func TestExportApprovalSaysWhatIsWritten(t *testing.T) {
	for _, testCase := range []struct {
		isPlain         bool
		wantCommandLine string
		wantWarning     string
	}{
		{false, "Encrypted with an export passphrase · /Users/alice/hisar-export.hisar", ""},
		{true, "Plain JSON, not encrypted · /Users/alice/hisar-export.hisar", plainExportWarning},
	} {
		biometric := &fakeBiometric{isAvailable: true}
		emitter := createFakeEmitter()
		approver := createCommandLineApprover(biometric, emitter, &fakeUnlocker{}, 0)
		request := createCommandLineRequest()
		request.Export = &clisocket.ExportDescription{IsPlain: testCase.isPlain, Path: "/Users/alice/hisar-export.hisar"}
		resultChannel := make(chan error, 1)
		go func() { resultChannel <- approver.ApproveCommandLine(context.Background(), request) }()
		event := waitForApprovalEvent(t, emitter)
		if event.Action != "export all your vaults" || event.CommandLine != testCase.wantCommandLine || event.Warning != testCase.wantWarning || event.IsRememberable {
			t.Errorf("event = %+v", event)
		}
		approver.Respond(event.RequestID, true, false)
		if err := waitForCommandLineResult(t, resultChannel); err != nil {
			t.Fatal(err)
		}
		if allReason := biometric.listAllReason(); len(allReason) != 1 || allReason[0] != `allow "hisar" in Terminal to export all your vaults` {
			t.Errorf("Touch ID reasons = %q", allReason)
		}
	}
}
