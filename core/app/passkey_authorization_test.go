package app

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"errors"
	"testing"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/tryhisar/hisar/core/browserprotocol"
	"github.com/tryhisar/hisar/core/passkey"
)

type passkeyVerifierFunc func(context.Context, string) error

func (verify passkeyVerifierFunc) VerifyPasskey(ctx context.Context, reason string) error {
	return verify(ctx, reason)
}

func TestPasskeyPoliciesReachTheRelyingParty(t *testing.T) {
	for _, policy := range []string{"required", "preferred", "discouraged"} {
		t.Run(policy, func(t *testing.T) {
			bridge := createBrowserTestBridge(t)
			token := bridge.pair(t, "policy-browser")
			verified := 0
			SetPasskeyVerifier(bridge.testService.service, passkeyVerifierFunc(func(context.Context, string) error { verified++; return nil }))
			rp := createPasskeyRelyingParty(t, "example.com", "https://example.com", []byte("policy-user"))
			rp.webAuthn.Config.AuthenticatorSelection.UserVerification = protocol.UserVerificationRequirement(policy)
			creation, err := rp.register(t, bridge, token, browserprotocol.PasskeyRequest{Origin: "https://example.com", UserName: "alex", UserVerification: policy})
			if err != nil {
				t.Fatal(err)
			}
			data, _ := decodeBase64URL(creation.AuthenticatorData)
			if (data[32]&passkey.FlagUserVerified != 0) != (policy != "discouraged") {
				t.Fatalf("creation flags %x for %s", data[32], policy)
			}
			if data[32]&passkey.FlagUserPresent == 0 {
				t.Fatal("confirmed creation has no UP")
			}
			if err := rp.signIn(t, bridge, token, browserprotocol.PasskeyRequest{Origin: "https://example.com", CredentialID: creation.CredentialID, UserVerification: policy}); err != nil {
				t.Fatal(err)
			}
			want := 2
			if policy == "discouraged" {
				want = 0
			}
			if verified != want {
				t.Fatalf("native verification calls = %d, want %d", verified, want)
			}
		})
	}
}

func TestPasskeyVerificationFailureProducesNoCredentialOrPRF(t *testing.T) {
	fixture := createUnlockedTestService(t)
	service := fixture.service
	for _, policy := range []string{"required", "preferred", "discouraged"} {
		SetPasskeyVerifier(service, passkeyVerifierFunc(func(context.Context, string) error { return errors.New("native check cancelled") }))
		_, err := service.createPasskey(context.Background(), &browserprotocol.PasskeyRequest{Origin: "https://example.com", Challenge: "AQ", UserHandle: "AQ", UserName: "alex", UserVerification: policy, PRF: &browserprotocol.PasskeyPRFRequest{Eval: &browserprotocol.PasskeyPRFValues{First: "AQ"}}})
		if err == nil {
			t.Fatalf("%s released credential or PRF after refusal", policy)
		}
	}
	keys, err := ListPasskeys(service)
	if err != nil || len(keys) != 0 {
		t.Fatalf("failed verification saved passkeys: %v %v", keys, err)
	}
	SetPasskeyVerifier(service, nil)
	if _, err := service.createPasskey(context.Background(), &browserprotocol.PasskeyRequest{Origin: "https://example.com", Challenge: "AQ", UserHandle: "AQ", UserVerification: "required"}); err == nil {
		t.Fatal("required succeeded without a native verifier")
	}
}

func TestPasskeyLateNativeEvidenceCannotCrossSessionEnd(t *testing.T) {
	for _, action := range []string{"lock", "account-switch", "cancel", "native-dismiss"} {
		t.Run(action, func(t *testing.T) {
			bridge := createBrowserTestBridge(t)
			service := bridge.testService.service
			token := bridge.pair(t, "stale-browser")
			pause := newPause()
			SetPasskeyVerifier(service, passkeyVerifierFunc(func(context.Context, string) error { pause.wait(); return nil }))
			id := rand.Text()
			request := browserprotocol.PasskeyRequest{Origin: "https://example.com", Challenge: "AQ", UserHandle: "AQ", UserVerification: "required", CeremonyID: id, PRF: &browserprotocol.PasskeyPRFRequest{Eval: &browserprotocol.PasskeyPRFValues{First: "AQ"}}}
			encoded, _ := json.Marshal(request)
			finished := make(chan error, 1)
			go func() {
				_, err := bridge.bridge.handleRequest(context.Background(), &browserprotocol.Request{Type: "passkeys.create", Token: token, Passkey: encoded})
				finished <- err
			}()
			<-pause.reached
			switch action {
			case "lock":
				service.Lock()
			case "account-switch":
				signInOtherAccount(t, service, createUnlockedTestService(t))
			case "cancel":
				if err := bridge.bridge.cancelPasskeyCeremony(token, id); err != nil {
					t.Fatal(err)
				}
			case "native-dismiss":
				CancelPasskeyCeremonies(service)
			}
			close(pause.resume)
			if err := <-finished; err == nil {
				t.Fatal("late verification produced a credential or PRF")
			}
		})
	}
}

func TestPlatformPasskeyPoliciesAndLegacyRefusal(t *testing.T) {
	for _, policy := range []string{"required", "preferred", "discouraged"} {
		t.Run(policy, func(t *testing.T) {
			fixture := createUnlockedTestService(t)
			service := fixture.service
			calls := 0
			SetPasskeyVerifier(service, passkeyVerifierFunc(func(context.Context, string) error { calls++; return nil }))
			hash := sha256.Sum256([]byte("native challenge"))
			created, err := CreatePasskeyForRelyingParty(service, PasskeyRegistration{RelyingPartyID: "example.com", ClientDataHash: encodeBase64URL(hash[:]), UserHandle: "AQ", UserVerification: policy})
			if err != nil {
				t.Fatal(err)
			}
			asserted, err := SignPasskeyWithPolicy(service, PasskeyAssertionRequest{RelyingPartyID: "example.com", CredentialID: created.CredentialID, ClientDataHash: encodeBase64URL(hash[:]), UserVerification: policy})
			if err != nil {
				t.Fatal(err)
			}
			for _, encoded := range []string{created.AuthenticatorData, asserted.AuthenticatorData} {
				data, _ := decodeBase64URL(encoded)
				if (data[32]&passkey.FlagUserVerified != 0) != (policy != "discouraged") || data[32]&passkey.FlagUserPresent == 0 {
					t.Fatalf("native flags %x for %s", data[32], policy)
				}
			}
			stored := getCachedItem(t, service, created.VaultID, created.ItemID).Details.Passkeys[0]
			key, err := x509.ParsePKCS8PrivateKey(stored.PrivateKey)
			if err != nil {
				t.Fatal(err)
			}
			authenticatorData, _ := decodeBase64URL(asserted.AuthenticatorData)
			signature, _ := decodeBase64URL(asserted.Signature)
			signed := sha256.Sum256(append(authenticatorData, hash[:]...))
			if !ecdsa.VerifyASN1(&key.(*ecdsa.PrivateKey).PublicKey, signed[:], signature) {
				t.Fatal("native assertion signature did not authenticate its flags")
			}
			want := 2
			if policy == "discouraged" {
				want = 0
			}
			if calls != want {
				t.Fatalf("native verification calls %d, want %d", calls, want)
			}
		})
	}
}

func TestPasskeyPersonalChoicesAndExplicitSharedSave(t *testing.T) {
	bridge := createBrowserTestBridge(t)
	service := bridge.testService.service
	personal, shared := bridge.testService.personalID, bridge.testService.familyID
	sharedLogin, err := service.SaveItem(shared, ItemDetail{Title: "AAA example", Username: "alex", URLs: []string{"https://example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	personalLogin, err := service.SaveItem(personal, ItemDetail{Title: "ZZZ example", Username: "alex", URLs: []string{"https://example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PreparePasskeyRegistration(service, "example.com", nil)
	if err != nil || len(prepared.Logins) != 2 {
		t.Fatalf("choices: %+v %v", prepared, err)
	}
	if !prepared.Logins[0].IsPersonal || prepared.Logins[0].ItemID != personalLogin.ID || prepared.Logins[1].IsPersonal {
		t.Fatalf("attacker influenced default scope: %+v", prepared.Logins)
	}
	hash := sha256.Sum256([]byte("shared registration"))
	created, err := CreatePasskeyForRelyingParty(service, PasskeyRegistration{RelyingPartyID: "example.com", UserHandle: "AQ", ClientDataHash: encodeBase64URL(hash[:]), UserVerification: "required", VaultID: shared, ItemID: sharedLogin.ID})
	if err != nil || created.VaultID != shared {
		t.Fatalf("explicit shared save failed: %+v %v", created, err)
	}
}

func TestPasskeyLegacyPolicyAndReplayAreRefused(t *testing.T) {
	bridge := createBrowserTestBridge(t)
	token := bridge.pair(t, "legacy-browser")
	request := browserprotocol.PasskeyRequest{Origin: "https://example.com", Challenge: "AQ", UserHandle: "AQ", CeremonyID: rand.Text()}
	encoded, _ := json.Marshal(request)
	_, err := bridge.bridge.handleRequest(context.Background(), &browserprotocol.Request{Type: "passkeys.create", Token: token, Passkey: encoded})
	if readResponseErrorCode(err) != browserprotocol.ErrorUpdateRequired {
		t.Fatalf("legacy ceremony: %v", err)
	}
	request.CeremonyID, request.UserVerification = rand.Text(), "required"
	encoded, _ = json.Marshal(request)
	for attempt := range 2 {
		_, err = bridge.bridge.handleRequest(context.Background(), &browserprotocol.Request{Type: "passkeys.create", Token: token, Passkey: encoded})
		if (err == nil) != (attempt == 0) {
			t.Fatalf("ceremony replay attempt %d: %v", attempt, err)
		}
	}
}
