//go:build darwin && cgo

package biometric

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestKeychainRoundtrip(t *testing.T) {
	biometric := New("hisar-test-" + fmt.Sprint(time.Now().UnixNano()))
	t.Logf("Keychain test service: %s", biometric.service)
	accountID := "TEST01"
	t.Cleanup(func() { biometric.DeleteKey(accountID) })

	if os.Getenv("HISAR_TEST_SIGNED_BIOMETRIC") != "1" {
		if err := biometric.StoreKey(accountID, bytes.Repeat([]byte{7}, 32)); err == nil {
			t.Fatal("unprovisioned test executable created an OS-protected biometric key")
		}
		return
	}
	if _, err := biometric.LoadKey(accountID); !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("load before store: %v", err)
	}
	firstKey := bytes.Repeat([]byte{7}, 32)
	if err := biometric.StoreKey(accountID, firstKey); err != nil {
		t.Fatal(err)
	}
	if loaded, err := biometric.LoadKey(accountID); err != nil || !bytes.Equal(loaded, firstKey) {
		t.Fatalf("load: %x %v", loaded, err)
	}
	secondKey := bytes.Repeat([]byte{9}, 32)
	if err := biometric.StoreKey(accountID, secondKey); err != nil {
		t.Fatal(err)
	}
	if loaded, _ := biometric.LoadKey(accountID); !bytes.Equal(loaded, secondKey) {
		t.Errorf("store did not replace the key: %x", loaded)
	}
	if err := biometric.DeleteKey(accountID); err != nil {
		t.Fatal(err)
	}
	if _, err := biometric.LoadKey(accountID); !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("load after delete: %v", err)
	}
	if err := biometric.DeleteKey(accountID); err != nil {
		t.Errorf("delete of missing key: %v", err)
	}
	if err := biometric.StoreKey(accountID, nil); err == nil {
		t.Error("empty key stored")
	}
}

func TestIsAvailableDoesNotPrompt(t *testing.T) {
	t.Logf("Touch ID available: %v", New("hisar-test").IsAvailable())
}
