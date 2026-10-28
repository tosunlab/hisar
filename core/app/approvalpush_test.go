package app

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
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

// A phone sends the account's signature over its push key and the device ID the server knows it by, once Hisar can sign
// it, and only to a server that takes it.
func TestAPhoneSignsItsPushKeyWhileUnlocked(t *testing.T) {
	fixture := createUnlockedTestService(t)
	relay := startFakeRelayForApp(t)
	fixture.apiServer.pushRelayURL = relay.server.URL
	fixture.apiServer.devices = []apitypes.Device{{ID: testPhoneDeviceID, Name: "Pixel", Platform: apitypes.PlatformAndroid, Current: true}}
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
	fixture.service.mutex.Lock()
	signingPublicKey := fixture.service.unlockedSession.Keyset.GetSigningPublicKey()
	fixture.service.mutex.Unlock()
	if err := verifyRegisteredPushKey(fixture.apiServer.getPushRegistration(), signingPublicKey, fixture.accountID, testPhoneDeviceID, pushKey); err != nil {
		t.Fatalf("the key signature does not verify over the phone's device ID: %v", err)
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

// verifyRegisteredPushKey checks the account's signature, in a registration the server got, over pushKey and deviceID.
func verifyRegisteredPushKey(registration *apitypes.PushRegistration, signingPublicKey []byte, accountID, deviceID string, pushKey testPushKey) error {
	if registration == nil {
		return errors.New("the phone never registered")
	}
	signature, err := base64.RawURLEncoding.DecodeString(registration.KeySignature)
	if err != nil {
		return err
	}
	return crypt.VerifyPushKey(signingPublicKey, accountID, deviceID, pushKey.key.PublicKey().Bytes(), pushKey.authSecret, signature)
}

// A new phone signs its push key over the device ID its sign-up session got, the one the server and the Mac check.
func TestASignedUpPhoneSignsItsPushKeyOverItsDeviceID(t *testing.T) {
	service, _, _ := createTestService(t, filepath.Join(t.TempDir(), "hisar.json"), createMemorySecretKeyStore())
	serverURL, apiServer := startRegistrationServerWithVaults(t)
	relay := startFakeRelayForApp(t)
	apiServer.mutex.Lock()
	apiServer.pushRelayURL, apiServer.acceptsPayloadKey = relay.server.URL, true
	apiServer.devices = []apitypes.Device{{ID: testPhoneDeviceID, Name: "iPhone", Platform: apitypes.PlatformIOS, Current: true}}
	apiServer.mutex.Unlock()
	if result, err := service.SignUp(serverURL+"/invite/token", "new@example.com", "New", "correct horse battery"); err != nil || result.Status.State != StateUnlocked {
		t.Fatalf("sign-up = %+v, %v", result, err)
	}

	pushKey := createTestPushKey(t)
	if err := service.SetPushPayloadKey(base64.RawURLEncoding.EncodeToString(pushKey.key.PublicKey().Bytes()), base64.RawURLEncoding.EncodeToString(pushKey.authSecret)); err != nil {
		t.Fatal(err)
	}
	if err := service.SetPushToken("apns", "device-token"); err != nil {
		t.Fatal(err)
	}
	apiServer.mutex.Lock()
	account := apiServer.account
	apiServer.mutex.Unlock()
	if err := verifyRegisteredPushKey(apiServer.getPushRegistration(), account.PublicKeys.Sign, account.AccountID, testPhoneDeviceID, pushKey); err != nil {
		t.Fatalf("the key signature does not verify over the phone's device ID: %v", err)
	}
}

// A phone signed in by a release that never stored its device ID registered its key signed over an empty one. After
// the next unlock it registers the key again, signed over the device ID the server knows it by.
func TestAPhoneWithoutItsDeviceIDRegistersItsKeyAgainAfterUnlock(t *testing.T) {
	fixture := createUnlockedTestService(t)
	prepareTestApprovalUnlock(t, fixture, "master password")
	relay := startFakeRelayForApp(t)
	fixture.apiServer.mutex.Lock()
	fixture.apiServer.pushRelayURL, fixture.apiServer.acceptsPayloadKey = relay.server.URL, true
	fixture.apiServer.devices = []apitypes.Device{
		{ID: testMacDeviceID, Name: "MacBook Pro", Platform: "darwin/desktop"},
		{ID: testPhoneDeviceID, Name: "iPhone", Platform: apitypes.PlatformIOS, Current: true},
	}
	fixture.apiServer.mutex.Unlock()
	fixture.service.mutex.Lock()
	signingPublicKey := fixture.service.unlockedSession.Keyset.GetSigningPublicKey()
	fixture.service.mutex.Unlock()

	fixture.service.Lock()
	pushKey := createTestPushKey(t)
	if err := fixture.service.SetPushPayloadKey(base64.RawURLEncoding.EncodeToString(pushKey.key.PublicKey().Bytes()), base64.RawURLEncoding.EncodeToString(pushKey.authSecret)); err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.SetPushToken("apns", "device-token"); err != nil {
		t.Fatal(err)
	}
	if registration := fixture.apiServer.getPushRegistration(); registration == nil || registration.KeySignature != "" {
		t.Fatalf("registration while locked = %+v", registration)
	}
	if _, err := fixture.service.Unlock("master password"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for verifyRegisteredPushKey(fixture.apiServer.getPushRegistration(), signingPublicKey, fixture.accountID, testPhoneDeviceID, pushKey) != nil {
		if time.Now().After(deadline) {
			t.Fatalf("after unlock the server has %+v, not a key signed over the phone's device ID", fixture.apiServer.getPushRegistration())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A released iPhone signed its push key over an empty device ID. The Mac encrypts the preview to that key as to a key
// signed over the phone's own device ID, and not to a key the account signed for another device.
func TestAnApprovalPreviewReachesAPhoneThatSignedItsKeyWithoutItsDeviceID(t *testing.T) {
	test := createPhoneApprovalTest(t)
	phoneKey, releasedPhoneKey, otherDeviceKey := createTestPushKey(t), createTestPushKey(t), createTestPushKey(t)
	releasedPhone := releasedPhoneKey.signedBy(test.keyset, test.accountID, "")
	releasedPhone.DeviceID = "released-phone"
	otherDevice := otherDeviceKey.signedBy(test.keyset, test.accountID, "another-device")
	otherDevice.DeviceID = "third-phone"
	test.apiServer.mutex.Lock()
	test.apiServer.pushKeys = &apitypes.DevicePushKeysResponse{AllKey: []apitypes.DevicePushKey{
		phoneKey.signedBy(test.keyset, test.accountID, testPhoneDeviceID), releasedPhone, otherDevice,
	}}
	test.apiServer.mutex.Unlock()

	stored, finish := test.start(t)
	defer finish()
	keysByDeviceID := map[string]testPushKey{testPhoneDeviceID: phoneKey, "released-phone": releasedPhoneKey}
	if len(stored.request.AllPushMessage) != len(keysByDeviceID) {
		t.Fatalf("push messages = %+v", stored.request.AllPushMessage)
	}
	for _, pushMessage := range stored.request.AllPushMessage {
		pushKey, isExpected := keysByDeviceID[pushMessage.DeviceID]
		if !isExpected {
			t.Fatalf("a preview went to %s", pushMessage.DeviceID)
		}
		if preview, err := OpenApprovalPush(test.service, pushKey.open(t, pushMessage)); err != nil || preview.RequestID != stored.request.RequestID {
			t.Fatalf("%s opens %+v, %v", pushMessage.DeviceID, preview, err)
		}
	}
}
