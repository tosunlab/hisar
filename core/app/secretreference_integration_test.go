package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const runHelperEnvironment = "HISAR_TEST_RUN_HELPER"

func TestMain(m *testing.M) {
	if os.Getenv(runHelperEnvironment) == "print" {
		os.Exit(runPrintingHelper())
	}
	os.Exit(m.Run())
}

// runPrintingHelper is the command hisar run starts: it prints the secret, records only its hash and exits 3.
func runPrintingHelper() int {
	secret := os.Getenv("DEPLOY_PASSWORD")
	fmt.Printf("password: %s\ntoken: %s\n", secret, os.Getenv("DEPLOY_TOKEN"))
	hash := sha256.Sum256([]byte(secret))
	os.WriteFile(os.Getenv("HASH_FILE"), []byte(hex.EncodeToString(hash[:])), 0o600)
	return 3
}

func getExitCode(err error) int {
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode()
	}
	return 0
}

// checkSecretReferenceThroughApp runs hisar run and hisar read against the app; Hisar is locked when it starts.
func checkSecretReferenceThroughApp(t *testing.T, service *Service, approver *unlockingApprover, binaryPath, socketPath, statePath string) {
	t.Helper()
	const password, token = "deploy-password-from-hisar", "deploy-token-from-hisar"
	if err := approver.unlocker.UnlockWithPassword(approver.password); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveItem(findPersonalVaultID(t, service), ItemDetail{
		Title: "Deploy", Password: password, URLs: []string{}, Fields: []ItemField{{Name: "API token", Value: token, Concealed: true}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GenerateSSHKey(findPersonalVaultID(t, service), "Deploy key", "ed25519"); err != nil {
		t.Fatal(err)
	}
	service.Lock()
	approvalCount := approver.getRequestCount()

	directory := t.TempDir()
	testBinary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	hashPath := filepath.Join(directory, "hash")
	environmentPath := filepath.Join(directory, ".env")
	os.WriteFile(environmentPath, []byte(fmt.Sprintf("# references only\nDEPLOY_PASSWORD=hisar://Personal/Deploy/password\nexport DEPLOY_TOKEN=\"hisar://Personal/Deploy/API%%20token\"\n%s=print\nHASH_FILE=%s\n", runHelperEnvironment, hashPath)), 0o600)
	output, err := runCommandLineBinary(t, binaryPath, socketPath, statePath, "", "run", "--env-file", environmentPath, "--", testBinary)
	if exitCode := getExitCode(err); exitCode != 3 {
		t.Fatalf("hisar run: exit code %d, %v\n%s", exitCode, err, output)
	}
	if !strings.Contains(output, "password: <concealed by hisar>\ntoken: <concealed by hisar>\n") || strings.Contains(output, password) || strings.Contains(output, token) {
		t.Errorf("hisar run output:\n%s", output)
	}
	expectedHash := sha256.Sum256([]byte(password))
	if hash, _ := os.ReadFile(hashPath); string(hash) != hex.EncodeToString(expectedHash[:]) {
		t.Errorf("the command did not get the password: %q", hash)
	}
	if count := approver.getRequestCount(); count != approvalCount+1 {
		t.Fatalf("hisar run asked %d times", count-approvalCount)
	}
	approver.mutex.Lock()
	request := approver.requests[len(approver.requests)-1]
	approver.mutex.Unlock()
	if !request.IsSecretRequest || !slices.Equal(request.SecretNames, []string{"Personal › Deploy › password", "Personal › Deploy › API token"}) || !slices.Equal(request.CommandLine, []string{testBinary}) {
		t.Errorf("approval request: %+v", request)
	}

	if output, err := runCommandLineBinary(t, binaryPath, socketPath, statePath, "", "read", "hisar://Personal/Deploy/API%20token"); err != nil || output != token {
		t.Errorf("hisar read: %q, %v", output, err)
	}

	os.Remove(hashPath)
	missingPath := filepath.Join(directory, "missing.env")
	os.WriteFile(missingPath, []byte(fmt.Sprintf("DEPLOY_PASSWORD=hisar://Personal/Nothing/password\n%s=print\nHASH_FILE=%s\n", runHelperEnvironment, hashPath)), 0o600)
	output, err = runCommandLineBinary(t, binaryPath, socketPath, statePath, "", "run", "--env-file", missingPath, "--", testBinary)
	if exitCode := getExitCode(err); exitCode != 1 || !strings.Contains(output, `hisar://Personal/Nothing/password: no item in vault "Personal" is titled "Nothing"`) {
		t.Errorf("unknown reference: exit code %d\n%s", exitCode, output)
	}
	if _, err := os.Stat(hashPath); !errors.Is(err, os.ErrNotExist) {
		t.Error("the command ran although a reference did not resolve")
	}

	privateKeyPath := filepath.Join(directory, "private-key.env")
	os.WriteFile(privateKeyPath, []byte(fmt.Sprintf("DEPLOY_PASSWORD=hisar://Personal/Deploy%%20key/private%%20key\n%s=print\nHASH_FILE=%s\n", runHelperEnvironment, hashPath)), 0o600)
	output, err = runCommandLineBinary(t, binaryPath, socketPath, statePath, "", "run", "--env-file", privateKeyPath, "--", testBinary)
	if exitCode := getExitCode(err); exitCode != 1 || !strings.Contains(output, "hisar://Personal/Deploy%20key/private%20key: SSH private keys never leave Hisar; use the Hisar SSH agent instead") || strings.Contains(output, "BEGIN OPENSSH") {
		t.Errorf("private key reference: exit code %d\n%s", exitCode, output)
	}
	if _, err := os.Stat(hashPath); !errors.Is(err, os.ErrNotExist) {
		t.Error("the command ran with a private key reference")
	}
}
