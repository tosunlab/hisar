package pushrelay

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/tryhisar/hisar/core/apitypes"
)

func newTestSealer(t *testing.T) *Sealer {
	t.Helper()
	sealer, err := NewSealer(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return sealer
}

func TestHandleOpensOnlyWithItsKeyAndUntouched(t *testing.T) {
	sealer := newTestSealer(t)
	handle, err := sealer.Seal(apitypes.PushPlatformAPNs, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(handle, "abc123") {
		t.Fatal("the handle shows the token")
	}
	platform, token, err := sealer.Open(handle)
	if err != nil || platform != apitypes.PushPlatformAPNs || token != "abc123" {
		t.Fatalf("open = %q %q %v", platform, token, err)
	}
	again, _ := sealer.Seal(apitypes.PushPlatformAPNs, "abc123")
	if again == handle {
		t.Fatal("two handles for one token are equal, so a server could link them")
	}
	sealed, _ := base64.RawURLEncoding.DecodeString(handle)
	sealed[len(sealed)-1] ^= 1
	if _, _, err := sealer.Open(base64.RawURLEncoding.EncodeToString(sealed)); err != ErrInvalidHandle {
		t.Fatalf("a changed handle opened: %v", err)
	}
	other, _ := NewSealer(bytes.Repeat([]byte{8}, 32))
	if _, _, err := other.Open(handle); err != ErrInvalidHandle {
		t.Fatalf("another key opened the handle: %v", err)
	}
	if _, err := sealer.Seal("webpush", "abc"); err == nil {
		t.Fatal("an unknown platform was sealed")
	}
}

type fakeAPNs struct {
	mutex       sync.Mutex
	allRequest  []*http.Request
	allBody     [][]byte
	unknownPath string
	server      *httptest.Server
}

func newFakeAPNs(t *testing.T) *fakeAPNs {
	fake := &fakeAPNs{}
	fake.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		fake.mutex.Lock()
		fake.allRequest = append(fake.allRequest, request)
		fake.allBody = append(fake.allBody, body)
		fake.mutex.Unlock()
		if request.URL.Path == fake.unknownPath {
			writer.WriteHeader(http.StatusGone)
			writer.Write([]byte(`{"reason":"Unregistered"}`))
			return
		}
		writer.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(fake.server.Close)
	return fake
}

func verifyES256(t *testing.T, token string, key *ecdsa.PublicKey) map[string]any {
	t.Helper()
	allPart := strings.Split(token, ".")
	if len(allPart) != 3 {
		t.Fatalf("not a JWT: %q", token)
	}
	signature, _ := base64.RawURLEncoding.DecodeString(allPart[2])
	digest := sha256.Sum256([]byte(allPart[0] + "." + allPart[1]))
	if len(signature) != 64 || !ecdsa.Verify(key, digest[:], new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:])) {
		t.Fatal("the provider token's signature does not verify")
	}
	claims := map[string]any{}
	payload, _ := base64.RawURLEncoding.DecodeString(allPart[1])
	json.Unmarshal(payload, &claims)
	return claims
}

func pushThrough(t *testing.T, relay *Relay, handle string, kind apitypes.PushKind) int {
	t.Helper()
	body, _ := json.Marshal(apitypes.RelayPushRequest{Handle: handle, Kind: kind})
	recorder := httptest.NewRecorder()
	relay.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/push", bytes.NewReader(body)))
	return recorder.Code
}

func register(t *testing.T, relay *Relay, platform apitypes.PushPlatform, token string) string {
	t.Helper()
	body, _ := json.Marshal(apitypes.RelayRegisterRequest{Platform: platform, Token: token})
	recorder := httptest.NewRecorder()
	relay.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/register", bytes.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("register answered %d", recorder.Code)
	}
	var answer apitypes.RelayRegisterResponse
	json.Unmarshal(recorder.Body.Bytes(), &answer)
	return answer.Handle
}

func TestAPNsPushCarriesOnlyTheKindAndASignedProviderToken(t *testing.T) {
	fake := newFakeAPNs(t)
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	sender := NewAPNsSender(APNsConfig{KeyID: "KEY123", TeamID: "TEAM123", Topic: "com.tryhisar.hisar", Key: key, BaseURL: fake.server.URL, SandboxBaseURL: fake.server.URL + "/sandbox"})
	relay := NewRelay(newTestSealer(t), map[apitypes.PushPlatform]Sender{apitypes.PushPlatformAPNs: sender, apitypes.PushPlatformAPNsSandbox: sender})

	handle := register(t, relay, apitypes.PushPlatformAPNs, "device-token-1")
	if code := pushThrough(t, relay, handle, apitypes.PushKindApproval); code != http.StatusNoContent {
		t.Fatalf("push answered %d", code)
	}
	request := fake.allRequest[0]
	if request.URL.Path != "/3/device/device-token-1" || request.Header.Get("apns-topic") != "com.tryhisar.hisar" || request.Header.Get("apns-push-type") != "alert" {
		t.Fatalf("request = %s %v", request.URL.Path, request.Header)
	}
	claims := verifyES256(t, strings.TrimPrefix(request.Header.Get("authorization"), "bearer "), &key.PublicKey)
	if claims["iss"] != "TEAM123" {
		t.Fatalf("claims = %v", claims)
	}
	var payload map[string]any
	json.Unmarshal(fake.allBody[0], &payload)
	want := `{"aps":{"alert":{"loc-key":"push.approval"},"sound":"default","thread-id":"approval"},"kind":"approval"}`
	if got, _ := json.Marshal(payload); string(got) != want {
		t.Fatalf("payload = %s", got)
	}

	sandboxHandle := register(t, relay, apitypes.PushPlatformAPNsSandbox, "device-token-2")
	pushThrough(t, relay, sandboxHandle, apitypes.PushKindSecurity)
	if fake.allRequest[1].URL.Path != "/sandbox/3/device/device-token-2" {
		t.Fatalf("a development build's push went to %s", fake.allRequest[1].URL.Path)
	}
}

func TestRelayAnswersGoneForAnUnregisteredTokenAndLimitsEachKind(t *testing.T) {
	fake := newFakeAPNs(t)
	fake.unknownPath = "/3/device/removed"
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	relay := NewRelay(newTestSealer(t), map[apitypes.PushPlatform]Sender{apitypes.PushPlatformAPNs: NewAPNsSender(APNsConfig{Key: key, BaseURL: fake.server.URL})})

	if code := pushThrough(t, relay, register(t, relay, apitypes.PushPlatformAPNs, "removed"), apitypes.PushKindApproval); code != http.StatusGone {
		t.Fatalf("an unregistered token answered %d", code)
	}
	handle := register(t, relay, apitypes.PushPlatformAPNs, "live")
	if code := pushThrough(t, relay, handle, apitypes.PushKindApproval); code != http.StatusNoContent {
		t.Fatalf("first push answered %d", code)
	}
	if code := pushThrough(t, relay, register(t, relay, apitypes.PushPlatformAPNs, "live"), apitypes.PushKindApproval); code != http.StatusAccepted {
		t.Fatalf("a second approval push within the interval, through a new handle, answered %d", code)
	}
	if code := pushThrough(t, relay, handle, apitypes.PushKindSecurity); code != http.StatusNoContent {
		t.Fatalf("a push of another kind answered %d", code)
	}
	if len(fake.allRequest) != 3 {
		t.Fatalf("APNs got %d requests", len(fake.allRequest))
	}
	if code := pushThrough(t, relay, handle, "marketing"); code != http.StatusBadRequest {
		t.Fatalf("an unknown kind answered %d", code)
	}
	if code := pushThrough(t, relay, "not-a-handle", apitypes.PushKindApproval); code != http.StatusBadRequest {
		t.Fatalf("a made-up handle answered %d", code)
	}
}

func TestRelayRefusesPlatformsItHasNoSenderFor(t *testing.T) {
	relay := NewRelay(newTestSealer(t), map[apitypes.PushPlatform]Sender{})
	body, _ := json.Marshal(apitypes.RelayRegisterRequest{Platform: "fcm", Token: "token"})
	recorder := httptest.NewRecorder()
	relay.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/register", bytes.NewReader(body)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("register answered %d", recorder.Code)
	}
}

func mustJSON(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
