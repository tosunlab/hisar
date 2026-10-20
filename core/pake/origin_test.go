package pake

import (
	"bytes"
	"testing"

	"github.com/bytemare/opaque"
)

func TestCanonicalOrigin(t *testing.T) {
	for input, expected := range map[string]string{
		"https://EXAMPLE.com:443/hisar":         "https://example.com",
		"http://example.com:080/prefix/":        "http://example.com",
		"https://example.com:8443/":             "https://example.com:8443",
		"https://bücher.example/":               "https://xn--bcher-kva.example",
		"https://[2001:0db8:0:0::1]:443/prefix": "https://[2001:db8::1]",
		"http://[::1]:8080/":                    "http://[::1]:8080",
		"http://127.0.0.1:4567":                 "http://127.0.0.1:4567",
	} {
		actual, err := CanonicalOrigin(input)
		if err != nil || actual != expected {
			t.Errorf("%q: got %q, %v; want %q", input, actual, err, expected)
		}
	}
	for _, input := range []string{"", "example.com", "ftp://example.com", "https://user:pass@example.com", "https://example.com.", "https://example.com:", "https://example.com:0", "https://example.com:65536", "https://example.com?alias=a", "https://example.com#", "https://[fe80::1%25en0]", "https://127.1", "http://0177.0.0.1", "http://0x7f000001", "http://2130706433", "http://exa_mple.com", "http://-example.com", "http://[127.0.0.1]"} {
		if actual, err := CanonicalOrigin(input); err == nil {
			t.Errorf("accepted ambiguous origin %q as %q", input, actual)
		}
	}
}

func TestOldRegistrationUsesBoundAuthentication(t *testing.T) {
	legacy := NewConfig()
	private, public := legacy.KeyGen()
	material := &opaque.ServerKeyMaterial{PrivateKey: private, PublicKeyBytes: public.Encode(), OPRFGlobalSeed: legacy.GenerateOPRFSeed(), Identity: ServerIdentity()}
	registrationServer, err := legacy.Server()
	if err != nil {
		t.Fatal(err)
	}
	if err := registrationServer.SetKeyMaterial(material); err != nil {
		t.Fatal(err)
	}
	registrant, err := legacy.Client()
	if err != nil {
		t.Fatal(err)
	}
	defer registrant.ClearState()
	password := []byte("already derived auth key")
	account := []byte("account identity")
	request, err := registrant.RegistrationInit(password)
	if err != nil {
		t.Fatal(err)
	}
	response, err := registrationServer.RegistrationResponse(request, account, nil)
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := registrant.RegistrationFinalize(response, account, ServerIdentity())
	if err != nil {
		t.Fatal(err)
	}

	for _, purpose := range []AuthPurpose{Login, Reauthenticate} {
		serverConfig, err := NewAuthConfig("https://intended.example/hisar", purpose)
		if err != nil {
			t.Fatal(err)
		}
		server, err := serverConfig.Server()
		if err != nil {
			t.Fatal(err)
		}
		if err := server.SetKeyMaterial(material); err != nil {
			t.Fatal(err)
		}
		otherPurpose := Login
		if purpose == Login {
			otherPurpose = Reauthenticate
		}
		for _, candidate := range []struct {
			origin   string
			purpose  AuthPurpose
			accepted bool
		}{
			{"https://INTENDED.example:443/another-prefix", purpose, true},
			{"https://fake.example", purpose, false},
			{"http://intended.example", purpose, false},
			{"https://intended.example:444", purpose, false},
			{"https://intended.example", otherPurpose, false},
		} {
			clientConfig, err := NewAuthConfig(candidate.origin, candidate.purpose)
			if err != nil {
				t.Fatal(err)
			}
			client, err := clientConfig.Client()
			if err != nil {
				t.Fatal(err)
			}
			ke1, err := client.GenerateKE1(password)
			if err != nil {
				t.Fatal(err)
			}
			ke2, output, err := server.GenerateKE2(ke1, &opaque.ClientRecord{RegistrationRecord: record, CredentialIdentifier: account, ClientIdentity: account})
			if err != nil {
				t.Fatal(err)
			}
			ke3, secret, _, err := client.GenerateKE3(ke2, account, ServerIdentity())
			if candidate.accepted {
				if err != nil {
					t.Fatal(err)
				}
				if err := server.LoginFinish(ke3, output.ClientMAC); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(secret, output.SessionSecret) {
					t.Fatal("session keys differ")
				}
			} else if err == nil {
				t.Fatalf("relay authenticated %s with purpose %s", candidate.origin, candidate.purpose)
			}
			clear(secret)
			clear(output.SessionSecret)
			client.ClearState()
		}
	}
}
