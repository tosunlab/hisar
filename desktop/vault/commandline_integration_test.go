package vault

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/desktop/commandline"
)

// unlockingApprover stands in for the approval window: it types a wrong and then the right password when Hisar is locked.
type unlockingApprover struct {
	mutex                   sync.Mutex
	unlocker                *ApprovalUnlocker
	password                string
	allRequest              []commandline.ApprovalRequest
	allWrongPasswordMessage []string
}

func (approver *unlockingApprover) ApproveCommandLine(_ context.Context, request commandline.ApprovalRequest) error {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	approver.allRequest = append(approver.allRequest, request)
	if !approver.unlocker.IsLocked() {
		return nil
	}
	if err := approver.unlocker.UnlockWithPassword("not the master password"); err != nil {
		approver.allWrongPasswordMessage = append(approver.allWrongPasswordMessage, err.Error())
	}
	return approver.unlocker.UnlockWithPassword(approver.password)
}

func (approver *unlockingApprover) getRequestCount() int {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	return len(approver.allRequest)
}

func runCommandLineBinary(t *testing.T, binaryPath, socketPath, statePath, input string, allArg ...string) (string, error) {
	t.Helper()
	command := exec.Command(binaryPath, allArg...)
	command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), cliprotocol.SocketEnvironment + "=" + socketPath, "HISAR_STATE=" + statePath, "HISAR_SECRET_KEY=-", "HISAR_NO_LAUNCH=1"}
	command.Stdin = strings.NewReader(input)
	output, err := command.CombinedOutput()
	return string(output), err
}

// TestIntegrationCommandLineThroughApp runs the built hisar against the app backend signed in to a real server.
func TestIntegrationCommandLineThroughApp(t *testing.T) {
	serverURL, inviteLink, binaryPath := os.Getenv("HISAR_TEST_SERVER_URL"), os.Getenv("HISAR_TEST_CLI_INVITE_LINK"), os.Getenv("HISAR_TEST_CLI_BINARY")
	if serverURL == "" || inviteLink == "" || binaryPath == "" {
		t.Skip("HISAR_TEST_SERVER_URL, HISAR_TEST_CLI_INVITE_LINK and HISAR_TEST_CLI_BINARY not set")
	}
	const password = "command line master password"
	service, _, _ := createTestService(t, filepath.Join(t.TempDir(), "desktop.json"), createMemorySecretKeyStore())
	if _, err := service.SignUp(inviteLink, "cli@example.com", "Command Line", password); err != nil {
		t.Fatal(err)
	}
	service.Lock()

	socketDirectory, err := os.MkdirTemp("/tmp", "hisar-cli-integration-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socketDirectory) })
	socketPath := filepath.Join(socketDirectory, "hisar", "cli.sock")
	statePath := filepath.Join(socketDirectory, "standalone", "state.json")
	unlockCount := 0
	approver := &unlockingApprover{unlocker: CreateApprovalUnlocker(service, func() { unlockCount++ }), password: password}
	approvals := commandline.CreateSessionApprovals(0, nil)
	runner := commandline.CreateRunner(commandline.CreateServer(commandline.ServerConfig{
		Handler: CreateCommandLineBridge(service, approvals), Approver: approver, Approvals: approvals,
	}), socketPath)
	runner.Start()
	t.Cleanup(runner.Stop)
	for range 200 {
		if connection, err := net.Dial("unix", socketPath); err == nil {
			connection.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	run := func(input string, allArg ...string) string {
		t.Helper()
		output, err := runCommandLineBinary(t, binaryPath, socketPath, statePath, input, allArg...)
		if err != nil {
			t.Fatalf("hisar %s: %v\n%s", strings.Join(allArg, " "), err, output)
		}
		return output
	}

	if output := run("", "status"); !strings.Contains(output, "mode:    Hisar app ("+socketPath+")") || !strings.Contains(output, "cli@example.com on "+serverURL) || !strings.Contains(output, "locked") {
		t.Errorf("status while locked:\n%s", output)
	}
	if output := run("GitHub\nalice\nhttps://github.com\nfrom-the-command-line\nadded through the app\n", "add"); !strings.Contains(output, "Unlock Hisar in its approval window") || !strings.Contains(output, `Saved "GitHub" to Personal.`) {
		t.Errorf("add:\n%s", output)
	}
	if service.GetStatus().State != StateUnlocked || unlockCount != 1 {
		t.Fatalf("the approval did not unlock Hisar: %s, %d", service.GetStatus().State, unlockCount)
	}
	if len(approver.allWrongPasswordMessage) != 1 || approver.allWrongPasswordMessage[0] != "Incorrect master password." {
		t.Errorf("wrong password: %v", approver.allWrongPasswordMessage)
	}
	if output := run("", "get", "github", "-f", "password"); output != "from-the-command-line\n" {
		t.Errorf("get -f password: %q", output)
	}
	if output := run("", "ls"); !strings.Contains(output, "GitHub") || !strings.Contains(output, "https://github.com") {
		t.Errorf("ls:\n%s", output)
	}
	if output := run("", "whoami"); !strings.Contains(output, "Command Line <cli@example.com>") || !strings.Contains(output, "server:  "+serverURL) {
		t.Errorf("whoami:\n%s", output)
	}
	if output := run("\n\n\n\nedited through the app\n", "edit", "github"); !strings.Contains(output, `Saved "GitHub".`) {
		t.Errorf("edit:\n%s", output)
	}
	allItem, err := service.ListAllItem(findPersonalVaultID(t, service))
	if err != nil || len(allItem) != 1 {
		t.Fatalf("items in the app: %+v %v", allItem, err)
	}
	if detail, err := service.GetItem(findPersonalVaultID(t, service), allItem[0].ID); err != nil || detail.Notes != "edited through the app" || detail.Password != "from-the-command-line" {
		t.Errorf("edited item in the app: %+v %v", detail, err)
	}
	if count := approver.getRequestCount(); count != 1 {
		t.Errorf("approval asked %d times in one session", count)
	}
	if output, err := runCommandLineBinary(t, binaryPath, socketPath, statePath, "", "login"); err == nil || !strings.Contains(output, "hisar --standalone login") {
		t.Errorf("login through the app: %v\n%s", err, output)
	}
	if _, err := os.Stat(statePath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the app mode wrote a standalone state file: %v", err)
	}

	service.Lock()
	if output := run("", "vaults"); !strings.Contains(output, "Personal") {
		t.Errorf("vaults after the lock:\n%s", output)
	}
	if count := approver.getRequestCount(); count != 2 || unlockCount != 2 {
		t.Errorf("after the lock: %d approvals, %d unlocks", count, unlockCount)
	}

	checkSecretReferenceThroughApp(t, service, approver, binaryPath, socketPath, statePath)
	checkShareLinkThroughApp(t, approver, binaryPath, socketPath, statePath)

	runner.Stop()
	if output := run("", "status"); !strings.Contains(output, "mode:    standalone") || !strings.Contains(output, "not signed in") {
		t.Errorf("status with the switch off:\n%s", output)
	}
}

func findPersonalVaultID(t *testing.T, service *Service) string {
	t.Helper()
	allVault, err := service.ListAllVault()
	if err != nil {
		t.Fatal(err)
	}
	for _, vault := range allVault {
		if vault.Type == "personal" {
			return vault.ID
		}
	}
	t.Fatal("no personal vault")
	return ""
}
