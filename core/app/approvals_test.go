package app

import (
	"context"
	"net"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/clisocket"
)

func TestSocketApprovalUnlocksHisar(t *testing.T) {
	for _, isLauncher := range []bool{false, true} {
		for _, method := range []string{"password", "Touch ID"} {
			name := "command line/" + method
			if isLauncher {
				name = "launcher/" + method
			}
			t.Run(name, func(t *testing.T) {
				const password = "approval test password"
				fixture := createUnlockedTestService(t)
				prepareTestApprovalUnlock(t, fixture, password)
				service := fixture.service
				approvals := clisocket.NewSessionApprovals(0, nil, nil)
				bridge := NewCommandLineBridge(service, approvals)
				service.Lock()
				var unlockCount, approvalCount atomic.Int32
				unlocker := NewApprovalUnlocker(service, func() { unlockCount.Add(1) })
				approver := socketApprover(func() error {
					approvalCount.Add(1)
					if method == "password" {
						return unlocker.UnlockWithPassword(password)
					}
					return unlocker.UnlockWithTouchID(func(string) error { return nil })
				})
				config := clisocket.ServerConfig{
					Handler: bridge, Approver: approver, LauncherApprover: approver,
					PeerResolver: staticPeerResolver{}, Approvals: approvals,
				}
				request := cliprotocol.Request{Type: cliprotocol.TypeListVault}
				if isLauncher {
					config.PeerResolver = raycastPeerResolver{}
					config.IdentifyApp = func(int) (clisocket.AppIdentity, bool) {
						return clisocket.AppIdentity{Path: "/Applications/Raycast.app", TeamID: "RAYCAST123"}, true
					}
					request.Type = cliprotocol.TypeLauncherListItem
				}
				server := clisocket.NewServer(config)
				serverSide, clientSide := net.Pipe()
				go server.ServeConnection(context.Background(), serverSide)
				t.Cleanup(func() { clientSide.Close() })
				client := &commandLineTest{connection: clientSide}
				if isLauncher {
					client.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeLauncherHello}, nil)
				}
				client.mustSend(t, request, nil)
				client.mustSend(t, request, nil)
				var hello cliprotocol.HelloResult
				client.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeHello}, &hello)
				if hello.State != StateUnlocked || !hello.IsApproved {
					t.Errorf("hello after approving while locked: %+v", hello)
				}
				if approvalCount.Load() != 1 || unlockCount.Load() != 1 {
					t.Errorf("%d approvals, %d unlocks; want one of each", approvalCount.Load(), unlockCount.Load())
				}
			})
		}
	}
}

func TestCompactSSHApprovalIsSavedAndKeptOnSignOut(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "desktop.json")
	service, _, _ := createTestService(t, statePath, createMemorySecretKeyStore())
	preference := NewApprovalPreference(service)
	if preference.IsCompactSSHApproval() {
		t.Error("compact approvals are on by default")
	}
	if err := preference.SetCompactSSHApproval(true); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SignOut(); err != nil {
		t.Fatal(err)
	}
	reloaded, _, _ := createTestService(t, statePath, createMemorySecretKeyStore())
	reloadedPreference := NewApprovalPreference(reloaded)
	if !reloadedPreference.IsCompactSSHApproval() {
		t.Error("the compact approval switch was not kept")
	}
	if err := reloadedPreference.SetCompactSSHApproval(false); err != nil || reloadedPreference.IsCompactSSHApproval() {
		t.Errorf("turning compact approvals off: %v", err)
	}
}

func TestPhoneApprovalSwitchIsSavedAndKeptOnSignOut(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "desktop.json")
	service, _, _ := createTestService(t, statePath, createMemorySecretKeyStore())
	preference := NewApprovalPreference(service)
	if !preference.IsPhoneApproval() {
		t.Error("phone approvals are off by default")
	}
	if err := preference.SetPhoneApproval(false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SignOut(); err != nil {
		t.Fatal(err)
	}
	reloaded, _, _ := createTestService(t, statePath, createMemorySecretKeyStore())
	reloadedPreference := NewApprovalPreference(reloaded)
	if reloadedPreference.IsPhoneApproval() {
		t.Error("the phone approval switch was not kept")
	}
	if err := reloadedPreference.SetPhoneApproval(true); err != nil || !reloadedPreference.IsPhoneApproval() {
		t.Errorf("turning phone approvals on: %v", err)
	}
}

func TestSSHRememberIsAnHourByDefaultAndKeptOnSignOut(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "desktop.json")
	service, _, _ := createTestService(t, statePath, createMemorySecretKeyStore())
	preference := NewApprovalPreference(service)
	if seconds := preference.GetSSHRememberSeconds(); seconds != 3600 {
		t.Errorf("SSH approvals are remembered for %d seconds by default", seconds)
	}
	if err := preference.SetSSHRememberSeconds(600); err == nil {
		t.Error("an unlisted duration was accepted")
	}
	if err := preference.SetSSHRememberSeconds(SSHRememberUntilLock); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SignOut(); err != nil {
		t.Fatal(err)
	}
	reloaded, _, _ := createTestService(t, statePath, createMemorySecretKeyStore())
	reloadedPreference := NewApprovalPreference(reloaded)
	if seconds := reloadedPreference.GetSSHRememberSeconds(); seconds != SSHRememberUntilLock {
		t.Errorf("kept %d seconds", seconds)
	}
	if err := reloadedPreference.SetSSHRememberSeconds(3600); err != nil || reloadedPreference.GetSSHRememberSeconds() != 3600 {
		t.Errorf("back to 1 hour: %v", err)
	}
}

func TestSigningRememberIsOffByDefaultAndKeptOnSignOut(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "desktop.json")
	service, _, _ := createTestService(t, statePath, createMemorySecretKeyStore())
	preference := NewApprovalPreference(service)
	if seconds := preference.GetSigningRememberSeconds(); seconds != 0 {
		t.Errorf("commit signing is remembered for %d seconds by default", seconds)
	}
	if err := preference.SetSigningRememberSeconds(300); err == nil {
		t.Error("an unlisted duration was accepted")
	}
	if err := preference.SetSigningRememberSeconds(600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SignOut(); err != nil {
		t.Fatal(err)
	}
	reloaded, _, _ := createTestService(t, statePath, createMemorySecretKeyStore())
	reloadedPreference := NewApprovalPreference(reloaded)
	if seconds := reloadedPreference.GetSigningRememberSeconds(); seconds != 600 {
		t.Errorf("kept %d seconds", seconds)
	}
	if err := reloadedPreference.SetSigningRememberSeconds(0); err != nil || reloadedPreference.GetSigningRememberSeconds() != 0 {
		t.Errorf("back to always ask: %v", err)
	}
}
