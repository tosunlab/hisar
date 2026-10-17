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

const (
	raycastBundlePath = "/Applications/Raycast.app"
	raycastNodePath   = "/Users/developer/Library/Application Support/com.raycast.macos/node/bin/node"
	raycastTeamID     = "A1B2C3D4E5"
)

func createRaycastRequester() appinfo.Requester {
	app := appinfo.App{
		Name: "Raycast", Path: raycastBundlePath, IconURL: "data:image/png;base64,Raycast",
		Signature: appinfo.Signature{Status: appinfo.SignatureStatusDeveloper, TeamID: raycastTeamID, SignerName: "Raycast Technologies Inc (A1B2C3D4E5)"},
	}
	node := appinfo.Process{PID: 71849, Path: raycastNodePath, Arguments: []string{raycastNodePath, "--max-old-space-size=4096"}}
	raycast := appinfo.Process{PID: 71827, Path: raycastBundlePath + "/Contents/MacOS/Raycast"}
	return appinfo.Requester{Kind: appinfo.RequesterKindApp, Name: "Raycast", App: app, Processes: []appinfo.Process{node, raycast}, ChainProcesses: []appinfo.Process{node}}
}

func createLauncherApprover(emitter *fakeEmitter, biometric Biometric, unlocker Unlocker, phone *fakePhone, timeout time.Duration) *Approver {
	config := ApproverConfig{
		Biometric: biometric, Emit: emitter.emit, ShowApprovalWindow: emitter.showWindow, HideApprovalWindow: emitter.hideWindow,
		ResolveRequester:  func(int) appinfo.Requester { return createRaycastRequester() },
		GetAppIconPNG:     func(string, int) ([]byte, error) { return []byte("Raycast PNG"), nil },
		IsCompactApproval: func() bool { return true },
		Unlocker:          unlocker,
		Timeout:           timeout,
	}
	if phone != nil {
		config.PhoneApprover = phone
	}
	return NewApprover(config)
}

func createLauncherRequest() clisocket.LauncherApprovalRequest {
	return clisocket.LauncherApprovalRequest{
		App:       clisocket.AppIdentity{Path: raycastBundlePath, TeamID: raycastTeamID},
		ClientPID: 71849, ClientPath: raycastNodePath, ParentPID: 71827,
	}
}

func startLauncherApproval(approver *Approver, ctx context.Context) chan error {
	resultChannel := make(chan error, 1)
	go func() { resultChannel <- approver.ApproveLauncher(ctx, createLauncherRequest()) }()
	return resultChannel
}

func TestLauncherApprovalNamesTheAppAndWhatItAllows(t *testing.T) {
	biometric := &fakeBiometric{isAvailable: true}
	emitter := createFakeEmitter()
	approver := createLauncherApprover(emitter, biometric, &fakeUnlocker{}, nil, 0)
	resultChannel := startLauncherApproval(approver, context.Background())
	event := waitForApprovalEvent(t, emitter)
	if event.Kind != ApprovalKindLauncher || event.AppName != "Raycast" || event.AppIconURL != "data:image/png;base64,Raycast" || event.AppPath != raycastBundlePath ||
		event.AppSigner != "Signed by Raycast Technologies Inc (A1B2C3D4E5)" || event.AppWarning != "" || event.ClientPath != raycastNodePath ||
		event.Action != "search Hisar and copy or paste passwords and one-time codes" || event.Coverage != "app" ||
		event.IsRememberable || event.IsUnlockRequired || event.TerminalName != "" || event.KeyTitle != "" || len(event.SecretNames) != 0 {
		t.Errorf("event = %+v", event)
	}
	if len(event.ChainProcesses) != 1 || event.ChainProcesses[0].Name != "node" {
		t.Errorf("chain %+v", event.ChainProcesses)
	}
	approver.presenter.waitUntilIdle()
	if event.IsCompact || !slices.Equal(emitter.listShownCompact(), []bool{false}) {
		t.Errorf("a launcher got the compact card: %v %v", event.IsCompact, emitter.listShownCompact())
	}
	approver.Respond(event.RequestID, true, false)
	if err := waitForCommandLineResult(t, resultChannel); err != nil {
		t.Fatalf("approval: %v", err)
	}
	if reasons := biometric.listReasons(); !slices.Equal(reasons, []string{`allow "Raycast" to search Hisar and copy or paste passwords and one-time codes`}) {
		t.Errorf("Touch ID reasons = %q", reasons)
	}
}

func TestLauncherApprovalUnlocksALockedHisar(t *testing.T) {
	biometric := &fakeBiometric{isAvailable: true}
	emitter := createFakeEmitter()
	unlocker := &fakeUnlocker{isLocked: true, isTouchIDUnlockReady: true}
	approver := createLauncherApprover(emitter, biometric, unlocker, createFakePhone(), 0)
	resultChannel := startLauncherApproval(approver, context.Background())
	event := waitForApprovalEvent(t, emitter)
	if !event.IsUnlockRequired || event.IsPasswordUnlock || event.IsPhoneApproval {
		t.Fatalf("event = %+v", event)
	}
	approver.Respond(event.RequestID, true, false)
	if err := waitForCommandLineResult(t, resultChannel); err != nil {
		t.Fatalf("approval: %v", err)
	}
	if reasons := biometric.listReasons(); unlocker.IsLocked() || !slices.Equal(reasons, []string{`unlock Hisar for "Raycast"`}) {
		t.Errorf("locked %v, Touch ID reasons %q", unlocker.IsLocked(), reasons)
	}
}

func TestLauncherApprovalOnThePhone(t *testing.T) {
	for _, isApproved := range []bool{true, false} {
		emitter := createFakeEmitter()
		phone := createFakePhone()
		approver := createLauncherApprover(emitter, &fakeBiometric{isAvailable: true}, &fakeUnlocker{}, phone, 5*time.Second)
		resultChannel := startLauncherApproval(approver, context.Background())
		waitForApprovalEvent(t, emitter)
		want := approval.Description{
			Kind:    approval.KindLauncher,
			AppName: "Raycast",
			AppIcon: []byte("Raycast PNG"),
			Purpose: "search Hisar and copy or paste passwords and one-time codes",
		}
		if description := phone.waitForStart(t); !reflect.DeepEqual(description, want) {
			t.Errorf("launcher request on the phone = %+v", description)
		}
		phone.answer(isApproved)
		if err := waitForCommandLineResult(t, resultChannel); (err == nil) != isApproved || (!isApproved && !errors.Is(err, clisocket.ErrDenied)) {
			t.Errorf("answered %v on the phone: %v", isApproved, err)
		}
	}
}

func TestLauncherApprovalDenyTimeoutAndDisconnect(t *testing.T) {
	emitter := createFakeEmitter()
	approver := createLauncherApprover(emitter, &fakeBiometric{isAvailable: true}, &fakeUnlocker{}, nil, 0)
	resultChannel := startLauncherApproval(approver, context.Background())
	approver.Respond(waitForApprovalEvent(t, emitter).RequestID, false, false)
	var timeoutError clisocket.TimeoutError
	if err := waitForCommandLineResult(t, resultChannel); !errors.Is(err, clisocket.ErrDenied) || errors.As(err, &timeoutError) {
		t.Errorf("deny: %v", err)
	}

	timeoutApprover := createLauncherApprover(createFakeEmitter(), &fakeBiometric{isAvailable: true}, &fakeUnlocker{}, nil, 20*time.Millisecond)
	if err := timeoutApprover.ApproveLauncher(context.Background(), createLauncherRequest()); !errors.As(err, &timeoutError) || timeoutError.Timeout != 20*time.Millisecond || !errors.Is(err, clisocket.ErrDenied) {
		t.Errorf("timeout: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	disconnectEmitter := createFakeEmitter()
	resultChannel = startLauncherApproval(createLauncherApprover(disconnectEmitter, &fakeBiometric{isAvailable: true}, &fakeUnlocker{}, nil, 0), ctx)
	waitForApprovalEvent(t, disconnectEmitter)
	cancel()
	if err := waitForCommandLineResult(t, resultChannel); !errors.Is(err, clisocket.ErrDenied) || errors.As(err, &timeoutError) {
		t.Errorf("launcher gone: %v", err)
	}

	if err := NewApprover(ApproverConfig{}).ApproveLauncher(context.Background(), createLauncherRequest()); !errors.Is(err, clisocket.ErrDenied) {
		t.Errorf("without an approval window: %v", err)
	}
}

func TestCommandLineApprovalSaysWhatItCovers(t *testing.T) {
	for _, coverage := range []clisocket.ApprovalCoverage{clisocket.CoverageTerminalSession, clisocket.CoverageApp, clisocket.CoverageConnection} {
		emitter := createFakeEmitter()
		approver := createCommandLineApprover(&fakeBiometric{isAvailable: true}, emitter, &fakeUnlocker{}, 0)
		request := createCommandLineRequest()
		request.Coverage = coverage
		if coverage != clisocket.CoverageTerminalSession {
			request.TerminalName = ""
		}
		resultChannel := make(chan error, 1)
		go func() { resultChannel <- approver.ApproveCommandLine(context.Background(), request) }()
		if event := waitForApprovalEvent(t, emitter); event.Coverage != string(coverage) || event.TerminalName != request.TerminalName {
			t.Errorf("%s: event = %+v", coverage, event)
		}
		approver.DenyAll()
		waitForCommandLineResult(t, resultChannel)
	}
}
