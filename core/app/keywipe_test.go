package app

import (
	"bytes"
	"testing"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
)

func TestLockClearsOwnedKeyBytes(t *testing.T) {
	fixture := createUnlockedTestService(t)
	service := fixture.service
	if _, err := service.ListVaults(); err != nil {
		t.Fatal(err)
	}
	keys := map[string][]byte{}
	newItem := func(name string) item.Item {
		privateKey, prfSecret := crypt.GenerateKey(), crypt.GenerateKey()
		keys[name+" private key"], keys[name+" PRF secret"] = privateKey, prfSecret
		return item.Item{Details: item.Details{Passkeys: []item.Passkey{{PrivateKey: privateKey, PRFSecret: prfSecret}}}}
	}
	service.mutex.Lock()
	for id, vault := range service.vaultsByID {
		keys[id] = vault.Key
	}
	keys["user key"] = service.unlockedSession.Keyset.UserKey
	service.itemsByVaultID[fixture.personalID] = []item.Item{newItem("cached passkey")}
	service.pendingImport = &pendingImport{items: []item.Item{newItem("import preview")}, queuedItems: []item.Item{newItem("import queue")}}
	secretKey, err := crypt.GenerateSecretKey(fixture.accountID)
	if err != nil {
		service.mutex.Unlock()
		t.Fatal(err)
	}
	service.newAccountSecretKey = &secretKey
	service.newAccountUnlockKey = crypt.GenerateKey()
	keys["account unlock key"] = service.newAccountUnlockKey
	service.mutex.Unlock()

	service.Lock()
	for name, key := range keys {
		if !bytes.Equal(key, make([]byte, len(key))) {
			t.Errorf("%s survived the lock", name)
		}
	}
	if secretKey != (crypt.SecretKey{}) {
		t.Error("the onboarding Secret Key survived the lock")
	}
}
