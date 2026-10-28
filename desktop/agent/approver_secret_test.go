package agent

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/approval"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/desktop/appinfo"
)

func TestCommandLineTimeoutReportsTheDialogsDeadline(t *testing.T) {
	for _, isPhoneAvailable := range []bool{false, true} {
		phone := createFakePhone()
		phone.isAvailable = isPhoneAvailable
		approver := NewApprover(ApproverConfig{
			Biometric: &fakeBiometric{isAvailable: true}, Emit: createFakeEmitter().emit,
			PhoneApprover: phone, Timeout: 20 * time.Millisecond, PhoneApprovalTimeout: 60 * time.Millisecond,
		})
		err := approver.ApproveCommandLine(context.Background(), createCommandLineRequest())
		var timeoutError clisocket.TimeoutError
		expected := 20 * time.Millisecond
		if isPhoneAvailable {
			expected = 60 * time.Millisecond
		}
		if !errors.As(err, &timeoutError) || timeoutError.Timeout != expected || !errors.Is(err, clisocket.ErrDenied) {
			t.Errorf("phone %v: %v", isPhoneAvailable, err)
		}
	}
}

func createSecretRequest(secretNames []string, commandLine []string) clisocket.ApprovalRequest {
	request := createCommandLineRequest()
	request.IsSecretRequest, request.SecretNames, request.CommandLine = true, secretNames, commandLine
	return request
}

func TestRunApprovalShowsTheCommandAndTheSecrets(t *testing.T) {
	biometric := &fakeBiometric{isAvailable: true}
	emitter := createFakeEmitter()
	approver := createCommandLineApprover(biometric, emitter, &fakeUnlocker{}, 0)
	secretNames := []string{"Personal › GitHub › password", "Personal › GitHub › API token", "Work › AWS › access key", "Work › AWS › secret key", "Work › Database › password", "Work › Sentry › token"}
	request := createSecretRequest(secretNames, []string{"/usr/local/bin/npm", "run", "test:unit", "--", "--grep", "it's slow"})
	resultChannel := make(chan error, 1)
	go func() { resultChannel <- approver.ApproveCommandLine(context.Background(), request) }()
	event := waitForApprovalEvent(t, emitter)
	if event.Action != "run a command with 6 secrets" || event.CommandLine != `/usr/local/bin/npm run test:unit -- --grep 'it'\''s slow'` ||
		!slices.Equal(event.SecretNames, secretNames[:4]) || event.MoreSecretCount != 2 || event.TerminalName != "ttys003" {
		t.Errorf("event = %+v", event)
	}
	approver.Respond(event.RequestID, true, false)
	if err := waitForCommandLineResult(t, resultChannel); err != nil {
		t.Fatal(err)
	}
	if reasons := biometric.listReasons(); len(reasons) != 1 || reasons[0] != `allow "hisar" in Terminal to run a command with 6 secrets` {
		t.Errorf("Touch ID reasons = %q", reasons)
	}
}

func TestReadApprovalNamesOneSecret(t *testing.T) {
	emitter := createFakeEmitter()
	approver := createCommandLineApprover(&fakeBiometric{isAvailable: true}, emitter, &fakeUnlocker{}, 0)
	resultChannel := make(chan error, 1)
	go func() {
		resultChannel <- approver.ApproveCommandLine(context.Background(), createSecretRequest([]string{"Personal › GitHub › one-time code"}, nil))
	}()
	event := waitForApprovalEvent(t, emitter)
	if event.Action != "read 1 secret" || event.CommandLine != "" || !slices.Equal(event.SecretNames, []string{"Personal › GitHub › one-time code"}) || event.MoreSecretCount != 0 {
		t.Errorf("event = %+v", event)
	}
	approver.Respond(event.RequestID, false, false)
	waitForCommandLineResult(t, resultChannel)
}

func TestOtherCommandLineApprovalHasNoSecrets(t *testing.T) {
	emitter := createFakeEmitter()
	approver := createCommandLineApprover(&fakeBiometric{isAvailable: true}, emitter, &fakeUnlocker{}, 0)
	resultChannel := startCommandLineApproval(approver, context.Background())
	event := waitForApprovalEvent(t, emitter)
	if event.Action != "" || event.CommandLine != "" || event.SecretNames == nil || len(event.SecretNames) != 0 {
		t.Errorf("event = %+v", event)
	}
	approver.Respond(event.RequestID, false, false)
	waitForCommandLineResult(t, resultChannel)
}

func TestPhoneSeesTheSecretsOfARunRequest(t *testing.T) {
	emitter := createFakeEmitter()
	phone := createFakePhone()
	approver := NewApprover(ApproverConfig{
		Biometric: &fakeBiometric{isAvailable: true}, Emit: emitter.emit, ShowApprovalWindow: emitter.showWindow, HideApprovalWindow: emitter.hideWindow,
		ResolveRequester: func(int) appinfo.Requester { return createTerminalRequester(nil) },
		PhoneApprover:    phone,
		Timeout:          5 * time.Second,
	})
	secretNames := []string{"A › a › password", "B › b › password", "C › c › password", "D › d › password", "E › e › password"}
	resultChannel := make(chan error, 1)
	go func() {
		resultChannel <- approver.ApproveCommandLine(context.Background(), createSecretRequest(secretNames, []string{"docker", "compose", "up"}))
	}()
	waitForApprovalEvent(t, emitter)
	description := phone.waitForStart(t)
	want := approval.Description{
		Kind: approval.KindCommandLine, AppName: "hisar in Terminal", Purpose: "run a command with 5 secrets", CommandLine: "docker compose up", TerminalName: "ttys003",
		Secrets:   []string{"A › a › password", "B › b › password", "C › c › password", "D › d › password", "and 1 more"},
		Allowance: commandLineAllowance,
		Lifetime:  "until 10 minutes after this terminal’s last command, or until Hisar locks",
	}
	if !reflect.DeepEqual(description, want) {
		t.Errorf("phone request = %+v", description)
	}
	phone.answer(true)
	if err := waitForCommandLineResult(t, resultChannel); err != nil {
		t.Errorf("approved on the phone: %v", err)
	}
}
