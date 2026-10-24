package importer

import (
	"testing"

	"github.com/tryhisar/hisar/core/item"
)

func TestDuplicatePasskeyContentIsPresentNotJustItsID(t *testing.T) {
	first := item.Passkey{CredentialID: []byte{1}, PrivateKey: []byte{2}, PRFSecret: []byte{3}, RelyingPartyID: "example.com"}
	other := item.Passkey{CredentialID: []byte{4}, PrivateKey: []byte{5}, RelyingPartyID: "other.example.com"}
	for _, test := range []struct {
		name   string
		stored []item.Passkey
		want   int
	}{
		{"stored superset", []item.Passkey{other, first}, 0},
		{"missing", []item.Passkey{other}, 1},
		{"changed key", []item.Passkey{{CredentialID: first.CredentialID, PrivateKey: []byte{9}, PRFSecret: first.PRFSecret, RelyingPartyID: first.RelyingPartyID}}, 1},
		{"changed PRF", []item.Passkey{{CredentialID: first.CredentialID, PrivateKey: first.PrivateKey, PRFSecret: []byte{9}, RelyingPartyID: first.RelyingPartyID}}, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			imported := item.Item{Overview: item.Overview{Title: "Login"}, Details: item.Details{Passkeys: []item.Passkey{first}}}
			stored := item.Item{Overview: item.Overview{Title: "Login"}, Details: item.Details{Passkeys: test.stored}}
			item.UpdatePasskeyOverview(&imported)
			item.UpdatePasskeyOverview(&stored)
			skipped := FindUnsavedDuplicates([]item.Item{imported}, []item.Item{stored}, map[int]int{0: 0})
			if len(skipped) != test.want {
				t.Fatalf("unsaved duplicates = %+v, want %d", skipped, test.want)
			}
		})
	}
}
