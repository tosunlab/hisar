package app

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
)

type testPushKey struct {
	key        *ecdh.PrivateKey
	authSecret []byte
}

func createTestPushKey(t *testing.T) testPushKey {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	authSecret := make([]byte, crypt.WebPushAuthSecretSize)
	rand.Read(authSecret)
	return testPushKey{key: key, authSecret: authSecret}
}

func (pushKey testPushKey) signedBy(keyset *crypt.Keyset, accountID, deviceID string) apitypes.DevicePushKey {
	publicKey := pushKey.key.PublicKey().Bytes()
	return apitypes.DevicePushKey{
		DeviceID:  deviceID,
		P256DH:    base64.RawURLEncoding.EncodeToString(publicKey),
		Auth:      base64.RawURLEncoding.EncodeToString(pushKey.authSecret),
		Signature: base64.RawURLEncoding.EncodeToString(keyset.SignPushKey(accountID, deviceID, publicKey, pushKey.authSecret)),
	}
}

func (pushKey testPushKey) open(t *testing.T, pushMessage apitypes.ApprovalPushMessage) string {
	t.Helper()
	message, err := base64.RawURLEncoding.DecodeString(pushMessage.Message)
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := crypt.DecryptWebPush(pushKey.key, pushKey.authSecret, message)
	if err != nil {
		t.Fatalf("the phone cannot read its message: %v", err)
	}
	return string(plaintext)
}

// The Mac encrypts the request's signed preview to each phone whose push key the account signed, and to no key the
// server made up; the phone shows it only while its signature checks out and the request waits.
func TestAnApprovalPushPreviewReachesOnlyPhonesTheAccountVouchedFor(t *testing.T) {
	test := createPhoneApprovalTest(t)
	otherKeyset, err := crypt.GenerateKeyset()
	if err != nil {
		t.Fatal(err)
	}
	phoneKey, serverKey := createTestPushKey(t), createTestPushKey(t)
	test.apiServer.mutex.Lock()
	test.apiServer.pushKeys = &apitypes.DevicePushKeysResponse{AllKey: []apitypes.DevicePushKey{
		phoneKey.signedBy(test.keyset, test.accountID, testPhoneDeviceID),
		serverKey.signedBy(otherKeyset, test.accountID, "server-device"),
	}}
	test.apiServer.mutex.Unlock()

	stored, finish := test.start(t)
	defer finish()
	if len(stored.request.AllPushMessage) != 1 || stored.request.AllPushMessage[0].DeviceID != testPhoneDeviceID {
		t.Fatalf("push messages = %+v", stored.request.AllPushMessage)
	}
	message := phoneKey.open(t, stored.request.AllPushMessage[0])
	preview, err := OpenApprovalPush(test.service, message)
	if err != nil {
		t.Fatal(err)
	}
	if preview.RequestID != stored.request.RequestID || preview.RequesterDeviceName != "MacBook-Pro" || preview.Kind != "ssh" ||
		preview.AppName != "Terminal" || preview.Purpose != "push to git.tryhisar.com" || preview.KeyName != "hisar gitlab" {
		t.Fatalf("preview = %+v", preview)
	}

	var pushMessage apitypes.PushMessage
	json.Unmarshal([]byte(message), &pushMessage)
	forged := pushMessage
	forged.Approval = &apitypes.PushApproval{Preview: base64.RawURLEncoding.EncodeToString([]byte(`{"requestId":"x","kind":"ssh","appName":"Mail","expiresAt":"2999-01-01T00:00:00Z"}`)), Signature: pushMessage.Approval.Signature}
	forgedMessage, _ := json.Marshal(forged)
	for name, candidate := range map[string]string{
		"a preview the account did not sign": string(forgedMessage),
		"a sharing push":                     `{"kind":"sharing"}`,
		"not JSON":                           "approval",
	} {
		if _, err := OpenApprovalPush(test.service, candidate); err == nil {
			t.Errorf("%s opened", name)
		}
	}
	test.service.mutex.Lock()
	test.service.config.Now = func() time.Time { return preview.ExpiresAt }
	test.service.mutex.Unlock()
	if _, err := OpenApprovalPush(test.service, message); err == nil {
		t.Error("an expired preview opened")
	}
}

// A server older than approval previews has no push keys; the request goes out as before, without push messages.
func TestAnApprovalWithoutPushKeysGoesOutWithoutPreviews(t *testing.T) {
	test := createPhoneApprovalTest(t)
	stored, finish := test.start(t)
	defer finish()
	if stored.request.AllPushMessage != nil {
		t.Fatalf("push messages = %+v", stored.request.AllPushMessage)
	}
}

// A phone sends the account's signature over its push key once Hisar can sign it, and only to a server that takes it.
func TestAPhoneSignsItsPushKeyWhileUnlocked(t *testing.T) {
	fixture := createUnlockedTestService(t)
	relay := startFakeRelayForApp(t)
	fixture.apiServer.pushRelayURL = relay.server.URL
	pushKey := createTestPushKey(t)
	p256dh := base64.RawURLEncoding.EncodeToString(pushKey.key.PublicKey().Bytes())
	auth := base64.RawURLEncoding.EncodeToString(pushKey.authSecret)

	if err := fixture.service.SetWebPushEndpoint("https://ntfy.example/upAbC", p256dh, auth); err != nil {
		t.Fatal(err)
	}
	if registration := fixture.apiServer.getPushRegistration(); registration == nil || registration.KeySignature != "" {
		t.Fatalf("registration with an older server = %+v", registration)
	}

	fixture.apiServer.mutex.Lock()
	fixture.apiServer.acceptsPayloadKey = true
	fixture.apiServer.mutex.Unlock()
	if err := fixture.service.SetWebPushEndpoint("https://ntfy.example/upAbC", p256dh, auth); err != nil {
		t.Fatal(err)
	}
	registration := fixture.apiServer.getPushRegistration()
	signature, err := base64.RawURLEncoding.DecodeString(registration.KeySignature)
	if err != nil {
		t.Fatal(err)
	}
	fixture.service.mutex.Lock()
	signingPublicKey, deviceID := fixture.service.unlockedSession.Keyset.GetSigningPublicKey(), fixture.service.state.DeviceID
	fixture.service.mutex.Unlock()
	if err := crypt.VerifyPushKey(signingPublicKey, fixture.accountID, deviceID, pushKey.key.PublicKey().Bytes(), pushKey.authSecret, signature); err != nil {
		t.Fatalf("the key signature does not verify: %v", err)
	}
	if verifier := GetApprovalPushVerifier(fixture.service); verifier.AccountID != fixture.accountID || verifier.SigningPublicKey != base64.RawURLEncoding.EncodeToString(signingPublicKey) {
		t.Fatalf("verifier = %+v", verifier)
	}

	fixture.service.Lock()
	if err := fixture.service.SetWebPushEndpoint("https://ntfy.example/upAbC", p256dh, auth); err != nil {
		t.Fatal(err)
	}
	if registration := fixture.apiServer.getPushRegistration(); registration.KeySignature != "" {
		t.Fatalf("a locked phone signed its key: %+v", registration)
	}
	if verifier := GetApprovalPushVerifier(fixture.service); verifier.SigningPublicKey == "" {
		t.Fatal("locking forgot the key a locked phone checks previews with")
	}
}
