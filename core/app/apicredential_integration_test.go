package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// checkAPICredentialThroughApp adds an API credential with hisar, then injects its secret and key ID into a command with
// hisar run, as a project's .env would name them.
func checkAPICredentialThroughApp(t *testing.T, binaryPath, socketPath, statePath string) {
	t.Helper()
	const secret, keyID = "sk_live_from_hisar", "key_from_hisar"
	input := "Stripe\nStripe\n" + keyID + "\n" + secret + "\napi.stripe.com\n2031-01-01\n"
	if output, err := runCommandLineBinary(t, binaryPath, socketPath, statePath, input, "add", "--type", "api-credential"); err != nil || !strings.Contains(output, `Saved "Stripe" to Personal.`) {
		t.Fatalf("hisar add --type api-credential: %v\n%s", err, output)
	}

	directory := t.TempDir()
	testBinary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	hashPath := filepath.Join(directory, "hash")
	environmentPath := filepath.Join(directory, ".env")
	os.WriteFile(environmentPath, []byte(fmt.Sprintf("DEPLOY_PASSWORD=hisar://Personal/Stripe/secret\nDEPLOY_TOKEN=hisar://Personal/Stripe/key%%20ID\n%s=print\nHASH_FILE=%s\n", runHelperEnvironment, hashPath)), 0o600)
	output, err := runCommandLineBinary(t, binaryPath, socketPath, statePath, "", "run", "--env-file", environmentPath, "--", testBinary)
	if exitCode := getExitCode(err); exitCode != 3 {
		t.Fatalf("hisar run: exit code %d, %v\n%s", exitCode, err, output)
	}
	if !strings.Contains(output, "password: <concealed by hisar>\ntoken: <concealed by hisar>\n") || strings.Contains(output, secret) || strings.Contains(output, keyID) {
		t.Errorf("hisar run output:\n%s", output)
	}
	expectedHash := sha256.Sum256([]byte(secret))
	if hash, _ := os.ReadFile(hashPath); string(hash) != hex.EncodeToString(expectedHash[:]) {
		t.Errorf("the command did not get the secret: %q", hash)
	}
	if output, err := runCommandLineBinary(t, binaryPath, socketPath, statePath, "", "read", "hisar://Personal/Stripe/host"); err != nil || output != "api.stripe.com" {
		t.Errorf("hisar read host: %q, %v", output, err)
	}
}
