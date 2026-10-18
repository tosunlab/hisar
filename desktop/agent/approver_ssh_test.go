package agent

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/sshagent"
	"github.com/tryhisar/hisar/desktop/appinfo"
)

type sshApproverOption struct {
	biometric              Biometric
	unlocker               Unlocker
	isCompact              bool
	processes              []appinfo.Process
	fileSystem             fakeFileSystem
	getSSHRememberDuration func() time.Duration
}

func createSSHApprover(emitter *fakeEmitter, option sshApproverOption) *Approver {
	if option.biometric == nil {
		option.biometric = &fakeBiometric{isAvailable: true}
	}
	config := ApproverConfig{
		Biometric: option.biometric, Emit: emitter.emit, ShowApprovalWindow: emitter.showWindow, HideApprovalWindow: emitter.hideWindow,
		ResolveRequester:  func(int) appinfo.Requester { return createTerminalRequester(option.processes) },
		ReadFile:          option.fileSystem.readFile,
		HomeDirectory:     testHomeDirectory,
		IsCompactApproval: func() bool { return option.isCompact },
	}
	if option.unlocker != nil {
		config.Unlocker = option.unlocker
	}
	if option.getSSHRememberDuration != nil {
		config.GetSSHRememberDuration = option.getSSHRememberDuration
	}
	return NewApprover(config)
}

func createPushChain() []appinfo.Process {
	return append([]appinfo.Process{
		createSSHProcess("-o", "SendEnv=GIT_PROTOCOL", "git@"+testRemoteHost, testRemoteGitPushed),
		createGitProcess(40, "push", "origin", "main"),
	}, createShellAndTerminal()...)
}

func TestSSHApprovalDescribesThePush(t *testing.T) {
	emitter := createFakeEmitter()
	biometric := &fakeBiometric{isAvailable: true}
	approver := createSSHApprover(emitter, sshApproverOption{biometric: biometric, processes: createPushChain()})
	resultChannel := startApproval(approver, context.Background(), createTestRequest())
	event := waitForApprovalEvent(t, emitter)
	if event.Purpose != PurposePush || event.Action != "push to" || event.Target != testRemoteHost || event.CommandLine != "git push origin main" ||
		event.RepositoryPath != "~/src/core" || event.HostAndPort != "git.tryhisar.com:22" || !event.IsRememberable || event.IsCompact || event.AppName != "Terminal" {
		t.Errorf("event = %+v", event)
	}
	approver.Respond(event.RequestID, true, true)
	if result := waitForResult(t, resultChannel); !result.result.IsAllowed || !result.result.IsRemembered {
		t.Errorf("result = %+v", result)
	}
	if reasons := biometric.listReasons(); len(reasons) != 1 || reasons[0] != `allow "Terminal" to push to git.tryhisar.com` {
		t.Errorf("Touch ID reasons = %q", reasons)
	}

	resultChannel = startApproval(approver, context.Background(), createTestRequest())
	event = waitForApprovalEvent(t, emitter)
	approver.Respond(event.RequestID, true, false)
	if result := waitForResult(t, resultChannel); !result.result.IsAllowed || result.result.IsRemembered {
		t.Errorf("unchecked box remembered: %+v", result)
	}
}

func TestSSHApprovalSaysHowLongItIsRemembered(t *testing.T) {
	emitter := createFakeEmitter()
	approver := createSSHApprover(emitter, sshApproverOption{processes: createPushChain()})
	resultChannel := startApproval(approver, context.Background(), createTestRequest())
	event := waitForApprovalEvent(t, emitter)
	if event.RememberPeriod != "for 1 hour" || event.RememberLabel != "" {
		t.Errorf("event = %+v", event)
	}
	if label := approver.buildSSHPhoneDescription(event, 0).RememberLabel; label != "Also allows Terminal for 1 hour" {
		t.Errorf("phone label = %q", label)
	}
	approver.Respond(event.RequestID, false, false)
	waitForResult(t, resultChannel)

	untilLock := createSSHApprover(emitter, sshApproverOption{processes: createPushChain(), getSSHRememberDuration: func() time.Duration { return 0 }})
	resultChannel = startApproval(untilLock, context.Background(), createTestRequest())
	event = waitForApprovalEvent(t, emitter)
	if event.RememberPeriod != "until Hisar locks" || !event.IsRememberedByDefault {
		t.Errorf("event = %+v", event)
	}
	if label := untilLock.buildSSHPhoneDescription(event, 0).RememberLabel; label != "Also allows Terminal until Hisar locks" {
		t.Errorf("phone label = %q", label)
	}
	untilLock.Respond(event.RequestID, true, true)
	if result := waitForResult(t, resultChannel); !result.result.IsAllowed || !result.result.IsRemembered {
		t.Errorf("result = %+v", result)
	}

	forwarded := createTestRequest()
	forwarded.IsForwarded, forwarded.IsRememberable = true, false
	resultChannel = startApproval(untilLock, context.Background(), forwarded)
	if event = waitForApprovalEvent(t, emitter); event.RememberPeriod != "" {
		t.Errorf("a request that is never remembered has a period: %+v", event)
	}
	untilLock.Respond(event.RequestID, false, false)
	waitForResult(t, resultChannel)
}

func TestCommitSigningIsNeverRemembered(t *testing.T) {
	emitter := createFakeEmitter()
	approver := createSSHApprover(emitter, sshApproverOption{processes: createSigningChain(), fileSystem: fakeFileSystem{
		testSigningBuffer: "tree 4b825dc642cb6eb9a060e54bf8d69288fbee4904\n\nAdd the approval dialog\n",
	}})
	request := createTestRequest()
	request.SignatureNamespace, request.DestinationHost, request.DestinationFingerprint = "git", "", ""
	resultChannel := startApproval(approver, context.Background(), request)
	event := waitForApprovalEvent(t, emitter)
	if event.Purpose != PurposeSignCommit || event.CommitSubject != "Add the approval dialog" || event.IsRememberable {
		t.Errorf("event = %+v", event)
	}
	approver.Respond(event.RequestID, true, true)
	if result := waitForResult(t, resultChannel); !result.result.IsAllowed || result.result.IsRemembered {
		t.Errorf("a commit signature was remembered: %+v", result)
	}
}

func TestCompactApprovalOnlyWhenNothingElseIsNeeded(t *testing.T) {
	cases := []struct {
		name        string
		option      sshApproverOption
		isForwarded bool
		want        bool
	}{
		{"unlocked with Touch ID", sshApproverOption{isCompact: true}, false, true},
		{"switch off", sshApproverOption{}, false, false},
		{"forwarded", sshApproverOption{isCompact: true}, true, false},
		{"without Touch ID", sshApproverOption{isCompact: true, biometric: &fakeBiometric{}}, false, false},
		{"locked", sshApproverOption{isCompact: true, unlocker: &fakeUnlocker{isLocked: true, isTouchIDUnlockReady: true}}, false, false},
	}
	for _, testCase := range cases {
		emitter := createFakeEmitter()
		approver := createSSHApprover(emitter, testCase.option)
		request := createTestRequest()
		request.IsForwarded = testCase.isForwarded
		resultChannel := startApproval(approver, context.Background(), request)
		event := waitForApprovalEvent(t, emitter)
		approver.presenter.waitUntilIdle()
		if event.IsCompact != testCase.want || !slices.Equal(emitter.listShownCompact(), []bool{testCase.want}) {
			t.Errorf("%s: compact %v, shown %v", testCase.name, event.IsCompact, emitter.listShownCompact())
		}
		approver.Respond(event.RequestID, false, false)
		waitForResult(t, resultChannel)
	}
}

func TestWindowFollowsTheFirstRequest(t *testing.T) {
	emitter := createFakeEmitter()
	approver := createSSHApprover(emitter, sshApproverOption{isCompact: true})
	forwarded := createTestRequest()
	forwarded.IsForwarded = true
	firstChannel := startApproval(approver, context.Background(), forwarded)
	firstEvent := waitForApprovalEvent(t, emitter)
	secondChannel := startApproval(approver, context.Background(), createTestRequest())
	secondEvent := waitForApprovalEvent(t, emitter)
	approver.presenter.waitUntilIdle()
	if !secondEvent.IsCompact || !slices.Equal(emitter.listShownCompact(), []bool{false, false}) {
		t.Errorf("a compact request resized the full dialog: %v", emitter.listShownCompact())
	}
	approver.Respond(firstEvent.RequestID, false, false)
	approver.Respond(secondEvent.RequestID, false, false)
	waitForResult(t, firstChannel)
	waitForResult(t, secondChannel)
}

func TestLockedSSHRequestUnlocksWithTouchID(t *testing.T) {
	emitter := createFakeEmitter()
	biometric := &fakeBiometric{isAvailable: true}
	unlocker := &fakeUnlocker{isLocked: true, isTouchIDUnlockReady: true}
	approver := createSSHApprover(emitter, sshApproverOption{biometric: biometric, unlocker: unlocker, isCompact: true, processes: createPushChain()})
	request := createTestRequest()
	request.IsUnlockRequired, request.KeyID, request.KeyTitle, request.Fingerprint = true, "", "", ""
	resultChannel := startApproval(approver, context.Background(), request)
	event := waitForApprovalEvent(t, emitter)
	if !event.IsUnlockRequired || event.IsPasswordUnlock || event.IsCompact || event.Purpose != PurposePush {
		t.Errorf("event = %+v", event)
	}
	approver.Respond(event.RequestID, true, true)
	if result := waitForResult(t, resultChannel); !result.result.IsAllowed || !result.result.IsRemembered {
		t.Errorf("result = %+v", result)
	}
	if unlocker.IsLocked() || unlocker.touchIDUnlockCount != 1 {
		t.Error("Touch ID did not unlock Hisar")
	}
	if reasons := biometric.listReasons(); len(reasons) != 1 || reasons[0] != `unlock Hisar and allow "Terminal" to push to git.tryhisar.com` {
		t.Errorf("Touch ID reasons = %q", reasons)
	}
}

func TestLockedSSHRequestUnlocksWithPassword(t *testing.T) {
	emitter := createFakeEmitter()
	biometric := &fakeBiometric{isAvailable: true}
	unlocker := &fakeUnlocker{isLocked: true, correctPassword: "correct horse battery"}
	approver := createSSHApprover(emitter, sshApproverOption{biometric: biometric, unlocker: unlocker})
	resultChannel := startApproval(approver, context.Background(), createTestRequest())
	event := waitForApprovalEvent(t, emitter)
	if !event.IsUnlockRequired || !event.IsPasswordUnlock {
		t.Fatalf("event = %+v", event)
	}
	approver.RespondWithPassword(event.RequestID, "wrong", true)
	if failedEvent := waitForFailedEvent(t, emitter); failedEvent.Message != "Incorrect master password." {
		t.Errorf("failure %q", failedEvent.Message)
	}
	approver.RespondWithPassword(event.RequestID, "correct horse battery", false)
	if result := waitForResult(t, resultChannel); !result.result.IsAllowed || result.result.IsRemembered {
		t.Errorf("result = %+v", result)
	}
	if unlocker.IsLocked() || len(biometric.listReasons()) != 0 {
		t.Error("the password did not unlock Hisar without Touch ID")
	}
}

func TestSSHApprovalWithMasterPasswordInsteadOfTouchID(t *testing.T) {
	emitter := createFakeEmitter()
	biometric := &fakeBiometric{isAvailable: true}
	unlocker := &fakeUnlocker{correctPassword: "correct horse battery"}
	approver := createSSHApprover(emitter, sshApproverOption{biometric: biometric, unlocker: unlocker})
	resultChannel := startApproval(approver, context.Background(), createTestRequest())
	event := waitForApprovalEvent(t, emitter)
	if event.IsUnlockRequired {
		t.Errorf("an unlocked Hisar asked to unlock: %+v", event)
	}
	approver.RespondWithPassword(event.RequestID, "correct horse battery", true)
	if result := waitForResult(t, resultChannel); !result.result.IsAllowed || !result.result.IsRemembered {
		t.Errorf("result = %+v", result)
	}
	if len(biometric.listReasons()) != 0 || len(unlocker.passwords) != 1 {
		t.Errorf("Touch ID %v, passwords %v", biometric.listReasons(), unlocker.passwords)
	}

	withoutUnlocker := createSSHApprover(emitter, sshApproverOption{})
	resultChannel = startApproval(withoutUnlocker, context.Background(), createTestRequest())
	event = waitForApprovalEvent(t, emitter)
	withoutUnlocker.RespondWithPassword(event.RequestID, "anything", false)
	if failedEvent := waitForFailedEvent(t, emitter); failedEvent.Message != errPasswordCheck.Error() {
		t.Errorf("failure %q", failedEvent.Message)
	}
	withoutUnlocker.Respond(event.RequestID, false, false)
	waitForResult(t, resultChannel)
}

func TestForwardedSSHRequestEvent(t *testing.T) {
	emitter := createFakeEmitter()
	approver := createSSHApprover(emitter, sshApproverOption{isCompact: true, processes: append([]appinfo.Process{createSSHProcess("-A", "build.example.com")}, createShellAndTerminal()...)})
	request := createTestRequest()
	request.IsForwarded, request.IsRememberable, request.ForwardedFromHost, request.DestinationPort = true, false, "build.example.com", 2222
	resultChannel := startApproval(approver, context.Background(), request)
	event := waitForApprovalEvent(t, emitter)
	if !event.IsForwarded || event.ForwardedFromHost != "build.example.com" || event.IsRememberable || event.IsCompact || event.DestinationHost != "github.com:2222" || event.HostAndPort != "github.com:2222" {
		t.Errorf("event = %+v", event)
	}
	approver.Respond(event.RequestID, true, true)
	if result := waitForResult(t, resultChannel); !result.result.IsAllowed || result.result.IsRemembered {
		t.Errorf("a forwarded request was remembered: %+v", result)
	}
}

var _ sshagent.Approver = (*Approver)(nil)
