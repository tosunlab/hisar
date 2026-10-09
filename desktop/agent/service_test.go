package agent

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh/agent"

	"github.com/tryhisar/hisar/core/sshagent"
)

type emptyKeySource struct{}

func (emptyKeySource) ListAllKey(context.Context) ([]sshagent.Key, error) { return nil, nil }

// createShortTempDirectory keeps unix socket paths under the macOS limit of about 104 bytes.
func createShortTempDirectory(t *testing.T) string {
	t.Helper()
	directory, err := os.MkdirTemp("", "hisar")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(directory) })
	return directory
}

func waitForRunning(t *testing.T, service *Service, wantRunning bool) Status {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		status := service.GetStatus()
		if status.IsRunning == wantRunning {
			return status
		}
		if time.Now().After(deadline) {
			t.Fatalf("running = %v, want %v: %+v", status.IsRunning, wantRunning, status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestServeAndStop(t *testing.T) {
	homeDirectory := createShortTempDirectory(t)
	approver := CreateApprover(ApproverConfig{Biometric: &fakeBiometric{isAvailable: true}})
	service := CreateService(Config{
		HomeDirectory: homeDirectory,
		Server:        sshagent.CreateServer(emptyKeySource{}, approver, nil),
		Approver:      approver,
	})
	socketPath := filepath.Join(homeDirectory, ".hisar", "agent.sock")
	status := service.GetStatus()
	if status.SocketPath != socketPath || status.IsRunning || status.IsSSHConfigured || !status.IsTouchIDApproval {
		t.Fatalf("status before serving: %+v", status)
	}
	if status.ConfigPath != filepath.Join(homeDirectory, ".ssh", "config") || !strings.Contains(status.ConfigSection, `IdentityAgent "~/.hisar/agent.sock"`) {
		t.Errorf("config in status: %+v", status)
	}

	ctx, cancel := context.WithCancel(context.Background())
	serveResult := make(chan error, 1)
	go func() { serveResult <- Serve(ctx, service) }()
	waitForRunning(t, service, true)

	connection, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	allKey, err := agent.NewClient(connection).List()
	connection.Close()
	if err != nil || len(allKey) != 0 {
		t.Errorf("list: %v %v", allKey, err)
	}

	cancel()
	select {
	case err := <-serveResult:
		if err != nil {
			t.Errorf("serve: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("agent did not stop")
	}
	if status := service.GetStatus(); status.IsRunning || status.ErrorMessage != "" {
		t.Errorf("status after stop: %+v", status)
	}
	if _, err := os.Lstat(socketPath); !os.IsNotExist(err) {
		t.Errorf("socket left behind: %v", err)
	}
}

func TestServeReportsListenError(t *testing.T) {
	homeDirectory := createShortTempDirectory(t)
	socketPath := filepath.Join(homeDirectory, "agent.sock")
	if err := os.WriteFile(socketPath, []byte("not a socket"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := CreateService(Config{HomeDirectory: homeDirectory, SocketPath: socketPath, Server: sshagent.CreateServer(emptyKeySource{}, CreateApprover(ApproverConfig{}), nil)})
	if err := Serve(context.Background(), service); err == nil {
		t.Fatal("serve on a regular file succeeded")
	}
	status := service.GetStatus()
	if status.IsRunning || !strings.Contains(status.ErrorMessage, "not a socket") {
		t.Errorf("status = %+v", status)
	}
	if !strings.Contains(status.ConfigSection, `IdentityAgent "~/agent.sock"`) {
		t.Errorf("section = %q", status.ConfigSection)
	}

	if err := Serve(context.Background(), CreateService(Config{HomeDirectory: homeDirectory})); err == nil {
		t.Error("serve without a server succeeded")
	}
}

func TestServiceConfiguresSSHConfig(t *testing.T) {
	homeDirectory := t.TempDir()
	service := CreateService(Config{HomeDirectory: homeDirectory, Now: func() time.Time { return testTime }})
	status, err := service.ConfigureSSHConfig()
	if err != nil || !status.IsSSHConfigured || status.BackupPath != "" {
		t.Fatalf("configure: %+v %v", status, err)
	}
	status, err = service.RemoveSSHConfig()
	if err != nil || status.IsSSHConfigured || !strings.Contains(status.BackupPath, ".hisar-backup-20261008-093000") {
		t.Fatalf("remove: %+v %v", status, err)
	}
	if status.IsTouchIDApproval {
		t.Error("Touch ID approval without an approver")
	}
}

func TestServiceRespondsToApproval(t *testing.T) {
	emitter := createFakeEmitter()
	approver := CreateApprover(ApproverConfig{Emit: emitter.emit})
	service := CreateService(Config{HomeDirectory: t.TempDir(), Approver: approver})
	resultChannel := startApproval(approver, context.Background(), createTestRequest())
	approvalEvent := waitForApprovalEvent(t, emitter)
	if allPending := service.ListAllPendingSSHApproval(); len(allPending) != 1 {
		t.Errorf("pending = %+v", allPending)
	}
	service.RespondSSHApproval("unknown", false, false)
	service.RespondSSHApproval(approvalEvent.RequestID, true, true)
	if result := waitForResult(t, resultChannel); !result.result.IsAllowed || !result.result.IsForAllClient {
		t.Errorf("result = %+v", result)
	}
	withoutApprover := CreateService(Config{HomeDirectory: t.TempDir()})
	withoutApprover.RespondSSHApproval("any", true, false)
	if allPending := withoutApprover.ListAllPendingSSHApproval(); len(allPending) != 0 {
		t.Errorf("pending without approver = %+v", allPending)
	}
}
