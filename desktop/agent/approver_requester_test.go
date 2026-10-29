package agent

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/sshagent"
	"github.com/tryhisar/hisar/desktop/appinfo"
)

const iTermBundlePath = "/Applications/iTerm.app"

func createITermSigningRequester() appinfo.Requester {
	processes := []appinfo.Process{
		{PID: 70, Path: "/usr/bin/ssh-keygen", Arguments: []string{"/usr/bin/ssh-keygen", "-Y", "sign", "-n", "git", "-f", "/Users/developer/.ssh/signing.pub", testSigningBuffer}, WorkingDirectory: testRepositoryPath},
		createGitProcess(60, "-c", "http.extraHeader=Authorization: Bearer abc", "commit", "-S", "-m", "Fix"),
		{PID: 50, Path: "/bin/zsh", Arguments: []string{"-zsh"}},
	}
	app := appinfo.App{
		Name: "iTerm2", Path: iTermBundlePath, IconURL: "data:image/png;base64,iTerm",
		Signature: appinfo.Signature{Status: appinfo.SignatureStatusDeveloper, TeamID: "H7V7XYVQ7D", SignerName: "George Nachman (H7V7XYVQ7D)"},
	}
	return appinfo.Requester{Kind: appinfo.RequesterKindApp, Name: "iTerm2", App: app, Processes: append(processes, appinfo.Process{PID: 40, Path: "/usr/bin/login"}), ChainProcesses: processes}
}

type requesterApproverOption struct {
	requester     appinfo.Requester
	biometric     *fakeBiometric
	phone         *fakePhone
	getAppIconPNG func(bundlePath string, size int) ([]byte, error)
}

func createRequesterApprover(emitter *fakeEmitter, option requesterApproverOption) *Approver {
	config := ApproverConfig{
		Biometric: option.biometric, Emit: emitter.emit, ShowApprovalWindow: emitter.showWindow, HideApprovalWindow: emitter.hideWindow,
		ResolveRequester: func(int) appinfo.Requester { return option.requester },
		GetAppIconPNG:    option.getAppIconPNG,
		ReadFile:         fakeFileSystem{testSigningBuffer: "tree x\n\nFix\n"}.readFile,
		HomeDirectory:    testHomeDirectory,
		Timeout:          5 * time.Second,
	}
	if option.phone != nil {
		config.PhoneApprover = option.phone
	}
	return NewApprover(config)
}

func TestSigningInITermNamesITerm(t *testing.T) {
	emitter := createFakeEmitter()
	biometric := &fakeBiometric{isAvailable: true}
	approver := createRequesterApprover(emitter, requesterApproverOption{requester: createITermSigningRequester(), biometric: biometric})
	resultChannel := startApproval(approver, context.Background(), sshagent.ApprovalRequest{ClientPID: 70, ClientName: "ssh-keygen", SignatureNamespace: "git", KeyTitle: "Signing"})
	event := waitForApprovalEvent(t, emitter)
	if event.AppName != "iTerm2" || event.RequesterKind != appinfo.RequesterKindApp || event.AppPath != iTermBundlePath || event.AppIconURL != "data:image/png;base64,iTerm" {
		t.Errorf("app: %+v", event)
	}
	if event.AppSignature != appinfo.SignatureStatusDeveloper || event.AppSigner != "iTerm2 is signed by George Nachman (H7V7XYVQ7D)" || event.AppWarning != "" {
		t.Errorf("signature %q %q %q", event.AppSignature, event.AppSigner, event.AppWarning)
	}
	names := []string{}
	for _, chainProcess := range event.ChainProcesses {
		names = append(names, chainProcess.Name)
	}
	if !slices.Equal(names, []string{"ssh-keygen", "git", "zsh"}) {
		t.Errorf("chain %q", names)
	}
	if gitCommandLine := event.ChainProcesses[1].CommandLine; strings.Contains(gitCommandLine, "abc") || !strings.Contains(gitCommandLine, "commit -S") {
		t.Errorf("git's command line %q", gitCommandLine)
	}
	if event.Purpose != PurposeSignCommit || event.CommitSubject != "Fix" || event.CommandLine == "" {
		t.Errorf("purpose from the resolved chain: %+v", event)
	}
	approver.Respond(event.RequestID, true, false)
	waitForResult(t, resultChannel)
	if reasons := biometric.listReasons(); len(reasons) != 1 || reasons[0] != `allow "iTerm2" to sign a commit` {
		t.Errorf("Touch ID reason %q", reasons)
	}
}

func TestUnsignedAppShowsAWarning(t *testing.T) {
	for _, testCase := range []struct {
		status      string
		wantSigner  string
		wantWarning string
	}{
		{appinfo.SignatureStatusUnsigned, "iTerm2 isn't signed", "iTerm2 is not signed"},
		{appinfo.SignatureStatusInvalid, "iTerm2's signature isn't valid", "The code signature of iTerm2 does not verify"},
		{appinfo.SignatureStatusApple, "iTerm2 is signed by Apple", ""},
		{"", "", ""},
	} {
		requester := createITermSigningRequester()
		requester.App.Signature = appinfo.Signature{Status: testCase.status}
		emitter := createFakeEmitter()
		phone := createFakePhone()
		approver := createRequesterApprover(emitter, requesterApproverOption{requester: requester, biometric: &fakeBiometric{isAvailable: true}, phone: phone})
		resultChannel := startApproval(approver, context.Background(), sshagent.ApprovalRequest{ClientPID: 70, ClientName: "ssh-keygen", DestinationHost: "github.com"})
		event := waitForApprovalEvent(t, emitter)
		description := phone.waitForStart(t)
		if event.AppSigner != testCase.wantSigner || !strings.HasPrefix(event.AppWarning, testCase.wantWarning) || (testCase.wantWarning == "") != (event.AppWarning == "") {
			t.Errorf("%q: signer %q, warning %q", testCase.status, event.AppSigner, event.AppWarning)
		}
		if description.AppWarning != event.AppWarning || description.AppName != "iTerm2" {
			t.Errorf("%q: phone %+v", testCase.status, description)
		}
		approver.DenyAll()
		waitForResult(t, resultChannel)
	}
}

func TestRequesterWithoutAnApp(t *testing.T) {
	for _, testCase := range []struct {
		requester  appinfo.Requester
		wantReason string
	}{
		{appinfo.Requester{Kind: appinfo.RequesterKindBackground, Name: "A background process"}, `allow "ssh in a background process" to sign in to github.com`},
		{appinfo.Requester{Kind: appinfo.RequesterKindSSHSession, Name: "An SSH session from 192.0.2.10"}, `allow "ssh in an SSH session from 192.0.2.10" to sign in to github.com`},
		{appinfo.Requester{Kind: appinfo.RequesterKindMultiplexer, Name: "tmux"}, `allow "ssh in tmux" to sign in to github.com`},
	} {
		emitter := createFakeEmitter()
		biometric := &fakeBiometric{isAvailable: true}
		approver := createRequesterApprover(emitter, requesterApproverOption{requester: testCase.requester, biometric: biometric})
		resultChannel := startApproval(approver, context.Background(), sshagent.ApprovalRequest{ClientPID: 70, ClientName: "ssh", DestinationHost: "github.com"})
		event := waitForApprovalEvent(t, emitter)
		if event.AppName != testCase.requester.Name || event.RequesterKind != testCase.requester.Kind || event.AppIconURL != "" || event.AppSigner != "" || event.AppWarning != "" {
			t.Errorf("%s: %+v", testCase.requester.Name, event)
		}
		approver.Respond(event.RequestID, true, false)
		waitForResult(t, resultChannel)
		if reasons := biometric.listReasons(); len(reasons) != 1 || reasons[0] != testCase.wantReason {
			t.Errorf("Touch ID reason %q", reasons)
		}
	}
}

// A Linux app its desktop entry names shows with its name and icon, but it is no app identity: Touch ID names the
// program in it, as for a process without an app, and nothing is scoped to the app.
func TestDesktopAppIsShownButNotAnIdentity(t *testing.T) {
	emitter := createFakeEmitter()
	biometric := &fakeBiometric{isAvailable: true}
	requester := appinfo.Requester{Kind: appinfo.RequesterKindDesktopApp, Name: "Konsole", App: appinfo.App{Name: "Konsole", IconURL: "data:image/png;base64,Konsole"}}
	approver := createRequesterApprover(emitter, requesterApproverOption{requester: requester, biometric: biometric})
	resultChannel := startApproval(approver, context.Background(), sshagent.ApprovalRequest{ClientPID: 70, ClientName: "ssh", DestinationHost: "github.com"})
	event := waitForApprovalEvent(t, emitter)
	if event.AppName != "Konsole" || event.RequesterKind != appinfo.RequesterKindDesktopApp || event.AppIconURL != "data:image/png;base64,Konsole" ||
		event.AppPath != "" || event.AppSignature != "" || event.AppSigner != "" || event.AppWarning != "" {
		t.Errorf("event: %+v", event)
	}
	approver.Respond(event.RequestID, true, false)
	waitForResult(t, resultChannel)
	if reasons := biometric.listReasons(); len(reasons) != 1 || reasons[0] != `allow "ssh in Konsole" to sign in to github.com` {
		t.Errorf("Touch ID reason %q", reasons)
	}
}

func TestCommandLineAndAgentNameTheirApp(t *testing.T) {
	emitter := createFakeEmitter()
	phone := createFakePhone()
	approver := createRequesterApprover(emitter, requesterApproverOption{requester: createITermSigningRequester(), biometric: &fakeBiometric{isAvailable: true}, phone: phone})
	resultChannel := startCommandLineApproval(approver, context.Background())
	event := waitForApprovalEvent(t, emitter)
	if event.AppName != "iTerm2" || event.AppSigner == "" || len(event.ChainProcesses) != 3 || phone.waitForStart(t).AppName != "hisar in iTerm2" {
		t.Errorf("command line: %+v", event)
	}
	approver.DenyAll()
	waitForCommandLineResult(t, resultChannel)

	background := createRequesterApprover(emitter, requesterApproverOption{requester: appinfo.Requester{Kind: appinfo.RequesterKindBackground, Name: "A background process"}, biometric: &fakeBiometric{isAvailable: true}, phone: phone})
	agentChannel := startAgentApproval(background, createAgentRequest("connect"))
	agentEvent := waitForApprovalEvent(t, emitter)
	if describeAgentRequester(agentEvent) != "Claude Code (via hisar mcp) in a background process" {
		t.Errorf("agent requester %q", describeAgentRequester(agentEvent))
	}
	background.DenyAll()
	waitForAgentResult(t, agentChannel)
}

func TestPhoneGetsTheAppIconWhenSmall(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		icon     []byte
		err      error
		wantIcon bool
	}{
		{"small", []byte("png"), nil, true},
		{"too large", make([]byte, maximumPhoneIconSize+1), nil, false},
		{"not rendered", nil, errors.New("no icon"), false},
	} {
		emitter := createFakeEmitter()
		phone := createFakePhone()
		requestedSizes := []int{}
		approver := createRequesterApprover(emitter, requesterApproverOption{
			requester: createITermSigningRequester(), biometric: &fakeBiometric{isAvailable: true}, phone: phone,
			getAppIconPNG: func(bundlePath string, size int) ([]byte, error) {
				requestedSizes = append(requestedSizes, size)
				return testCase.icon, testCase.err
			},
		})
		resultChannel := startApproval(approver, context.Background(), sshagent.ApprovalRequest{ClientPID: 70, ClientName: "ssh", DestinationHost: "github.com"})
		waitForApprovalEvent(t, emitter)
		description := phone.waitForStart(t)
		if (description.AppIcon != nil) != testCase.wantIcon || (testCase.wantIcon && !reflect.DeepEqual(description.AppIcon, testCase.icon)) || !slices.Equal(requestedSizes, []int{appinfo.PhoneIconSize}) {
			t.Errorf("%s: icon %d bytes, sizes %v", testCase.name, len(description.AppIcon), requestedSizes)
		}
		approver.DenyAll()
		waitForResult(t, resultChannel)
	}
}

func TestLowerFirstLetter(t *testing.T) {
	for text, want := range map[string]string{"A background process": "a background process", "An SSH session": "an SSH session", "SSH": "SSH", "": "", "tmux": "tmux"} {
		if got := lowerFirstLetter(text); got != want {
			t.Errorf("%q: %q", text, got)
		}
	}
}
