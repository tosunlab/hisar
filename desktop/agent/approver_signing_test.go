package agent

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/sshagent"
	"github.com/tryhisar/hisar/desktop/appinfo"
)

const (
	testItermBundlePath = "/Applications/iTerm.app"
	testCommitPayload   = "tree 4b825dc642cb6eb9a060e54bf8d69288fbee4904\n\nFix the login redirect\n"
	testTagPayload      = "object 1111111111111111111111111111111111111111\ntype commit\ntag v1.0.0\ntagger Deniz <deniz@example.com> 1760000000 +0300\n\nRelease 1.0.0\n"
)

// signingTest drives one approver whose requester, process chain, clock and setting the test changes between signatures.
type signingTest struct {
	mutex           sync.Mutex
	emitter         *fakeEmitter
	phone           *fakePhone
	approver        *Approver
	now             time.Time
	rememberSeconds int
	requester       appinfo.Requester
	fileSystem      fakeFileSystem
}

func createSigningTest(t *testing.T, rememberSeconds int, phone *fakePhone) *signingTest {
	t.Helper()
	test := &signingTest{
		emitter:         createFakeEmitter(),
		phone:           phone,
		now:             time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC),
		rememberSeconds: rememberSeconds,
		fileSystem:      fakeFileSystem{testSigningBuffer: testCommitPayload},
	}
	test.requester = createTerminalRequester(createRebaseChain(testRepositoryPath))
	config := ApproverConfig{
		Biometric: &fakeBiometric{isAvailable: true}, Emit: test.emitter.emit, ShowApprovalWindow: test.emitter.showWindow, HideApprovalWindow: test.emitter.hideWindow,
		ResolveRequester: func(int) appinfo.Requester {
			test.mutex.Lock()
			defer test.mutex.Unlock()
			return test.requester
		},
		ReadFile: func(path string, maximumSize int64) ([]byte, error) {
			test.mutex.Lock()
			defer test.mutex.Unlock()
			return test.fileSystem.readFile(path, maximumSize)
		},
		HomeDirectory: testHomeDirectory,
		Timeout:       5 * time.Second,
		GetSigningRememberDuration: func() time.Duration {
			test.mutex.Lock()
			defer test.mutex.Unlock()
			return time.Duration(test.rememberSeconds) * time.Second
		},
		Now: func() time.Time {
			test.mutex.Lock()
			defer test.mutex.Unlock()
			return test.now
		},
	}
	if phone != nil {
		config.PhoneApprover = phone
	}
	test.approver = NewApprover(config)
	return test
}

func createRebaseChain(repositoryPath string) []appinfo.Process {
	return []appinfo.Process{
		{PID: 60, Path: "/usr/bin/ssh-keygen", Arguments: []string{"ssh-keygen", "-Y", "sign", "-n", "git", "-f", "/Users/developer/.ssh/signing.pub", testSigningBuffer}, WorkingDirectory: repositoryPath},
		{PID: 40, Path: testXcodeGitPath, Arguments: []string{"git", "rebase", "-i", "main"}, WorkingDirectory: repositoryPath},
		{PID: 30, Path: "/bin/zsh", Arguments: []string{"-zsh"}, WorkingDirectory: repositoryPath},
		{PID: 10, Path: testTerminalPath, Arguments: []string{testTerminalPath}, WorkingDirectory: "/"},
	}
}

func createSigningRequest() sshagent.ApprovalRequest {
	return sshagent.ApprovalRequest{
		KeyID: "key-1", KeyTitle: "Signing", Fingerprint: "SHA256:signing", ClientName: "ssh-keygen", ClientPath: "/usr/bin/ssh-keygen",
		ClientPID: 60, ApplicationPath: terminalBundlePath, SignatureNamespace: sshagent.GitSignatureNamespace,
	}
}

func (test *signingTest) change(change func(test *signingTest)) {
	test.mutex.Lock()
	defer test.mutex.Unlock()
	change(test)
}

// approveInDialog expects a dialog for the signature and approves it with the checkbox as given.
func (test *signingTest) approveInDialog(t *testing.T, request sshagent.ApprovalRequest, isRemembered bool) ApprovalEvent {
	t.Helper()
	resultChannel := startApproval(test.approver, context.Background(), request)
	event := waitForApprovalEvent(t, test.emitter)
	test.approver.Respond(event.RequestID, true, isRemembered)
	if result := waitForResult(t, resultChannel); !result.result.IsAllowed || result.result.IsRemembered {
		t.Errorf("result = %+v", result)
	}
	return event
}

// isSignedWithoutAsking reports whether a signature is allowed at once; one that asks is denied when the short context ends.
func (test *signingTest) isSignedWithoutAsking(t *testing.T, request sshagent.ApprovalRequest) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	result, err := test.approver.Approve(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsAllowed {
		select {
		case <-test.emitter.approvalEventChannel:
		case <-time.After(time.Second):
			t.Error("a signature was denied without a dialog")
		}
	}
	return result.IsAllowed
}

func TestSigningIsNotRememberedWithoutTheSetting(t *testing.T) {
	test := createSigningTest(t, 0, nil)
	event := test.approveInDialog(t, createSigningRequest(), true)
	if event.IsRememberable || event.RememberLabel != "" {
		t.Errorf("the dialog offers remembering without the setting: %+v", event)
	}
	if test.isSignedWithoutAsking(t, createSigningRequest()) {
		t.Error("a commit signature was remembered with the setting off")
	}
}

func TestRememberedSigningCoversARebase(t *testing.T) {
	test := createSigningTest(t, 600, nil)
	event := test.approveInDialog(t, createSigningRequest(), true)
	if !event.IsRememberable || event.IsRememberedByDefault || event.RememberLabel != "Don’t ask again for Terminal in ~/src/core for 10 minutes" || event.Purpose != PurposeSignCommit {
		t.Fatalf("event = %+v", event)
	}
	for commit := range 30 {
		test.change(func(test *signingTest) { test.now = test.now.Add(10 * time.Second) })
		if !test.isSignedWithoutAsking(t, createSigningRequest()) {
			t.Fatalf("commit %d of the rebase asked again", commit+2)
		}
	}
	if showCount, _ := test.emitter.getWindowCount(); showCount != 1 {
		t.Errorf("the rebase showed the dialog %d times", showCount)
	}
}

func TestUncheckedSigningAsksAgain(t *testing.T) {
	test := createSigningTest(t, 3600, nil)
	event := test.approveInDialog(t, createSigningRequest(), false)
	if event.RememberLabel != "Don’t ask again for Terminal in ~/src/core for 1 hour" {
		t.Errorf("label = %q", event.RememberLabel)
	}
	if test.isSignedWithoutAsking(t, createSigningRequest()) {
		t.Error("an unchecked signature was remembered")
	}
}

func TestRememberedSigningScope(t *testing.T) {
	cases := []struct {
		name           string
		change         func(test *signingTest, request *sshagent.ApprovalRequest)
		isRememberable bool
	}{
		{"another key", func(test *signingTest, request *sshagent.ApprovalRequest) { request.Fingerprint = "SHA256:other" }, true},
		{"another repository", func(test *signingTest, request *sshagent.ApprovalRequest) {
			test.requester = createTerminalRequester(createRebaseChain(testHomeDirectory + "/src/website"))
		}, true},
		{"another app", func(test *signingTest, request *sshagent.ApprovalRequest) {
			app := appinfo.App{Name: "iTerm2", Path: testItermBundlePath}
			test.requester = appinfo.Requester{Kind: appinfo.RequesterKindApp, Name: "iTerm2", App: app, Processes: createRebaseChain(testRepositoryPath)}
		}, true},
		{"a background process", func(test *signingTest, request *sshagent.ApprovalRequest) {
			test.requester = appinfo.Requester{Kind: appinfo.RequesterKindBackground, Name: "A background process", Processes: createRebaseChain(testRepositoryPath)}
		}, false},
		{"no repository", func(test *signingTest, request *sshagent.ApprovalRequest) {
			test.requester = createTerminalRequester(createRebaseChain("")[:1])
		}, false},
		{"a tag", func(test *signingTest, request *sshagent.ApprovalRequest) {
			test.fileSystem[testSigningBuffer] = testTagPayload
		}, false},
		{"a key that always asks", func(test *signingTest, request *sshagent.ApprovalRequest) { request.IsAlwaysAskKey = true }, false},
		{"a forwarded request", func(test *signingTest, request *sshagent.ApprovalRequest) { request.IsForwarded = true }, false},
	}
	for _, testCase := range cases {
		test := createSigningTest(t, 600, nil)
		test.approveInDialog(t, createSigningRequest(), true)
		request := createSigningRequest()
		test.change(func(test *signingTest) { testCase.change(test, &request) })
		resultChannel := startApproval(test.approver, context.Background(), request)
		event := waitForApprovalEvent(t, test.emitter)
		if event.IsRememberable != testCase.isRememberable {
			t.Errorf("%s: offers remembering %v", testCase.name, event.IsRememberable)
		}
		test.approver.Respond(event.RequestID, false, false)
		waitForResult(t, resultChannel)
		test.change(func(test *signingTest) {
			test.requester = createTerminalRequester(createRebaseChain(testRepositoryPath))
			test.fileSystem = fakeFileSystem{testSigningBuffer: testCommitPayload}
		})
		if !test.isSignedWithoutAsking(t, createSigningRequest()) {
			t.Errorf("%s: the original scope was forgotten", testCase.name)
		}
	}
}

func TestRememberedSigningExpires(t *testing.T) {
	test := createSigningTest(t, 600, nil)
	test.approveInDialog(t, createSigningRequest(), true)
	test.change(func(test *signingTest) { test.now = test.now.Add(10*time.Minute - time.Second) })
	if !test.isSignedWithoutAsking(t, createSigningRequest()) {
		t.Fatal("forgotten before it expired")
	}
	test.change(func(test *signingTest) { test.now = test.now.Add(time.Second) })
	if test.isSignedWithoutAsking(t, createSigningRequest()) {
		t.Error("used after it expired")
	}
}

func TestLockClearsRememberedSigning(t *testing.T) {
	test := createSigningTest(t, 600, nil)
	test.approveInDialog(t, createSigningRequest(), true)
	test.approver.ClearRememberedSignings()
	if test.isSignedWithoutAsking(t, createSigningRequest()) {
		t.Fatal("remembered after Hisar locked")
	}

	resultChannel := startApproval(test.approver, context.Background(), createSigningRequest())
	event := waitForApprovalEvent(t, test.emitter)
	test.approver.ClearRememberedSignings()
	test.approver.Respond(event.RequestID, true, true)
	waitForResult(t, resultChannel)
	if test.isSignedWithoutAsking(t, createSigningRequest()) {
		t.Error("an approval asked for before the lock was remembered after it")
	}
}

func TestPhoneChoosesWhetherSigningIsRemembered(t *testing.T) {
	phone := createFakePhone()
	test := createSigningTest(t, 600, phone)

	resultChannel := startApproval(test.approver, context.Background(), createSigningRequest())
	event := waitForApprovalEvent(t, test.emitter)
	description := phone.waitForStart(t)
	if description.RememberLabel != "Also allows Terminal to sign commits in ~/src/core for 10 minutes" || description.IsRememberedByDefault {
		t.Errorf("phone request = %+v", description)
	}
	test.approver.SetRemembered(event.RequestID, true)
	phone.answer(true)
	waitForResult(t, resultChannel)
	if test.isSignedWithoutAsking(t, createSigningRequest()) {
		t.Fatal("an answer without the phone's choice was remembered")
	}
	phone.waitForStart(t)

	resultChannel = startApproval(test.approver, context.Background(), createSigningRequest())
	waitForApprovalEvent(t, test.emitter)
	phone.waitForStart(t)
	phone.answerRemembering(true, true)
	waitForResult(t, resultChannel)
	if !test.isSignedWithoutAsking(t, createSigningRequest()) {
		t.Error("the phone's choice to remember was ignored")
	}
}
