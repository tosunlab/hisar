package app

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func importFixtureReplacer(t *testing.T) *strings.Replacer {
	t.Helper()
	var replacements []string
	for _, kind := range []string{"PKCS8", "OPENSSH", "PROTECTED"} {
		public, private, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		var block *pem.Block
		switch kind {
		case "PKCS8":
			der, err := x509.MarshalPKCS8PrivateKey(private)
			if err != nil {
				t.Fatal(err)
			}
			block = &pem.Block{Type: "PRIVATE KEY", Bytes: der}
		case "OPENSSH":
			block, err = ssh.MarshalPrivateKey(private, "")
		case "PROTECTED":
			block, err = ssh.MarshalPrivateKeyWithPassphrase(private, "", []byte("secret phrase"))
		}
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(string(pem.EncodeToMemory(block)))
		if err != nil {
			t.Fatal(err)
		}
		sshPublic, err := ssh.NewPublicKey(public)
		if err != nil {
			t.Fatal(err)
		}
		replacements = append(replacements,
			"{{"+kind+"_PRIVATE_KEY}}", string(encoded[1:len(encoded)-1]),
			"{{"+kind+"_PUBLIC_KEY}}", strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPublic))),
			"{{"+kind+"_FINGERPRINT}}", ssh.FingerprintSHA256(sshPublic))
	}
	return strings.NewReplacer(replacements...)
}

func TestIdenticalFixtureReimportKeepsRetentionCount(t *testing.T) {
	replacer := importFixtureReplacer(t)
	for _, name := range []string{"bitwarden.csv", "1password.csv", "1password-export.data", "bitwarden.json", "lastpass.csv", "chrome.csv", "firefox.csv", "safari.csv"} {
		t.Run(name, func(t *testing.T) {
			test := createUnlockedTestService(t)
			content, err := os.ReadFile("../importer/testdata/" + name)
			if err != nil {
				t.Fatal(err)
			}
			content = []byte(replacer.Replace(string(content)))
			if name == "1password-export.data" {
				importOnePasswordExport(t, test.service, test.personalID, string(content))
			} else {
				setImportPath(test.service, writeImportFile(t, name, string(content)))
				if _, err := test.service.ChooseImportFile(test.personalID, ""); err != nil {
					t.Fatal(err)
				}
				if _, err := test.service.StartImport(test.personalID, true); err != nil {
					t.Fatal(err)
				}
				runImportToEnd(t, test.service)
			}
			first, _ := runImportToEnd(t, test.service)
			if _, err := test.service.ChooseImportFile(test.personalID, ""); err != nil {
				t.Fatal(err)
			}
			if _, err := test.service.StartImport(test.personalID, true); err != nil {
				t.Fatal(err)
			}
			second, _ := runImportToEnd(t, test.service)
			if second.TotalCount != 0 || second.LeftBehindCount != first.LeftBehindCount {
				t.Errorf("identical reimport = %+v, first left behind = %d", second, first.LeftBehindCount)
			}
		})
	}
}

func TestIdenticalStarredAndSSHReimportsAllowDeletion(t *testing.T) {
	replacer := importFixtureReplacer(t)
	for name, content := range map[string]string{
		"starred.csv": "name,url,username,password,favorite\nGitHub,https://github.com,alice,password,1\n",
		"ssh.json":    replacer.Replace(`{"encrypted":false,"items":[{"type":5,"name":"Deploy","sshKey":{"privateKey":"{{OPENSSH_PRIVATE_KEY}}"}}]}`),
	} {
		t.Run(name, func(t *testing.T) {
			test := createUnlockedTestService(t)
			path := writeImportFile(t, name, content)
			setImportPath(test.service, path)
			for round := range 2 {
				if _, err := test.service.ChooseImportFile(test.personalID, ""); err != nil {
					t.Fatal(err)
				}
				if _, err := test.service.StartImport(test.personalID, true); err != nil {
					t.Fatal(err)
				}
				progress, _ := runImportToEnd(t, test.service)
				if progress.LeftBehindCount != 0 {
					t.Errorf("round %d left behind = %d", round, progress.LeftBehindCount)
				}
			}
			if err := test.service.DeleteImportFile(); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Errorf("identical source remains: %v", err)
			}
		})
	}
}

func TestMergedPasskeyRetentionOnlyCountsOtherDifferences(t *testing.T) {
	for _, different := range []bool{false, true} {
		t.Run(map[bool]string{false: "identical", true: "different password"}[different], func(t *testing.T) {
			test := createUnlockedTestService(t)
			saved, err := test.service.SaveItem(test.personalID, ItemDetail{Title: "GitHub", Username: "alice", URLs: []string{"https://github.com"}, Password: "password"})
			if err != nil {
				t.Fatal(err)
			}
			credential := createExchangedPasskey(t, 1).encode(t, "github.com", "alice")
			delete(credential, "fido2Extensions")
			password := "password"
			if different {
				password = "new-password"
			}
			content := buildCredentialExchange(t, map[string]any{"title": "GitHub", "scope": map[string]any{"urls": []string{"https://github.com"}}, "credentials": []any{
				map[string]any{"type": "basic-auth", "username": map[string]any{"fieldType": "string", "value": "alice"}, "password": map[string]any{"fieldType": "concealed-string", "value": password}}, credential,
			}})
			for round := range 2 {
				if _, err := PrepareCredentialExchangeImport(test.service, test.personalID, content); err != nil {
					t.Fatal(err)
				}
				if _, err := test.service.StartImport(test.personalID, true); err != nil {
					t.Fatal(err)
				}
				beforeSave, err := test.service.GetItem(test.personalID, saved.ID)
				if err != nil || len(beforeSave.Passkeys) != round {
					t.Fatalf("queued passkey appeared before its save: %+v, %v", beforeSave, err)
				}
				progress, _ := runImportToEnd(t, test.service)
				want := 0
				if different {
					want = 1
				}
				if progress.LeftBehindCount != want {
					t.Errorf("round %d left behind = %d, want %d", round, progress.LeftBehindCount, want)
				}
			}
			loaded, err := test.service.GetItem(test.personalID, saved.ID)
			if err != nil || len(loaded.Passkeys) != 1 || loaded.Password != "password" {
				t.Fatalf("merged login = %+v, %v", loaded, err)
			}
		})
	}
}
