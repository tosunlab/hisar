package item

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPasskeyJSON(t *testing.T) {
	passkey := Passkey{
		CredentialID:    []byte{1, 2, 3},
		RelyingPartyID:  "example.com",
		UserHandle:      []byte{9, 8},
		UserName:        "alex@example.com",
		UserDisplayName: "Alex",
		PrivateKey:      []byte{0x30, 0x81},
		CreatedAt:       time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC),
	}
	encoded, err := json.Marshal(Details{Passkeys: []Passkey{passkey}})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{`"credentialId":"AQID"`, `"relyingPartyId":"example.com"`, `"userHandle":"CQg="`, `"privateKey":"MIE="`, `"createdAt":"2026-10-09T12:00:00Z"`} {
		if !strings.Contains(string(encoded), name) {
			t.Errorf("%s missing from %s", name, encoded)
		}
	}
	var decoded Details
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded.Passkeys, []Passkey{passkey}) {
		t.Errorf("decoded = %+v", decoded.Passkeys)
	}
	empty, _ := json.Marshal(Details{})
	overview, _ := json.Marshal(Overview{Title: "T"})
	if strings.Contains(string(empty)+string(overview), "passkey") {
		t.Errorf("empty passkeys serialized: %s %s", empty, overview)
	}
}

func TestUpdatePasskeyOverview(t *testing.T) {
	vaultItem := Item{Details: Details{Passkeys: []Passkey{{RelyingPartyID: "example.com"}, {RelyingPartyID: "login.example.org"}, {RelyingPartyID: "example.com"}}}}
	UpdatePasskeyOverview(&vaultItem)
	if !reflect.DeepEqual(vaultItem.Overview.PasskeyRelyingPartyIDs, []string{"example.com", "login.example.org"}) {
		t.Errorf("relying party IDs = %v", vaultItem.Overview.PasskeyRelyingPartyIDs)
	}
	vaultItem.Details.Passkeys = nil
	UpdatePasskeyOverview(&vaultItem)
	if vaultItem.Overview.PasskeyRelyingPartyIDs != nil {
		t.Errorf("relying party IDs after removing passkeys = %v", vaultItem.Overview.PasskeyRelyingPartyIDs)
	}
}
