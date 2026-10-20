package importer

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/sshkey"
	"golang.org/x/crypto/ssh"
)

func TestSSHImportReportsReplacedSourceComments(t *testing.T) {
	for _, test := range []struct {
		name         string
		embedded     string
		public       string
		cxf          string
		wantOmission bool
	}{
		{"OpenSSH different", "work", "", "", true},
		{"OpenSSH same", "Deploy", "", "", false},
		{"OpenSSH empty", "", "", "", false},
		{"Bitwarden different", "Deploy", "home", "", true},
		{"Bitwarden same", "Deploy", "Deploy", "", false},
		{"CXF different", "Deploy", "", "home", true},
		{"CXF same", "Deploy", "", "Deploy", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			key, err := sshkey.GenerateEd25519Key(test.embedded)
			if err != nil {
				t.Fatal(err)
			}
			publicKey := key.AuthorizedKey
			if test.public != "" {
				parts := strings.Fields(publicKey)
				publicKey = strings.Join(parts[:2], " ") + " " + test.public
			}
			var source any
			if test.cxf != "" {
				raw, err := ssh.ParseRawPrivateKey(key.PrivateKeyPEM)
				if err != nil {
					t.Fatal(err)
				}
				der, err := x509.MarshalPKCS8PrivateKey(*raw.(*ed25519.PrivateKey))
				if err != nil {
					t.Fatal(err)
				}
				source = map[string]any{"version": map[string]int{"major": 1, "minor": 0}, "accounts": []any{map[string]any{"items": []any{map[string]any{"title": "Deploy", "credentials": []any{map[string]any{"type": "ssh-key", "privateKey": base64.RawURLEncoding.EncodeToString(der), "keyComment": test.cxf}}}}}}}
			} else {
				source = map[string]any{"encrypted": false, "items": []any{map[string]any{"type": 5, "name": "Deploy", "sshKey": map[string]string{"privateKey": string(key.PrivateKeyPEM), "publicKey": publicKey}}}}
			}
			content, err := json.Marshal(source)
			if err != nil {
				t.Fatal(err)
			}
			var result *Result
			if test.cxf != "" {
				result, err = ParseCredentialExchange(content)
			} else {
				result, err = Parse("source.json", content, FormatAutomatic)
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Items) != 1 {
				t.Fatalf("items = %+v", result.Items)
			}
			if !strings.HasSuffix(result.Items[0].Overview.PublicKey, " Deploy") {
				t.Errorf("imported comment is not title: %q", result.Items[0].Overview.PublicKey)
			}
			hasOmission := false
			for _, skipped := range result.Skipped {
				if strings.Contains(skipped.Reason, "SSH key comment") {
					hasOmission = true
				}
			}
			if hasOmission != test.wantOmission {
				t.Errorf("comment omission = %v, want %v: %+v", hasOmission, test.wantOmission, result.Skipped)
			}
			if test.wantOmission && CountLeftBehind(0, result.Skipped) == 0 {
				t.Error("source with lost comment can be deleted")
			}
		})
	}
}

func TestSSHImportReportsEmbeddedCommentsForEveryKeyType(t *testing.T) {
	rsaKey, err := sshkey.GenerateRSAKey(2048, "source comment")
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string][]byte{"RSA": rsaKey.PrivateKeyPEM}
	for _, curve := range []elliptic.Curve{elliptic.P256(), elliptic.P384(), elliptic.P521()} {
		key, err := ecdsa.GenerateKey(curve, rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		block, err := ssh.MarshalPrivateKey(key, "source comment")
		if err != nil {
			t.Fatal(err)
		}
		keys[curve.Params().Name] = pem.EncodeToMemory(block)
	}
	for name, key := range keys {
		t.Run(name, func(t *testing.T) {
			content, err := json.Marshal(map[string]any{"encrypted": false, "items": []any{map[string]any{"type": 5, "name": "Deploy", "sshKey": map[string]string{"privateKey": string(key)}}}})
			if err != nil {
				t.Fatal(err)
			}
			result, err := Parse("source.json", content, FormatAutomatic)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Items) != 1 || len(result.Skipped) != 1 || !strings.Contains(result.Skipped[0].Reason, "SSH key comment") {
				t.Fatalf("source comment not reported: %+v", result)
			}
		})
	}
}
