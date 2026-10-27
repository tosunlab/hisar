package app

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"testing"

	"github.com/tryhisar/hisar/core/emergencykit"
)

func TestRevealSecretKeyVerifiesPasswordWithoutSavingAFile(t *testing.T) {
	fixture := createVerifiableService(t)
	service := fixture.service
	service.config.FileSaver = nil
	if value, err := service.RevealSecretKey("wrong password"); err == nil || value != (RevealedSecretKey{}) {
		t.Fatalf("wrong password returned a Secret Key: %v", err)
	}
	value, err := service.RevealSecretKey(testMasterPassword)
	if err != nil {
		t.Fatal(err)
	}
	key, err := service.config.SecretKeyStore.LoadSecretKey(fixture.accountID)
	if err != nil {
		t.Fatal(err)
	}
	if value.SecretKey != key.Encode() || value.QRPayload != emergencykit.BuildQRPayload(service.buildEmergencyKit(key)) {
		t.Fatal("revealed key and QR payload do not name this account")
	}
	encoded, err := base64.StdEncoding.DecodeString(value.QRCode)
	if err != nil {
		t.Fatal(err)
	}
	image, err := png.Decode(bytes.NewReader(encoded))
	if err != nil || image.Bounds().Dx() != 320 || image.Bounds().Dy() != 320 {
		t.Fatalf("QR image: %v", err)
	}
	service.Lock()
	if value, err := service.RevealSecretKey(testMasterPassword); err == nil || value != (RevealedSecretKey{}) {
		t.Fatal("locked service returned a Secret Key")
	}
}

func TestRevealSecretKeyRefusesAnEndedSession(t *testing.T) {
	for _, boundary := range []string{"lock", "account switch"} {
		t.Run(boundary, func(t *testing.T) {
			fixture := createVerifiableService(t)
			service := fixture.service
			keyLoaded := newPause()
			service.config.SecretKeyStore = pausingSecretKeyStore{SecretKeyStore: service.config.SecretKeyStore, pause: keyLoaded}
			type revealed struct {
				value RevealedSecretKey
				err   error
			}
			result := make(chan revealed, 1)
			go func() {
				value, err := service.RevealSecretKey(testMasterPassword)
				result <- revealed{value, err}
			}()
			<-keyLoaded.reached
			if boundary == "lock" {
				service.Lock()
			} else {
				signInOtherAccount(t, service, createUnlockedTestService(t))
			}
			close(keyLoaded.resume)
			if got := <-result; got.err == nil || got.value != (RevealedSecretKey{}) {
				t.Fatal("verification released an old session's Secret Key")
			}
		})
	}
}
