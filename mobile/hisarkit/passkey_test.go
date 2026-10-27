package hisarkit

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/browserprotocol"
)

type storedPasskey struct {
	CredentialID   string `json:"credentialId"`
	RelyingPartyID string `json:"relyingPartyId"`
	UserName       string `json:"userName"`
	UserHandle     string `json:"userHandle"`
	VaultID        string `json:"vaultID"`
	ItemID         string `json:"itemID"`
	Title          string `json:"title"`
}

func decodeBase64URL(t *testing.T, text string) []byte {
	t.Helper()
	decoded, err := base64.RawURLEncoding.DecodeString(text)
	if err != nil {
		t.Fatalf("%q: %v", text, err)
	}
	return decoded
}

// readCOSEPublicKey reads x and y at the fixed offsets of Hisar's attested credential data: a 32-byte credential ID and a 77-byte COSE key.
func readCOSEPublicKey(t *testing.T, authenticatorData []byte) *ecdsa.PublicKey {
	t.Helper()
	const coseKeyOffset = 37 + 16 + 2 + 32
	if len(authenticatorData) != coseKeyOffset+77 || authenticatorData[coseKeyOffset] != 0xa5 {
		t.Fatalf("authenticator data = %x", authenticatorData)
	}
	coseKey := authenticatorData[coseKeyOffset:]
	return &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(coseKey[10:42]), Y: new(big.Int).SetBytes(coseKey[45:77])}
}

type passkeyVerifier struct{}

func (passkeyVerifier) AuthenticatePasskey(string) error { return nil }

func checkPasskeyFlow(t *testing.T, hisar *Hisar, personalID, itemID string) {
	t.Helper()
	hisar.SetPasskeyVerifier(passkeyVerifier{})
	clientDataHash := sha256.Sum256([]byte(`{"type":"webauthn.create","challenge":"AQID","origin":"https://github.com"}`))
	prepared := decodeJSON[struct {
		IsExcluded bool `json:"isExcluded"`
		AllLogin   []struct {
			ItemID string `json:"itemID"`
		} `json:"logins"`
	}](t)(hisar.PreparePasskeyRegistration("github.com", "[]"))
	if prepared.IsExcluded || len(prepared.AllLogin) != 1 || prepared.AllLogin[0].ItemID != itemID {
		t.Fatalf("prepare = %+v", prepared)
	}
	registration, _ := json.Marshal(map[string]any{
		"userVerification": "required",
		"relyingPartyId":   "github.com", "userName": "octocat", "userHandle": base64.RawURLEncoding.EncodeToString([]byte("octocat-id")),
		"clientDataHash": base64.RawURLEncoding.EncodeToString(clientDataHash[:]), "vaultID": personalID, "itemID": itemID,
	})
	created := decodeJSON[struct {
		CredentialID      string `json:"credentialId"`
		AttestationObject string `json:"attestationObject"`
		AuthenticatorData string `json:"authenticatorData"`
		ItemID            string `json:"itemID"`
	}](t)(hisar.CreatePasskey(string(registration)))
	authenticatorData := decodeBase64URL(t, created.AuthenticatorData)
	relyingPartyIDHash := sha256.Sum256([]byte("github.com"))
	if created.ItemID != itemID || !strings.HasSuffix(string(decodeBase64URL(t, created.AttestationObject)), string(authenticatorData)) || string(authenticatorData[:32]) != string(relyingPartyIDHash[:]) || authenticatorData[32] != 0x5d {
		t.Fatalf("created = %+v", created)
	}
	publicKey := readCOSEPublicKey(t, authenticatorData)

	passkeys := decodeJSON[[]storedPasskey](t)(hisar.ListAllPasskeyForRelyingParty("github.com", ""))
	if len(passkeys) != 1 || passkeys[0].CredentialID != created.CredentialID || passkeys[0].UserHandle != base64.RawURLEncoding.EncodeToString([]byte("octocat-id")) || passkeys[0].Title != "GitHub" {
		t.Fatalf("passkeys for github.com = %+v", passkeys)
	}
	if everyPasskeys := decodeJSON[[]storedPasskey](t)(hisar.ListAllPasskey()); len(everyPasskeys) != 1 || everyPasskeys[0].RelyingPartyID != "github.com" {
		t.Errorf("every passkey = %+v", everyPasskeys)
	}

	assertionHash := sha256.Sum256([]byte(`{"type":"webauthn.get","challenge":"BAUG","origin":"https://github.com"}`))
	assertionJSON, _ := json.Marshal(map[string]string{"relyingPartyId": "github.com", "credentialId": created.CredentialID, "clientDataHash": base64.RawURLEncoding.EncodeToString(assertionHash[:]), "userVerification": "required"})
	assertion := decodeJSON[struct {
		CredentialID      string `json:"credentialId"`
		AuthenticatorData string `json:"authenticatorData"`
		Signature         string `json:"signature"`
		UserHandle        string `json:"userHandle"`
	}](t)(hisar.AssertPasskeyWithPolicy(string(assertionJSON)))
	assertionData := decodeBase64URL(t, assertion.AuthenticatorData)
	signedData := sha256.Sum256(append(assertionData, assertionHash[:]...))
	if !ecdsa.VerifyASN1(publicKey, signedData[:], decodeBase64URL(t, assertion.Signature)) || assertionData[32] != 0x1d || string(decodeBase64URL(t, assertion.UserHandle)) != "octocat-id" {
		t.Fatalf("assertion = %+v", assertion)
	}
	if _, err := hisar.AssertPasskey("github.com", created.CredentialID, base64.RawURLEncoding.EncodeToString(assertionHash[:])); err == nil {
		t.Error("legacy policy-unaware API remained usable")
	}

	itemWithoutPasskey := decodeJSON[map[string]any](t)(hisar.GetItem(personalID, itemID))
	delete(itemWithoutPasskey, "passkeys")
	itemWithoutPasskey["title"] = "GitHub renamed"
	itemJSON, _ := json.Marshal(itemWithoutPasskey)
	edited := decodeJSON[struct {
		Title      string `json:"title"`
		AllPasskey []struct {
			CredentialID string `json:"credentialId"`
		} `json:"passkeys"`
	}](t)(hisar.SaveItem(personalID, string(itemJSON)))
	if edited.Title != "GitHub renamed" || len(edited.AllPasskey) != 1 || edited.AllPasskey[0].CredentialID != created.CredentialID {
		t.Fatalf("edit without passkeys dropped them: %+v", edited)
	}
	destination := decodeJSON[struct {
		ID string `json:"id"`
	}](t)(hisar.CreateVault("Move test"))
	type movedDetail struct {
		ID       string `json:"id"`
		Password string `json:"password"`
		Passkeys []struct {
			CredentialID string `json:"credentialId"`
		} `json:"passkeys"`
	}
	beforeMove := decodeJSON[movedDetail](t)(hisar.GetItem(personalID, itemID))
	moved := decodeJSON[movedDetail](t)(hisar.MoveItem(personalID, itemID, destination.ID))
	if moved.ID != itemID || moved.Password != beforeMove.Password || len(moved.Passkeys) != 1 || moved.Passkeys[0].CredentialID != created.CredentialID {
		t.Fatalf("move changed item identity or contents: %+v", moved)
	}
	if _, err := hisar.GetItem(personalID, itemID); err == nil {
		t.Fatal("moved item remained in its source")
	}
	decodeJSON[movedDetail](t)(hisar.MoveItem(destination.ID, itemID, personalID))
	if _, err := hisar.DeleteVault(destination.ID, `{}`); err != nil {
		t.Fatal(err)
	}
	afterDelete := decodeJSON[struct {
		AllPasskey []struct{} `json:"passkeys"`
	}](t)(hisar.DeletePasskey(personalID, itemID, created.CredentialID))
	if len(afterDelete.AllPasskey) != 0 {
		t.Errorf("after delete = %+v", afterDelete)
	}
}

func TestPasskeyArgumentsAndErrors(t *testing.T) {
	hisar, err := New(t.TempDir(), "iPhone", createMemorySecretKeyStore(), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hisar.ListAllPasskeyForRelyingParty("example.com", "not json"); err == nil || !strings.Contains(err.Error(), "JSON array") {
		t.Errorf("broken allowed credential IDs: %v", err)
	}
	if _, err := hisar.PreparePasskeyRegistration("example.com", `{"a":1}`); err == nil {
		t.Error("an object accepted as excluded credential IDs")
	}
	if _, err := hisar.CreatePasskey("{"); err == nil || !strings.Contains(err.Error(), "not valid JSON") {
		t.Errorf("broken registration: %v", err)
	}
	if _, err := hisar.ListAllPasskey(); err == nil || err.Error() != "sign in to Hisar first" {
		t.Errorf("signed out: %v", err)
	}
	if _, err := hisar.AssertPasskey("co.uk", "AQ", "AQ"); err == nil || strings.Contains(err.Error(), "invalid-request") {
		t.Errorf("error code kept in the message: %v", err)
	}
	if values, err := decodeStringArray(" "); err != nil || values != nil {
		t.Errorf("empty array text = %v %v", values, err)
	}
	if _, err := encodePasskeyJSON(nil, browserprotocol.NewError(browserprotocol.ErrorNotFound, "passkey: passkey not found")); err == nil || err.Error() != "passkey not found" {
		t.Errorf("converted error = %v", err)
	}
	if _, err := encodePasskeyJSON(nil, errors.New("plain")); err == nil || err.Error() != "plain" {
		t.Errorf("plain error = %v", err)
	}
}

func TestPasskeyClientDataForAnAndroidApp(t *testing.T) {
	signingCertificateSHA256 := sha256.Sum256([]byte("an app's signing certificate"))
	origin, err := BuildAndroidAppOrigin(signingCertificateSHA256[:])
	if err != nil {
		t.Fatal(err)
	}
	if origin != "android:apk-key-hash:"+base64.RawURLEncoding.EncodeToString(signingCertificateSHA256[:]) {
		t.Errorf("origin = %s", origin)
	}
	built := decodeJSON[struct {
		ClientDataJSON string `json:"clientDataJSON"`
		ClientDataHash string `json:"clientDataHash"`
	}](t)(BuildPasskeyClientData("webauthn.get", "-_8B", origin, "com.example.app"))
	clientDataJSON := decodeBase64URL(t, built.ClientDataJSON)
	if hash := sha256.Sum256(clientDataJSON); built.ClientDataHash != base64.RawURLEncoding.EncodeToString(hash[:]) {
		t.Errorf("hash %s is not the SHA-256 of %s", built.ClientDataHash, clientDataJSON)
	}
	var clientData map[string]any
	if err := json.Unmarshal(clientDataJSON, &clientData); err != nil {
		t.Fatal(err)
	}
	if clientData["type"] != "webauthn.get" || clientData["challenge"] != "-_8B" || clientData["origin"] != origin || clientData["androidPackageName"] != "com.example.app" {
		t.Errorf("client data = %s", clientDataJSON)
	}

	if _, err := BuildPasskeyClientData("payment.get", "-_8B", origin, "com.example.app"); err == nil {
		t.Error("a payment ceremony was accepted")
	}
	if _, err := BuildPasskeyClientData("webauthn.create", "not base64url!", origin, "com.example.app"); err == nil {
		t.Error("a challenge that is not base64url was accepted")
	}
	if _, err := BuildPasskeyClientData("webauthn.create", "-_8B", origin, ""); err == nil {
		t.Error("an Android app without its package name was accepted")
	}
	if _, err := BuildAndroidAppOrigin(signingCertificateSHA256[:20]); err == nil {
		t.Error("a 20-byte digest was accepted")
	}
}

func TestPasskeyRelyingPartyOfABrowserOrigin(t *testing.T) {
	for _, test := range []struct{ origin, relyingPartyID, want string }{
		{"https://webauthn.io/", "webauthn.io", "webauthn.io"},
		{"https://login.example.com", "example.com", "example.com"},
		{"https://example.com/", "", "example.com"},
	} {
		if got, err := ResolvePasskeyRelyingPartyID(test.origin, test.relyingPartyID); err != nil || got != test.want {
			t.Errorf("ResolvePasskeyRelyingPartyID(%q, %q) = %q, %v", test.origin, test.relyingPartyID, got, err)
		}
	}
	for _, test := range []struct{ origin, relyingPartyID string }{
		{"https://evil.example/", "example.com"},
		{"https://example.co.uk/", "co.uk"},
		{"http://example.com/", "example.com"},
		{"android:apk-key-hash:o-gkSHjX7zxYu_zWPMvrWP6BUmpphy57-aF_UA8n0Sw", "example.com"},
	} {
		if got, err := ResolvePasskeyRelyingPartyID(test.origin, test.relyingPartyID); err == nil {
			t.Errorf("ResolvePasskeyRelyingPartyID(%q, %q) = %q, want an error", test.origin, test.relyingPartyID, got)
		}
	}
}
