package app

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"encoding/pem"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/sshagent"
	"github.com/tryhisar/hisar/core/sshkey"
)

func checkSSHKeyDetail(t *testing.T, detail ItemDetail, title, keyType string) {
	t.Helper()
	if detail.ID == "" || detail.Category != item.CategorySSHKey || detail.Title != title || detail.KeyType != keyType {
		t.Errorf("detail = %+v", detail)
	}
	if !strings.HasPrefix(detail.Fingerprint, "SHA256:") || !strings.HasPrefix(detail.PublicKey, keyType+" ") || !strings.HasSuffix(detail.PublicKey, " "+title) {
		t.Errorf("key material = %q %q", detail.Fingerprint, detail.PublicKey)
	}
	encoded, _ := json.Marshal(detail)
	if strings.Contains(string(encoded), "PRIVATE KEY") || strings.Contains(string(encoded), "privateKey") {
		t.Errorf("detail contains the private key: %s", encoded)
	}
}

func TestGenerateSSHKey(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service

	ed25519Detail, err := service.GenerateSSHKey(test.personalID, "  Laptop  ", "")
	if err != nil {
		t.Fatal(err)
	}
	checkSSHKeyDetail(t, ed25519Detail, "Laptop", ssh.KeyAlgoED25519)
	cachedItem := getCachedItem(t, service, test.personalID, ed25519Detail.ID)
	signer, err := sshkey.ParseKeyMaterial([]byte(cachedItem.Details.PrivateKey))
	if err != nil {
		t.Fatalf("stored private key: %v", err)
	}
	if ssh.FingerprintSHA256(signer.PublicKey()) != ed25519Detail.Fingerprint {
		t.Error("fingerprint does not match the stored private key")
	}

	rsaDetail, err := service.GenerateSSHKey(test.personalID, "Legacy server", "RSA")
	if err != nil {
		t.Fatal(err)
	}
	checkSSHKeyDetail(t, rsaDetail, "Legacy server", ssh.KeyAlgoRSA)
	rsaItem := getCachedItem(t, service, test.personalID, rsaDetail.ID)
	rsaSigner, err := sshkey.ParseKeyMaterial([]byte(rsaItem.Details.PrivateKey))
	if err != nil {
		t.Fatal(err)
	}
	if bitLength := rsaSigner.PublicKey().(ssh.CryptoPublicKey).CryptoPublicKey().(*rsa.PublicKey).N.BitLen(); bitLength != 4096 {
		t.Errorf("RSA key has %d bits, want 4096", bitLength)
	}

	if _, err := service.GenerateSSHKey(test.personalID, "Old", "dsa"); err == nil || !strings.Contains(err.Error(), "Ed25519 or RSA") {
		t.Errorf("unsupported key type: %v", err)
	}
	if _, err := service.GenerateSSHKey(test.personalID, " ", ""); err == nil || !strings.Contains(err.Error(), "title") {
		t.Errorf("empty title: %v", err)
	}
	if _, err := service.GenerateSSHKey("missing", "Laptop", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing vault: %v", err)
	}

	allItem, err := service.ListAllItem(test.personalID)
	if err != nil || len(allItem) != 2 {
		t.Fatalf("list after reload: %+v %v", allItem, err)
	}
	for _, listed := range allItem {
		checkSSHKeyDetail(t, listed, listed.Title, listed.KeyType)
	}
	if reloaded := getCachedItem(t, service, test.personalID, ed25519Detail.ID); reloaded.Details.PrivateKey != cachedItem.Details.PrivateKey {
		t.Error("private key changed after reloading from the server")
	}
}

func createPrivateKeyText(t *testing.T, passphrase string) (string, string) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(privateKey, "imported")
	if passphrase != "" {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(privateKey, "imported", []byte(passphrase))
	}
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(block)), ssh.FingerprintSHA256(signer.PublicKey())
}

func TestImportSSHKey(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	encryptedKey, fingerprint := createPrivateKeyText(t, "correct passphrase")

	for _, testCase := range []struct {
		name, privateKey, passphrase, wantError string
	}{
		{"empty", "  ", "", "paste the private key"},
		{"not a key", "hello", "", "not a private key"},
		{"passphrase missing", encryptedKey, "", "protected by a passphrase"},
		{"wrong passphrase", encryptedKey, "wrong", "passphrase is not correct"},
	} {
		if _, err := service.ImportSSHKey(test.personalID, "Imported", testCase.privateKey, testCase.passphrase); err == nil || !strings.Contains(err.Error(), testCase.wantError) {
			t.Errorf("%s: %v, want %q", testCase.name, err, testCase.wantError)
		}
	}

	detail, err := service.ImportSSHKey(test.personalID, "Imported", "\n"+encryptedKey+"\n\n", "correct passphrase")
	if err != nil {
		t.Fatal(err)
	}
	checkSSHKeyDetail(t, detail, "Imported", ssh.KeyAlgoED25519)
	if detail.Fingerprint != fingerprint {
		t.Errorf("fingerprint = %s, want %s", detail.Fingerprint, fingerprint)
	}
	cachedItem := getCachedItem(t, service, test.personalID, detail.ID)
	if _, err := ssh.ParsePrivateKey([]byte(cachedItem.Details.PrivateKey)); err != nil {
		t.Errorf("stored key must not need the passphrase: %v", err)
	}

	plainKey, plainFingerprint := createPrivateKeyText(t, "")
	plainDetail, err := service.ImportSSHKey(test.familyID, "Plain", plainKey, "ignored passphrase")
	if err != nil || plainDetail.Fingerprint != plainFingerprint {
		t.Errorf("import a key without passphrase: %+v %v", plainDetail, err)
	}

	service.Lock()
	if _, err := service.ImportSSHKey(test.personalID, "Imported", encryptedKey, "correct passphrase"); !errors.Is(err, ErrLocked) {
		t.Errorf("import while locked: %v", err)
	}
	if _, err := service.GenerateSSHKey(test.personalID, "Laptop", ""); !errors.Is(err, ErrLocked) {
		t.Errorf("generate while locked: %v", err)
	}
}

func TestSaveItemKeepsSSHKeyMaterial(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	generated, err := service.GenerateSSHKey(test.personalID, "Laptop", "")
	if err != nil {
		t.Fatal(err)
	}
	originalItem := getCachedItem(t, service, test.personalID, generated.ID)

	edited := generated
	edited.Title = " Work laptop "
	edited.Notes = "Rotated every year"
	edited.Category = item.CategoryLogin
	edited.PublicKey, edited.Fingerprint, edited.KeyType = "ssh-ed25519 AAAAtampered", "SHA256:tampered", "ssh-rsa"
	edited.Username, edited.Password, edited.URLs = "alice", "password", []string{"https://example.com"}
	edited.Fields = []ItemField{{Name: "Extra", Value: "value"}}
	saved, err := service.SaveItem(test.personalID, edited)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Title != "Work laptop" || saved.Notes != "Rotated every year" || saved.Revision == generated.Revision {
		t.Errorf("saved = %+v", saved)
	}
	if saved.Category != item.CategorySSHKey || saved.PublicKey != generated.PublicKey || saved.Fingerprint != generated.Fingerprint || saved.KeyType != generated.KeyType {
		t.Errorf("key material changed: %+v", saved)
	}
	if saved.Username != "" || saved.Password != "" || len(saved.URLs) != 0 || len(saved.Fields) != 0 {
		t.Errorf("login fields stored on an SSH key: %+v", saved)
	}

	if _, err := service.ListAllItem(test.personalID); err != nil {
		t.Fatal(err)
	}
	reloaded := getCachedItem(t, service, test.personalID, generated.ID)
	if reloaded.Details.PrivateKey != originalItem.Details.PrivateKey || reloaded.Overview.PublicKey != originalItem.Overview.PublicKey {
		t.Error("private or public key changed on the server")
	}
	if reloaded.Overview.Title != "Work laptop" || reloaded.Details.Notes != "Rotated every year" {
		t.Errorf("reloaded = %+v", reloaded.Overview)
	}

	if _, err := service.SaveItem(test.personalID, ItemDetail{Title: "Fake key", Category: item.CategorySSHKey, PublicKey: "ssh-ed25519 AAAA"}); err == nil || !strings.Contains(err.Error(), "generate or import") {
		t.Errorf("new ssh-key item through SaveItem: %v", err)
	}

	login, err := service.SaveItem(test.personalID, ItemDetail{Title: "GitHub", Username: "alice", Password: "hunter2hunter2", PublicKey: "ignored"})
	if err != nil {
		t.Fatal(err)
	}
	login.Category = item.CategorySSHKey
	login.Notes = "edited"
	savedLogin, err := service.SaveItem(test.personalID, login)
	if err != nil {
		t.Fatal(err)
	}
	if savedLogin.Category != "" || savedLogin.Password != "hunter2hunter2" || savedLogin.PublicKey != "" || savedLogin.Notes != "edited" {
		t.Errorf("login after edit: %+v", savedLogin)
	}
}

func TestCopyFieldPublicKey(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	generated, err := service.GenerateSSHKey(test.personalID, "Laptop", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.CopyField(test.personalID, generated.ID, "public key"); err != nil {
		t.Fatal(err)
	}
	if text, _ := test.clipboard.GetText(); text != generated.PublicKey {
		t.Errorf("clipboard = %q, want %q", text, generated.PublicKey)
	}
	if err := service.CopyField(test.personalID, generated.ID, "fingerprint"); err != nil {
		t.Fatal(err)
	}
	if text, _ := test.clipboard.GetText(); text != generated.Fingerprint {
		t.Errorf("clipboard = %q, want the fingerprint", text)
	}
	for _, fieldName := range []string{"private key", "privateKey", "password"} {
		if err := service.CopyField(test.personalID, generated.ID, fieldName); err == nil {
			t.Errorf("%s copied", fieldName)
		}
		if text, _ := test.clipboard.GetText(); strings.Contains(text, "PRIVATE KEY") {
			t.Fatalf("private key copied with %q", fieldName)
		}
	}
}

func TestListAllKey(t *testing.T) {
	signedOut, _, _ := createTestService(t, filepath.Join(t.TempDir(), "desktop.json"), createMemorySecretKeyStore())
	if allKey, err := CreateSSHKeySource(signedOut).ListAllKey(context.Background()); !errors.Is(err, ErrSignedOut) || len(allKey) != 0 {
		t.Errorf("signed out: %d keys, %v", len(allKey), err)
	}

	test := createUnlockedTestService(t)
	service := test.service
	keySource := CreateSSHKeySource(service)
	familyKey, err := service.GenerateSSHKey(test.familyID, "Family server", "")
	if err != nil {
		t.Fatal(err)
	}
	personalKey, err := service.GenerateSSHKey(test.personalID, "Laptop", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveItem(test.personalID, ItemDetail{Title: "GitHub", Password: "hunter2hunter2"}); err != nil {
		t.Fatal(err)
	}

	checkAllKey := func(stage string) {
		t.Helper()
		allKey, err := keySource.ListAllKey(context.Background())
		if err != nil {
			t.Fatalf("%s: %v", stage, err)
		}
		if len(allKey) != 2 || allKey[0].ID != personalKey.ID || allKey[1].ID != familyKey.ID {
			t.Fatalf("%s: keys = %+v", stage, allKey)
		}
		if allKey[0].VaultID != test.personalID || allKey[0].Title != "Laptop" || allKey[1].VaultID != test.familyID {
			t.Errorf("%s: key metadata = %+v", stage, allKey)
		}
		for _, key := range allKey {
			if _, err := sshkey.ParseKeyMaterial(key.PrivateKeyPEM); err != nil {
				t.Errorf("%s: %s: %v", stage, key.Title, err)
			}
		}
	}
	checkAllKey("cached")

	service.mutex.Lock()
	service.allVaultByID = nil
	service.allItemByVaultID = map[string][]item.Item{}
	service.mutex.Unlock()
	checkAllKey("loaded from the server")

	if *test.lockedCount != 0 {
		t.Fatalf("OnLock called %d times before lock", *test.lockedCount)
	}
	if status := service.Lock(); status.State != StateLocked {
		t.Fatalf("status = %+v", status)
	}
	if *test.lockedCount != 1 {
		t.Errorf("OnLock called %d times after lock", *test.lockedCount)
	}
	if allKey, err := keySource.ListAllKey(context.Background()); !errors.Is(err, ErrLocked) || !errors.Is(err, sshagent.ErrLocked) || err.Error() != ErrLocked.Error() || len(allKey) != 0 {
		t.Errorf("locked: %d keys, %v", len(allKey), err)
	}
	service.Lock()
	if *test.lockedCount != 1 {
		t.Errorf("OnLock called again while already locked: %d", *test.lockedCount)
	}
}
