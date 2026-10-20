package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/tryhisar/hisar/core/browserprotocol"
)

func hashPlatformClientData(t *testing.T, ceremonyType protocol.CeremonyType, challenge protocol.URLEncodedBase64, origin string) ([]byte, string) {
	t.Helper()
	clientDataJSON, err := json.Marshal(protocol.CollectedClientData{Type: ceremonyType, Challenge: challenge.String(), Origin: origin})
	if err != nil {
		t.Fatal(err)
	}
	clientDataHash := sha256.Sum256(clientDataJSON)
	return clientDataJSON, encodeBase64URL(clientDataHash[:])
}

func (relyingParty *passkeyRelyingParty) registerOnPlatform(t *testing.T, service *Service, origin string, registration PasskeyRegistration) (PasskeyRegistrationResult, error) {
	t.Helper()
	if registration.UserVerification == "" {
		registration.UserVerification = "preferred"
	}
	options, session, err := relyingParty.webAuthn.BeginRegistration(relyingParty.user)
	if err != nil {
		t.Fatal(err)
	}
	clientDataJSON, clientDataHash := hashPlatformClientData(t, protocol.CreateCeremony, options.Response.Challenge, origin)
	registration.ClientDataHash = clientDataHash
	registration.UserHandle = encodeBase64URL(relyingParty.user.userHandle)
	result, err := CreatePasskeyForRelyingParty(service, registration)
	if err != nil {
		return result, err
	}
	body, _ := json.Marshal(map[string]any{
		"id": result.CredentialID, "rawId": result.CredentialID, "type": "public-key", "clientExtensionResults": map[string]any{},
		"response": map[string]any{"clientDataJSON": encodeBase64URL(clientDataJSON), "attestationObject": result.AttestationObject},
	})
	parsed, err := protocol.ParseCredentialCreationResponseBody(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	credential, err := relyingParty.webAuthn.CreateCredential(relyingParty.user, *session, parsed)
	if err != nil {
		t.Fatalf("relying party rejected the registration: %v", err)
	}
	if credential.AttestationFormat != "none" || !credential.Flags.UserVerified || !credential.Flags.BackupState || encodeBase64URL(credential.ID) != result.CredentialID {
		t.Errorf("credential = %+v", credential)
	}
	relyingParty.user.credentials = append(relyingParty.user.credentials, *credential)
	return result, nil
}

func (relyingParty *passkeyRelyingParty) signInOnPlatform(t *testing.T, service *Service, origin, relyingPartyID, credentialID string) error {
	t.Helper()
	options, session, err := relyingParty.webAuthn.BeginDiscoverableLogin()
	if err != nil {
		t.Fatal(err)
	}
	clientDataJSON, clientDataHash := hashPlatformClientData(t, protocol.AssertCeremony, options.Response.Challenge, origin)
	assertion, err := SignPasskeyWithPolicy(service, PasskeyAssertionRequest{RelyingPartyID: relyingPartyID, CredentialID: credentialID, ClientDataHash: clientDataHash, UserVerification: "preferred"})
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{
		"id": assertion.CredentialID, "rawId": assertion.CredentialID, "type": "public-key", "clientExtensionResults": map[string]any{},
		"response": map[string]any{
			"clientDataJSON": encodeBase64URL(clientDataJSON), "authenticatorData": assertion.AuthenticatorData,
			"signature": assertion.Signature, "userHandle": assertion.UserHandle,
		},
	})
	parsed, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	_, err = relyingParty.webAuthn.ValidateDiscoverableLogin(func(_, userHandle []byte) (webauthn.User, error) {
		if !bytes.Equal(userHandle, relyingParty.user.userHandle) {
			return nil, errors.New("unknown user handle")
		}
		return relyingParty.user, nil
	}, *session, parsed)
	return err
}

func readResponseErrorCode(err error) string {
	var responseError *browserprotocol.ResponseError
	if errors.As(err, &responseError) {
		return responseError.Code
	}
	return ""
}

func TestPlatformPasskeyCreateAndSignInFromClientDataHash(t *testing.T) {
	testService := createUnlockedTestService(t)
	service := testService.service
	personalID := testService.personalID
	existing, err := service.SaveItem(personalID, ItemDetail{Title: "Example", Username: "alex@example.com", Password: "pw", URLs: []string{"https://login.example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveItem(testService.familyID, ItemDetail{Title: "Other site", Username: "x", URLs: []string{"https://other.example.net"}}); err != nil {
		t.Fatal(err)
	}

	prepared, err := PreparePasskeyRegistration(service, "Example.com", nil)
	if err != nil || prepared.IsExcluded || len(prepared.Logins) != 1 || prepared.Logins[0].ItemID != existing.ID {
		t.Fatalf("prepare = %+v %v", prepared, err)
	}
	relyingParty := createPasskeyRelyingParty(t, "example.com", "https://login.example.com", []byte("platform-user"))
	created, err := relyingParty.registerOnPlatform(t, service, "https://login.example.com", PasskeyRegistration{
		RelyingPartyID: "example.com", UserName: "alex@example.com", UserDisplayName: "Alex", VaultID: personalID, ItemID: existing.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.VaultID != personalID || created.ItemID != existing.ID {
		t.Errorf("saved to %s/%s", created.VaultID, created.ItemID)
	}

	storedPasskeys, err := ListPasskeysForRelyingParty(service, "example.com", nil)
	expected := []StoredPasskey{{
		CredentialID: created.CredentialID, RelyingPartyID: "example.com", UserName: "alex@example.com", UserDisplayName: "Alex",
		UserHandle: encodeBase64URL([]byte("platform-user")), VaultID: personalID, ItemID: existing.ID, Title: "Example",
	}}
	if err != nil || !reflect.DeepEqual(storedPasskeys, expected) {
		t.Fatalf("passkeys for example.com = %+v %v", storedPasskeys, err)
	}
	if everyPasskeys, err := ListPasskeys(service); err != nil || !reflect.DeepEqual(everyPasskeys, expected) {
		t.Errorf("every passkey = %+v %v", everyPasskeys, err)
	}
	if allowedPasskeys, err := ListPasskeysForRelyingParty(service, "example.com", []string{"AQ"}); err != nil || len(allowedPasskeys) != 0 {
		t.Errorf("passkeys allowed by another credential ID = %+v %v", allowedPasskeys, err)
	}
	if subdomainPasskeys, err := ListPasskeysForRelyingParty(service, "login.example.com", nil); err != nil || len(subdomainPasskeys) != 0 {
		t.Errorf("passkeys for login.example.com = %+v %v", subdomainPasskeys, err)
	}

	if err := relyingParty.signInOnPlatform(t, service, "https://login.example.com", "example.com", created.CredentialID); err != nil {
		t.Fatalf("sign in: %v", err)
	}
	if err := relyingParty.signInOnPlatform(t, service, "https://login.example.com", "login.example.com", created.CredentialID); readResponseErrorCode(err) != browserprotocol.ErrorNotFound {
		t.Errorf("signing for another relying party ID: %v", err)
	}

	if prepared, err := PreparePasskeyRegistration(service, "example.com", []string{created.CredentialID}); err != nil || !prepared.IsExcluded {
		t.Errorf("prepare with the passkey excluded = %+v %v", prepared, err)
	}
	if _, err := relyingParty.registerOnPlatform(t, service, "https://login.example.com", PasskeyRegistration{
		RelyingPartyID: "example.com", UserName: "alex@example.com", ExcludedCredentialIDs: []string{created.CredentialID},
	}); readResponseErrorCode(err) != browserprotocol.ErrorInvalidRequest {
		t.Errorf("creating with an excluded credential: %v", err)
	}

	replacement, err := relyingParty.registerOnPlatform(t, service, "https://login.example.com", PasskeyRegistration{
		RelyingPartyID: "example.com", UserName: "alex", VaultID: personalID, ItemID: existing.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if detail, err := service.GetItem(personalID, existing.ID); err != nil || len(detail.Passkeys) != 1 || detail.Passkeys[0].CredentialID != replacement.CredentialID || detail.Password != "pw" {
		t.Fatalf("after replacing the passkey: %+v %v", detail, err)
	}
	if err := relyingParty.signInOnPlatform(t, service, "https://login.example.com", "example.com", created.CredentialID); readResponseErrorCode(err) != browserprotocol.ErrorNotFound {
		t.Errorf("signing with the replaced passkey: %v", err)
	}
}

func TestPlatformPasskeyCreatesNewLoginAndSurvivesEdits(t *testing.T) {
	testService := createUnlockedTestService(t)
	service := testService.service
	relyingParty := createPasskeyRelyingParty(t, "www.shop.example", "https://www.shop.example", []byte{9, 9})
	created, err := relyingParty.registerOnPlatform(t, service, "https://www.shop.example", PasskeyRegistration{RelyingPartyID: "www.shop.example", UserName: "buyer"})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := service.GetItem(created.VaultID, created.ItemID)
	if err != nil || created.VaultID != testService.personalID || detail.Title != "shop.example" || detail.Username != "buyer" || !reflect.DeepEqual(detail.URLs, []string{"https://www.shop.example"}) || len(detail.Passkeys) != 1 {
		t.Fatalf("new login = %+v %v", detail, err)
	}

	encoded, _ := json.Marshal(detail)
	var edited ItemDetail
	if err := json.Unmarshal(encoded, &edited); err != nil {
		t.Fatal(err)
	}
	edited.Title = "Shop"
	edited.Passkeys = []ItemPasskey{}
	if saved, err := service.SaveItem(created.VaultID, edited); err != nil || saved.Title != "Shop" || len(saved.Passkeys) != 1 {
		t.Fatalf("edit = %+v %v", saved, err)
	}
	if err := relyingParty.signInOnPlatform(t, service, "https://www.shop.example", "www.shop.example", created.CredentialID); err != nil {
		t.Fatalf("sign in after the edit: %v", err)
	}
	if _, err := service.DeletePasskey(created.VaultID, created.ItemID, created.CredentialID); err != nil {
		t.Fatal(err)
	}
	if storedPasskeys, err := ListPasskeys(service); err != nil || len(storedPasskeys) != 0 {
		t.Errorf("after delete = %+v %v", storedPasskeys, err)
	}
}

func TestPlatformPasskeyRefusals(t *testing.T) {
	testService := createUnlockedTestService(t)
	service := testService.service
	validHash := encodeBase64URL(make([]byte, 32))
	other, err := service.SaveItem(testService.personalID, ItemDetail{Title: "Other", URLs: []string{"https://other.example.net"}})
	if err != nil {
		t.Fatal(err)
	}
	for name, registration := range map[string]PasskeyRegistration{
		"public suffix":       {RelyingPartyID: "co.uk", UserHandle: "AQ", ClientDataHash: validHash},
		"IP address":          {RelyingPartyID: "192.168.1.10", UserHandle: "AQ", ClientDataHash: validHash},
		"empty user handle":   {RelyingPartyID: "example.com", ClientDataHash: validHash},
		"short hash":          {RelyingPartyID: "example.com", UserHandle: "AQ", ClientDataHash: "AQ"},
		"missing hash":        {RelyingPartyID: "example.com", UserHandle: "AQ"},
		"bad excluded ID":     {RelyingPartyID: "example.com", UserHandle: "AQ", ClientDataHash: validHash, ExcludedCredentialIDs: []string{"*"}},
		"65-byte user handle": {RelyingPartyID: "example.com", UserHandle: encodeBase64URL(make([]byte, 65)), ClientDataHash: validHash},
	} {
		if _, err := CreatePasskeyForRelyingParty(service, registration); readResponseErrorCode(err) != browserprotocol.ErrorInvalidRequest {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := CreatePasskeyForRelyingParty(service, PasskeyRegistration{
		UserVerification: "preferred",
		RelyingPartyID:   "example.com", UserHandle: "AQ", ClientDataHash: validHash, VaultID: testService.personalID, ItemID: other.ID,
	}); readResponseErrorCode(err) != browserprotocol.ErrorNoMatch {
		t.Errorf("saving to a login for another site: %v", err)
	}
	if _, err := SignPasskeyWithPolicy(service, PasskeyAssertionRequest{RelyingPartyID: "example.com", CredentialID: "AQ", ClientDataHash: "AQ", UserVerification: "preferred"}); readResponseErrorCode(err) != browserprotocol.ErrorInvalidRequest {
		t.Errorf("signing a short hash: %v", err)
	}
	if _, err := SignPasskeyWithPolicy(service, PasskeyAssertionRequest{RelyingPartyID: "example.com", CredentialID: "AQ", ClientDataHash: validHash, UserVerification: "preferred"}); readResponseErrorCode(err) != browserprotocol.ErrorNotFound {
		t.Errorf("signing with an unknown passkey: %v", err)
	}
	if _, err := ListPasskeysForRelyingParty(service, "com", nil); readResponseErrorCode(err) != browserprotocol.ErrorInvalidRequest {
		t.Errorf("listing for a public suffix: %v", err)
	}
	service.Lock()
	if _, err := ListPasskeys(service); readResponseErrorCode(err) != browserprotocol.ErrorLocked {
		t.Errorf("listing while locked: %v", err)
	}
	if _, err := SignPasskeyWithPolicy(service, PasskeyAssertionRequest{RelyingPartyID: "example.com", CredentialID: "AQ", ClientDataHash: validHash, UserVerification: "preferred"}); readResponseErrorCode(err) != browserprotocol.ErrorLocked {
		t.Errorf("signing while locked: %v", err)
	}
	if _, err := PreparePasskeyRegistration(service, "example.com", nil); readResponseErrorCode(err) != browserprotocol.ErrorLocked {
		t.Errorf("preparing while locked: %v", err)
	}
}
