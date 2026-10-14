package passkey

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/protocol/webauthncose"
	"github.com/go-webauthn/webauthn/webauthn"
)

func TestResolveRelyingPartyID(t *testing.T) {
	cases := []struct {
		origin          string
		relyingPartyID  string
		expectedID      string
		isErrorExpected bool
	}{
		{"https://example.com", "", "example.com", false},
		{"https://example.com", "example.com", "example.com", false},
		{"https://login.example.com", "example.com", "example.com", false},
		{"https://a.b.example.co.uk", "example.co.uk", "example.co.uk", false},
		{"https://a.b.example.co.uk", "b.example.co.uk", "b.example.co.uk", false},
		{"https://Login.Example.com:8443", "EXAMPLE.com", "example.com", false},
		{"https://example.com", "login.example.com", "", true},
		{"https://example.com", "other.com", "", true},
		{"https://badexample.com", "example.com", "", true},
		{"https://example.com", "com", "", true},
		{"https://example.co.uk", "co.uk", "", true},
		{"https://alice.github.io", "github.io", "", true},
		{"https://alice.github.io", "alice.github.io", "alice.github.io", false},
		{"https://example.com", ".example.com", "", true},
		{"https://example.com", "example.com.", "", true},
		{"https://192.168.1.10", "", "192.168.1.10", false},
		{"https://192.168.1.10", "168.1.10", "", true},
		{"http://localhost:5173", "", "localhost", false},
		{"http://localhost:5173", "localhost", "localhost", false},
		{"http://app.localhost", "app.localhost", "app.localhost", false},
		{"http://example.com", "", "", true},
		{"http://127.0.0.1:8080", "", "", true},
		{"ftp://example.com", "", "", true},
		{"chrome-extension://abc", "", "", true},
		{"https://example.com/path", "", "", true},
		{"https://user@example.com", "", "", true},
		{"null", "", "", true},
		{"", "", "", true},
	}
	for _, testCase := range cases {
		relyingPartyID, err := ResolveRelyingPartyID(testCase.origin, testCase.relyingPartyID)
		if testCase.isErrorExpected {
			if err == nil {
				t.Errorf("%s with %q accepted as %q", testCase.origin, testCase.relyingPartyID, relyingPartyID)
			}
			continue
		}
		if err != nil || relyingPartyID != testCase.expectedID {
			t.Errorf("%s with %q = %q, %v; want %q", testCase.origin, testCase.relyingPartyID, relyingPartyID, err, testCase.expectedID)
		}
	}
}

func TestBuildClientDataJSON(t *testing.T) {
	clientData, err := BuildClientDataJSON(ClientData{Type: TypeGet, Challenge: []byte{0xfb, 0xff, 0x01}, Origin: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if expected := `{"type":"webauthn.get","challenge":"-_8B","origin":"https://example.com","crossOrigin":false}`; string(clientData) != expected {
		t.Errorf("client data = %s", clientData)
	}
	clientData, err = BuildClientDataJSON(ClientData{Type: TypeCreate, Challenge: []byte{1}, Origin: "https://pay.example.org", TopOrigin: "https://shop.example.com", IsCrossOrigin: true})
	if err != nil {
		t.Fatal(err)
	}
	if expected := `{"type":"webauthn.create","challenge":"AQ","origin":"https://pay.example.org","crossOrigin":true,"topOrigin":"https://shop.example.com"}`; string(clientData) != expected {
		t.Errorf("cross-origin client data = %s", clientData)
	}
	clientData, _ = BuildClientDataJSON(ClientData{Type: TypeGet, Challenge: []byte{1}, Origin: "https://a.example", TopOrigin: "https://ignored.example"})
	if strings.Contains(string(clientData), "topOrigin") {
		t.Errorf("top origin without crossOrigin: %s", clientData)
	}
	for _, invalid := range []ClientData{
		{Type: "payment.get", Challenge: []byte{1}, Origin: "https://a.example"},
		{Type: TypeGet, Origin: "https://a.example"},
		{Type: TypeGet, Challenge: make([]byte, 1025), Origin: "https://a.example"},
	} {
		if _, err := BuildClientDataJSON(invalid); err == nil {
			t.Errorf("%+v accepted", invalid)
		}
	}
}

func createTestKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	x, _ := new(big.Int).SetString("60fed4ba255a9d31c961eb74c6356d68c049b8923b61fa6ce669622e60f29fb6", 16)
	y, _ := new(big.Int).SetString("7903fe1008b8bc99a41ae9e95628bc64f2f1b20c2d7e9f5177a3c294d4462299", 16)
	return &ecdsa.PrivateKey{PublicKey: ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}}
}

func TestEncodeCOSEPublicKey(t *testing.T) {
	coseKey, err := EncodeCOSEPublicKey(&createTestKey(t).PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	expected := "a5010203262001215820" + "60fed4ba255a9d31c961eb74c6356d68c049b8923b61fa6ce669622e60f29fb6" +
		"225820" + "7903fe1008b8bc99a41ae9e95628bc64f2f1b20c2d7e9f5177a3c294d4462299"
	if hex.EncodeToString(coseKey) != expected {
		t.Errorf("COSE key = %x", coseKey)
	}
	parsedKey, err := webauthncose.ParsePublicKey(coseKey)
	if err != nil {
		t.Fatal(err)
	}
	ec2Key, isEC2 := parsedKey.(webauthncose.EC2PublicKeyData)
	if !isEC2 || ec2Key.Algorithm != AlgorithmES256 || ec2Key.Curve != 1 {
		t.Fatalf("parsed COSE key = %+v", parsedKey)
	}
	if publicKey, err := ec2Key.ToECDSA(); err != nil || !publicKey.Equal(&createTestKey(t).PublicKey) {
		t.Errorf("COSE key decodes to %v, %v", publicKey, err)
	}
	otherCurveKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := EncodeCOSEPublicKey(&otherCurveKey.PublicKey); err == nil {
		t.Error("P-384 key encoded")
	}
}

func TestBuildAttestationObject(t *testing.T) {
	cases := map[int]string{
		5:   "45",
		37:  "5825",
		300: "59012c",
	}
	for length, lengthHeader := range cases {
		authenticatorData := bytes.Repeat([]byte{0xaa}, length)
		encoded := BuildAttestationObject(authenticatorData)
		prefix := "a3" + "63666d74" + "646e6f6e65" + "6761747453746d74" + "a0" + "68617574684461746" + "1" + lengthHeader
		if !strings.HasPrefix(hex.EncodeToString(encoded), prefix) || !bytes.HasSuffix(encoded, authenticatorData) || len(encoded) != len(prefix)/2+length {
			t.Errorf("attestation object for %d bytes = %x", length, encoded)
		}
	}
}

func TestBuildAuthenticatorData(t *testing.T) {
	credentialID := bytes.Repeat([]byte{0x11}, 300)
	attestedCredentialData, err := BuildAttestedCredentialData(credentialID, &createTestKey(t).PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	authenticatorData := BuildAuthenticatorData("example.com", flagCreation, attestedCredentialData)
	relyingPartyIDHash := sha256.Sum256([]byte("example.com"))
	if !bytes.Equal(authenticatorData[:32], relyingPartyIDHash[:]) {
		t.Errorf("rpIdHash = %x", authenticatorData[:32])
	}
	if authenticatorData[32] != 0x5d {
		t.Errorf("flags = %#x, want UP|UV|BE|BS|AT", authenticatorData[32])
	}
	if binary.BigEndian.Uint32(authenticatorData[33:37]) != 0 {
		t.Errorf("sign count = %x", authenticatorData[33:37])
	}
	if hex.EncodeToString(authenticatorData[37:53]) != strings.ReplaceAll(AAGUID, "-", "") {
		t.Errorf("AAGUID = %x", authenticatorData[37:53])
	}
	if binary.BigEndian.Uint16(authenticatorData[53:55]) != 300 || !bytes.Equal(authenticatorData[55:355], credentialID) {
		t.Errorf("credential ID length = %x", authenticatorData[53:55])
	}
	if authenticatorData[355] != 0xa5 || len(authenticatorData) != 355+77 {
		t.Errorf("COSE key at %d: %x", len(authenticatorData), authenticatorData[355:])
	}
	if assertionData := BuildAuthenticatorData("example.com", flagAssertion, nil); len(assertionData) != 37 || assertionData[32] != 0x1d {
		t.Errorf("assertion authenticator data = %x", assertionData)
	}
	if _, err := BuildAttestedCredentialData(nil, &createTestKey(t).PublicKey); err == nil {
		t.Error("empty credential ID accepted")
	}
	if _, err := BuildAttestedCredentialData(make([]byte, 1024), &createTestKey(t).PublicKey); err == nil {
		t.Error("1024-byte credential ID accepted")
	}
}

type testUser struct {
	userHandle      []byte
	credentials     []webauthn.Credential
	userName        string
	userDisplayName string
}

func (user *testUser) WebAuthnID() []byte                         { return user.userHandle }
func (user *testUser) WebAuthnName() string                       { return user.userName }
func (user *testUser) WebAuthnDisplayName() string                { return user.userDisplayName }
func (user *testUser) WebAuthnCredentials() []webauthn.Credential { return user.credentials }

func encodeBase64URL(value []byte) string {
	return base64.RawURLEncoding.EncodeToString(value)
}

func createTestRelyingParty(t *testing.T, relyingPartyID string, origins ...string) *webauthn.WebAuthn {
	t.Helper()
	relyingParty, err := webauthn.New(&webauthn.Config{RPID: relyingPartyID, RPDisplayName: "Example", RPOrigins: origins})
	if err != nil {
		t.Fatal(err)
	}
	return relyingParty
}

func registerWithRelyingParty(t *testing.T, relyingParty *webauthn.WebAuthn, user *testUser, request CreationRequest) (Creation, *webauthn.Credential, error) {
	t.Helper()
	options, session, err := relyingParty.BeginRegistration(user)
	if err != nil {
		t.Fatal(err)
	}
	request.Challenge = options.Response.Challenge
	creation, err := CreateCredential(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{
		"id":                      encodeBase64URL(creation.CredentialID),
		"rawId":                   encodeBase64URL(creation.CredentialID),
		"type":                    "public-key",
		"authenticatorAttachment": "platform",
		"clientExtensionResults":  map[string]any{},
		"response": map[string]any{
			"clientDataJSON":    encodeBase64URL(creation.ClientDataJSON),
			"attestationObject": encodeBase64URL(creation.AttestationObject),
			"transports":        []string{"hybrid", "internal"},
		},
	})
	parsed, err := protocol.ParseCredentialCreationResponseBody(bytes.NewReader(body))
	if err != nil {
		return creation, nil, err
	}
	credential, err := relyingParty.CreateCredential(user, *session, parsed)
	return creation, credential, err
}

func signInWithRelyingParty(t *testing.T, relyingParty *webauthn.WebAuthn, user *testUser, request AssertionRequest, credentialRelyingPartyID string, credentialID, privateKey []byte) error {
	t.Helper()
	options, session, err := relyingParty.BeginDiscoverableLogin()
	if err != nil {
		t.Fatal(err)
	}
	request.Challenge = options.Response.Challenge
	assertion, err := SignAssertion(request, credentialRelyingPartyID, privateKey)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{
		"id":                     encodeBase64URL(credentialID),
		"rawId":                  encodeBase64URL(credentialID),
		"type":                   "public-key",
		"clientExtensionResults": map[string]any{},
		"response": map[string]any{
			"clientDataJSON":    encodeBase64URL(assertion.ClientDataJSON),
			"authenticatorData": encodeBase64URL(assertion.AuthenticatorData),
			"signature":         encodeBase64URL(assertion.Signature),
			"userHandle":        encodeBase64URL(user.userHandle),
		},
	})
	parsed, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(body))
	if err != nil {
		return err
	}
	_, err = relyingParty.ValidateDiscoverableLogin(func(rawID, userHandle []byte) (webauthn.User, error) {
		if !bytes.Equal(userHandle, user.userHandle) {
			return nil, errors.New("unknown user")
		}
		return user, nil
	}, *session, parsed)
	return err
}

func TestCreateAndSignVerifiedByWebAuthnLibrary(t *testing.T) {
	relyingParty := createTestRelyingParty(t, "example.com", "https://login.example.com")
	user := &testUser{userHandle: []byte("user-handle-0001"), userName: "alex@example.com", userDisplayName: "Alex"}
	creation, credential, err := registerWithRelyingParty(t, relyingParty, user, CreationRequest{Origin: "https://login.example.com", RelyingPartyID: "example.com"})
	if err != nil {
		t.Fatalf("registration rejected: %v", err)
	}
	if !bytes.Equal(credential.ID, creation.CredentialID) || len(creation.CredentialID) != CredentialIDLength {
		t.Errorf("credential ID = %x", credential.ID)
	}
	if credential.AttestationFormat != "none" || hex.EncodeToString(credential.Authenticator.AAGUID) != strings.ReplaceAll(AAGUID, "-", "") {
		t.Errorf("attestation %q, AAGUID %x", credential.AttestationFormat, credential.Authenticator.AAGUID)
	}
	if !credential.Flags.UserPresent || !credential.Flags.UserVerified || !credential.Flags.BackupEligible || !credential.Flags.BackupState {
		t.Errorf("flags = %+v", credential.Flags)
	}
	parsedPublicKey, err := x509.ParsePKIXPublicKey(creation.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	privateKey, err := x509.ParsePKCS8PrivateKey(creation.PrivateKey)
	if err != nil || !privateKey.(*ecdsa.PrivateKey).PublicKey.Equal(parsedPublicKey) {
		t.Fatalf("SPKI public key does not belong to the stored key: %v", err)
	}

	user.credentials = []webauthn.Credential{*credential}
	assertionRequest := AssertionRequest{Origin: "https://login.example.com", RelyingPartyID: "example.com"}
	if err := signInWithRelyingParty(t, relyingParty, user, assertionRequest, "example.com", creation.CredentialID, creation.PrivateKey); err != nil {
		t.Fatalf("sign in rejected: %v", err)
	}
	otherSite := createTestRelyingParty(t, "example.com", "https://example.com")
	if err := signInWithRelyingParty(t, otherSite, user, assertionRequest, "example.com", creation.CredentialID, creation.PrivateKey); err == nil {
		t.Error("assertion for login.example.com accepted by a relying party expecting example.com")
	}
	if err := signInWithRelyingParty(t, relyingParty, user, AssertionRequest{Origin: "https://evil.example.net", RelyingPartyID: "example.com"}, "example.com", creation.CredentialID, creation.PrivateKey); !errors.Is(err, ErrInvalidRelyingPartyID) {
		t.Errorf("signing for a foreign origin: %v", err)
	}
	if err := signInWithRelyingParty(t, relyingParty, user, AssertionRequest{Origin: "https://login.example.com", RelyingPartyID: "login.example.com"}, "example.com", creation.CredentialID, creation.PrivateKey); !errors.Is(err, ErrInvalidRelyingPartyID) {
		t.Errorf("signing with a credential of another relying party ID: %v", err)
	}
}

func TestCrossOriginCreationVerifiedByWebAuthnLibrary(t *testing.T) {
	relyingParty, err := webauthn.New(&webauthn.Config{
		RPID: "pay.example.org", RPDisplayName: "Pay", RPOrigins: []string{"https://pay.example.org"},
		RPTopOrigins: []string{"https://shop.example.com"}, RPAllowCrossOrigin: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	user := &testUser{userHandle: []byte{1, 2, 3, 4}, userName: "alex"}
	request := CreationRequest{Origin: "https://pay.example.org", TopOrigin: "https://shop.example.com", IsCrossOrigin: true}
	if _, _, err := registerWithRelyingParty(t, relyingParty, user, request); err != nil {
		t.Fatalf("cross-origin registration rejected: %v", err)
	}
}

func TestCreateCredentialRejectsForeignRelyingParty(t *testing.T) {
	if _, err := CreateCredential(CreationRequest{Origin: "https://evil.example.net", RelyingPartyID: "example.com", Challenge: []byte{1}}); !errors.Is(err, ErrInvalidRelyingPartyID) {
		t.Errorf("foreign relying party: %v", err)
	}
	if _, err := CreateCredential(CreationRequest{Origin: "http://example.com", Challenge: []byte{1}}); !errors.Is(err, ErrInvalidOrigin) {
		t.Errorf("plain http origin: %v", err)
	}
	relyingParty := createTestRelyingParty(t, "example.com", "https://example.com")
	user := &testUser{userHandle: []byte{1}, userName: "alex"}
	if _, _, err := registerWithRelyingParty(t, relyingParty, user, CreationRequest{Origin: "https://sub.example.com"}); err == nil {
		t.Error("registration from sub.example.com with its own RP ID accepted by example.com")
	}
}

func TestSignAssertionRejectsBadKey(t *testing.T) {
	request := AssertionRequest{Origin: "https://example.com", Challenge: []byte{1}}
	if _, err := SignAssertion(request, "example.com", []byte("not a key")); err == nil {
		t.Error("garbage key accepted")
	}
	otherCurveKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := x509.MarshalPKCS8PrivateKey(otherCurveKey)
	if _, err := SignAssertion(request, "example.com", encoded); err == nil {
		t.Error("P-384 key accepted")
	}
}

func TestValidateRelyingPartyID(t *testing.T) {
	cases := []struct {
		relyingPartyID  string
		expectedID      string
		isErrorExpected bool
	}{
		{"example.com", "example.com", false},
		{" Login.Example.COM ", "login.example.com", false},
		{"example.co.uk", "example.co.uk", false},
		{"alice.github.io", "alice.github.io", false},
		{"xn--bcher-kva.example", "xn--bcher-kva.example", false},
		{"", "", true},
		{"com", "", true},
		{"co.uk", "", true},
		{"github.io", "", true},
		{"localhost", "", true},
		{"192.168.1.10", "", true},
		{"::1", "", true},
		{".example.com", "", true},
		{"example.com.", "", true},
		{"example.com:443", "", true},
		{"example.com/path", "", true},
		{"user@example.com", "", true},
		{"*.example.com", "", true},
		{"exa mple.com", "", true},
		{strings.Repeat("a", 250) + ".com", "", true},
	}
	for _, testCase := range cases {
		relyingPartyID, err := ValidateRelyingPartyID(testCase.relyingPartyID)
		if testCase.isErrorExpected {
			if !errors.Is(err, ErrInvalidRelyingPartyID) {
				t.Errorf("%q accepted as %q (%v)", testCase.relyingPartyID, relyingPartyID, err)
			}
			continue
		}
		if err != nil || relyingPartyID != testCase.expectedID {
			t.Errorf("%q = %q, %v; want %q", testCase.relyingPartyID, relyingPartyID, err, testCase.expectedID)
		}
	}
}

func buildPlatformClientDataJSON(t *testing.T, ceremonyType protocol.CeremonyType, challenge protocol.URLEncodedBase64, origin string) ([]byte, []byte) {
	t.Helper()
	clientDataJSON, err := json.Marshal(protocol.CollectedClientData{Type: ceremonyType, Challenge: challenge.String(), Origin: origin})
	if err != nil {
		t.Fatal(err)
	}
	clientDataHash := sha256.Sum256(clientDataJSON)
	return clientDataJSON, clientDataHash[:]
}

func TestClientDataHashCeremoniesVerifiedByWebAuthnLibrary(t *testing.T) {
	relyingParty := createTestRelyingParty(t, "example.com", "https://login.example.com")
	user := &testUser{userHandle: []byte("ios-user-handle"), userName: "alex@example.com"}
	options, session, err := relyingParty.BeginRegistration(user)
	if err != nil {
		t.Fatal(err)
	}
	clientDataJSON, clientDataHash := buildPlatformClientDataJSON(t, protocol.CreateCeremony, options.Response.Challenge, "https://login.example.com")
	creation, err := CreateCredentialWithClientDataHash("Example.com", clientDataHash)
	if err != nil {
		t.Fatal(err)
	}
	if creation.ClientDataJSON != nil || len(creation.CredentialID) != CredentialIDLength {
		t.Errorf("creation = %+v", creation)
	}
	body, _ := json.Marshal(map[string]any{
		"id": encodeBase64URL(creation.CredentialID), "rawId": encodeBase64URL(creation.CredentialID), "type": "public-key", "clientExtensionResults": map[string]any{},
		"response": map[string]any{"clientDataJSON": encodeBase64URL(clientDataJSON), "attestationObject": encodeBase64URL(creation.AttestationObject)},
	})
	parsedCreation, err := protocol.ParseCredentialCreationResponseBody(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	credential, err := relyingParty.CreateCredential(user, *session, parsedCreation)
	if err != nil {
		t.Fatalf("registration from a client data hash rejected: %v", err)
	}
	if credential.AttestationFormat != "none" || !credential.Flags.UserPresent || !credential.Flags.UserVerified || !credential.Flags.BackupEligible || !credential.Flags.BackupState || credential.Authenticator.SignCount != 0 {
		t.Errorf("credential = %+v", credential)
	}
	user.credentials = []webauthn.Credential{*credential}

	assertionOptions, assertionSession, err := relyingParty.BeginDiscoverableLogin()
	if err != nil {
		t.Fatal(err)
	}
	assertionClientDataJSON, assertionClientDataHash := buildPlatformClientDataJSON(t, protocol.AssertCeremony, assertionOptions.Response.Challenge, "https://login.example.com")
	assertion, err := SignAssertionWithClientDataHash("example.com", "example.com", assertionClientDataHash, creation.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	if assertion.ClientDataJSON != nil {
		t.Errorf("assertion carries client data %s", assertion.ClientDataJSON)
	}
	assertionBody, _ := json.Marshal(map[string]any{
		"id": encodeBase64URL(creation.CredentialID), "rawId": encodeBase64URL(creation.CredentialID), "type": "public-key", "clientExtensionResults": map[string]any{},
		"response": map[string]any{
			"clientDataJSON": encodeBase64URL(assertionClientDataJSON), "authenticatorData": encodeBase64URL(assertion.AuthenticatorData),
			"signature": encodeBase64URL(assertion.Signature), "userHandle": encodeBase64URL(user.userHandle),
		},
	})
	parsedAssertion, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(assertionBody))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := relyingParty.ValidateDiscoverableLogin(func(_, userHandle []byte) (webauthn.User, error) {
		if !bytes.Equal(userHandle, user.userHandle) {
			return nil, errors.New("unknown user")
		}
		return user, nil
	}, *assertionSession, parsedAssertion); err != nil {
		t.Fatalf("assertion from a client data hash rejected: %v", err)
	}
}

func TestClientDataHashRefusals(t *testing.T) {
	validHash := make([]byte, ClientDataHashLength)
	if _, err := CreateCredentialWithClientDataHash("co.uk", validHash); !errors.Is(err, ErrInvalidRelyingPartyID) {
		t.Errorf("public suffix: %v", err)
	}
	if _, err := CreateCredentialWithClientDataHash("example.com", validHash[:31]); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("31-byte hash: %v", err)
	}
	creation, err := CreateCredentialWithClientDataHash("example.com", validHash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SignAssertionWithClientDataHash("login.example.com", "example.com", validHash, creation.PrivateKey); !errors.Is(err, ErrInvalidRelyingPartyID) {
		t.Errorf("another relying party ID: %v", err)
	}
	if _, err := SignAssertionWithClientDataHash("example.com", "example.com", nil, creation.PrivateKey); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("missing hash: %v", err)
	}
	if _, err := SignAssertionWithClientDataHash("example.com", "example.com", validHash, []byte("not a key")); err == nil {
		t.Error("garbage key accepted")
	}
}
