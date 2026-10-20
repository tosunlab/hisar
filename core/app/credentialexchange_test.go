package app

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/browserprotocol"
	"github.com/tryhisar/hisar/core/passkey"
)

type exchangedPasskey struct {
	privateKey   *ecdsa.PrivateKey
	credentialID []byte
	userHandle   []byte
	prfSecret    []byte
}

func createExchangedPasskey(t *testing.T, credentialID byte) exchangedPasskey {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return exchangedPasskey{privateKey: privateKey, credentialID: bytes.Repeat([]byte{credentialID}, 16), userHandle: []byte{credentialID, 1}, prfSecret: bytes.Repeat([]byte{credentialID}, 32)}
}

func (exchanged exchangedPasskey) encode(t *testing.T, relyingPartyID, userName string) map[string]any {
	t.Helper()
	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(exchanged.privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]any{
		"type": "passkey", "credentialId": encodeBase64URL(exchanged.credentialID), "rpId": relyingPartyID, "username": userName, "userDisplayName": "",
		"userHandle": encodeBase64URL(exchanged.userHandle), "key": encodeBase64URL(privateKeyDER),
		"fido2Extensions": map[string]any{"hmacCredentials": map[string]any{
			"algorithm": "hmac-sha256", "credWithUV": encodeBase64URL(exchanged.prfSecret), "credWithoutUV": encodeBase64URL(make([]byte, 32)),
		}},
	}
}

// buildCredentialExchange encodes items as the CXF JSON the iPhone app passes on from ASCredentialImportManager.
func buildCredentialExchange(t *testing.T, items ...map[string]any) []byte {
	t.Helper()
	content, err := json.Marshal(map[string]any{
		"version": map[string]any{"major": 1, "minor": 0}, "exporterRpId": "1password.com", "exporterDisplayName": "1Password", "timestamp": 1767225600,
		"accounts": []any{map[string]any{"id": "AQ", "username": "alice", "email": "alice@example.com", "collections": []any{}, "items": items}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func TestCredentialExchangeImportAddsPasskeysThatSignIn(t *testing.T) {
	testBridge := createBrowserTestBridge(t)
	service, personalID := testBridge.testService.service, testBridge.testService.personalID
	token := testBridge.pair(t, "chrome-1")
	existing, err := service.SaveItem(personalID, ItemDetail{Title: "GitHub", Username: "alice", URLs: []string{"https://github.com/login"}, Password: "kept-password"})
	if err != nil {
		t.Fatal(err)
	}
	github, gitlab := createExchangedPasskey(t, 1), createExchangedPasskey(t, 2)
	content := buildCredentialExchange(t,
		map[string]any{"id": "AQ", "title": "GitHub", "scope": map[string]any{"urls": []string{"https://github.com/login"}}, "credentials": []any{
			map[string]any{"type": "basic-auth", "username": map[string]any{"fieldType": "string", "value": "alice"}},
			github.encode(t, "github.com", "alice"),
		}},
		map[string]any{"id": "Ag", "title": "GitLab", "credentials": []any{gitlab.encode(t, "gitlab.com", "alice@example.com")}},
	)

	preview, err := PrepareCredentialExchangeImport(service, personalID, content)
	if err != nil {
		t.Fatal(err)
	}
	if preview.FileName != "1Password" || preview.FilePath != "" || preview.FormatName != "Credential Exchange" || preview.ItemCount != 2 || preview.PasskeyCount != 2 ||
		len(preview.Duplicates) != 1 || preview.Duplicates[0].Title != "GitHub" || preview.Duplicates[0].PasskeyCount != 1 {
		t.Fatalf("preview = %+v", preview)
	}
	if _, err := service.StartImport(personalID, true); err != nil {
		t.Fatal(err)
	}
	if progress, _ := runImportToEnd(t, service); progress.SavedCount != 2 || progress.FileName != "1Password" || len(progress.Failed) != 0 {
		t.Fatalf("progress = %+v", progress)
	}
	// Credential Exchange has no local export file; the source app still holds omitted non-UV material.
	service.CancelImport()

	// Skipping the duplicate added its passkey to the login the vault had, which keeps its password.
	itemsByTitle := listItemsByTitle(t, service, personalID)
	if updated := itemsByTitle["GitHub"]; updated.ID != existing.ID || len(updated.Passkeys) != 1 || len(itemsByTitle) != 2 {
		t.Fatalf("items = %+v", itemsByTitle)
	}
	if updated, err := service.GetItem(personalID, existing.ID); err != nil || updated.Password != "kept-password" {
		t.Errorf("the login lost its password: %q %v", updated.Password, err)
	}
	if imported := itemsByTitle["GitLab"]; len(imported.Passkeys) != 1 || imported.Username != "alice@example.com" || imported.Passkeys[0].RelyingPartyID != "gitlab.com" {
		t.Errorf("GitLab = %+v", imported)
	}

	// The imported key signs: the iPhone path, verified with the key 1Password had.
	clientDataHash := sha256.Sum256([]byte("client data"))
	assertion, err := SignPasskeyWithPolicy(service, PasskeyAssertionRequest{RelyingPartyID: "github.com", CredentialID: encodeBase64URL(github.credentialID), ClientDataHash: encodeBase64URL(clientDataHash[:]), UserVerification: "preferred"})
	if err != nil {
		t.Fatal(err)
	}
	authenticatorData, _ := decodeBase64URL(assertion.AuthenticatorData)
	signature, _ := decodeBase64URL(assertion.Signature)
	signedData := sha256.Sum256(append(authenticatorData, clientDataHash[:]...))
	if !ecdsa.VerifyASN1(&github.privateKey.PublicKey, signedData[:], signature) || assertion.UserHandle != encodeBase64URL(github.userHandle) {
		t.Errorf("the assertion does not verify with the exported key: %+v", assertion)
	}

	// The browser path gives the PRF outputs 1Password gave, from the exported hmac-secret credential.
	salt := []byte("vault key salt")
	result, code := testBridge.sendPasskeyRequest(t, "passkeys.get", token, browserprotocol.PasskeyRequest{
		Origin: "https://github.com", RelyingPartyID: "github.com", Challenge: "Y2hhbGxlbmdl", CredentialID: encodeBase64URL(github.credentialID),
		PRF: &browserprotocol.PasskeyPRFRequest{Eval: &browserprotocol.PasskeyPRFValues{First: encodeBase64URL(salt)}},
	})
	prfAssertion, _ := result.(browserprotocol.PasskeyAssertionResult)
	if code != "" || prfAssertion.PRF == nil || prfAssertion.PRF.Results == nil || prfAssertion.PRF.Results.First != encodeBase64URL(passkey.EvaluatePRF(github.prfSecret, salt)) {
		t.Errorf("prf = %+v %s", prfAssertion.PRF, code)
	}

	// Importing the same credentials again adds no passkey twice.
	if _, err := PrepareCredentialExchangeImport(service, personalID, content); err != nil {
		t.Fatal(err)
	}
	if progress, err := service.StartImport(personalID, true); err != nil || progress.TotalCount != 0 {
		t.Errorf("second import = %+v %v", progress, err)
	}
}

// The CXF Android's credential transfer hands over (core/importer's spec fixture) links its login to the scope's app, and
// the imported passkey signs.
func TestCredentialExchangeImportLinksAndroidApps(t *testing.T) {
	fixture := createUnlockedTestService(t)
	service, personalID := fixture.service, fixture.personalID
	content, err := os.ReadFile("../importer/testdata/credential-exchange-android.json")
	if err != nil {
		t.Fatal(err)
	}
	preview, err := PrepareCredentialExchangeImport(service, personalID, content)
	if err != nil {
		t.Fatal(err)
	}
	if preview.FileName != "Example Exporter" || preview.ItemCount != 6 || preview.PasskeyCount != 2 || len(preview.Skipped) != 6 {
		t.Fatalf("preview = %+v", preview)
	}
	if !strings.Contains(preview.Skipped[5].Reason, "SSH key comment") {
		t.Errorf("missing SSH comment warning: %+v", preview.Skipped)
	}
	if _, err := service.StartImport(personalID, true); err != nil {
		t.Fatal(err)
	}
	if progress, _ := runImportToEnd(t, service); progress.SavedCount != 6 || len(progress.Failed) != 0 || progress.LeftBehindCount != 6 {
		t.Fatalf("progress = %+v", progress)
	}

	const certificate = "80:36:59:41:C9:DC:84:B8:D8:46:4C:E0:B7:EF:64:27:B1:DF:DE:34:F6:31:7B:58:FB:92:A9:03:E9:F8:2C:65"
	example := listItemsByTitle(t, service, personalID)["Example"]
	if !slices.Equal(example.URLs, []string{"https://login.example.com"}) ||
		!slices.Equal(example.AndroidApps, []ItemAndroidApp{{PackageName: "com.example.app", CertificateSHA256: certificate}}) {
		t.Errorf("Example: websites %q, apps %+v", example.URLs, example.AndroidApps)
	}
	logins, err := ListLoginsForAndroidApp(service, "com.example.app", certificate)
	if err != nil || !slices.Equal(listLoginTitles(logins), []string{"Example"}) {
		t.Errorf("logins for the app: %+v %v", logins, err)
	}

	clientDataHash := sha256.Sum256([]byte("client data"))
	if _, err := SignPasskeyWithPolicy(service, PasskeyAssertionRequest{RelyingPartyID: "login.example.com", CredentialID: "7eRKGgxZmQPuErE3IN4Lqg", ClientDataHash: encodeBase64URL(clientDataHash[:]), UserVerification: "preferred"}); err != nil {
		t.Errorf("the imported passkey does not sign: %v", err)
	}
}
