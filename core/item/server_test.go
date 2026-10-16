package item

import (
	"reflect"
	"testing"
)

func TestReadAndWriteServer(t *testing.T) {
	vaultItem := Item{
		Overview: Overview{Category: CategoryServer, Username: "deploy", URLs: []string{"https://console.example.com"}},
		Details: Details{Password: "server-password", Fields: []Field{
			{Name: "Provider", Value: "Example Cloud"},
			{Name: "ssh KEY", Value: " key-item-1 "},
			{Name: "host", Value: " ssh://build.example.com:2222 "},
		}},
	}
	server, otherFields := ReadServer(&vaultItem)
	if server != (Server{Host: " ssh://build.example.com:2222 ", SSHKeyID: " key-item-1 "}) || !reflect.DeepEqual(otherFields, []Field{{Name: "Provider", Value: "Example Cloud"}}) {
		t.Errorf("server = %+v, other fields %+v", server, otherFields)
	}
	NormalizeServer(&vaultItem)
	wantFields := []Field{
		{Name: FieldHost, Value: "build.example.com"},
		{Name: FieldPort, Value: "2222"},
		{Name: FieldSSHKey, Value: "key-item-1"},
		{Name: "Provider", Value: "Example Cloud"},
	}
	if !reflect.DeepEqual(vaultItem.Details.Fields, wantFields) {
		t.Errorf("normalized fields =\n%+v\nwant\n%+v", vaultItem.Details.Fields, wantFields)
	}
	if vaultItem.Overview.Username != "deploy" || vaultItem.Details.Password != "server-password" || len(vaultItem.Overview.URLs) != 1 {
		t.Errorf("normalizing changed the login data: %+v %+v", vaultItem.Overview, vaultItem.Details)
	}
	if len(ListServerFields()) != 3 {
		t.Errorf("server fields = %d", len(ListServerFields()))
	}
	login := Item{Details: Details{Fields: []Field{{Name: "Host", Value: "db:5432"}}}}
	NormalizeServer(&login)
	if login.Details.Fields[0].Value != "db:5432" {
		t.Errorf("NormalizeServer changed a login: %+v", login.Details.Fields)
	}
}

func TestNormalizeServerSplitsAHostTypedWithItsPort(t *testing.T) {
	for _, test := range []struct{ host, port, wantHost, wantPort string }{
		{"ssh://build.example.com:2222", "", "build.example.com", "2222"},
		{"https://console.example.com/", "", "console.example.com", ""},
		{"db.example.com:5432", "", "db.example.com", "5432"},
		{"[2001:db8::1]:22", "", "2001:db8::1", "22"},
		{"2001:db8::1", "", "2001:db8::1", ""},
		{"db.example.com:5432", "6543", "db.example.com:5432", "6543"},
		{"ssh://build.example.com:2222", "22", "build.example.com", "22"},
		{"https://example.com/admin", "", "https://example.com/admin", ""},
		{" 10.0.0.5 ", " 22 ", "10.0.0.5", "22"},
	} {
		got := Server{Host: test.host, Port: test.port}.Normalize()
		if got.Host != test.wantHost || got.Port != test.wantPort {
			t.Errorf("Normalize(%q, %q) = %q, %q; want %q, %q", test.host, test.port, got.Host, got.Port, test.wantHost, test.wantPort)
		}
	}
}

func TestServerPortWarningAndSummary(t *testing.T) {
	for port, want := range map[string]string{"": "", "22": "", "65535": "", "0": "The port is not a number from 1 to 65535", "70000": "The port is not a number from 1 to 65535", "ssh": "The port is not a number from 1 to 65535"} {
		if got := (Server{Port: port}).PortWarning(); got != want {
			t.Errorf("PortWarning(%q) = %q, want %q", port, got, want)
		}
	}
	for _, test := range []struct {
		server Server
		want   string
	}{
		{Server{Host: "build.example.com", Port: "2222"}, "build.example.com:2222"},
		{Server{Host: "2001:db8::1", Port: "22"}, "[2001:db8::1]:22"},
		{Server{Host: "build.example.com"}, "build.example.com"},
		{Server{Port: "22"}, ""},
	} {
		if got := test.server.Summary(); got != test.want {
			t.Errorf("Summary(%+v) = %q, want %q", test.server, got, test.want)
		}
	}
}

func TestOnlyLoginsAndServersOfferLoginFill(t *testing.T) {
	for category, want := range map[string]bool{"": true, CategoryLogin: true, CategoryServer: true, CategorySecureNote: false, CategoryAPICredential: false, CategorySSHKey: false} {
		vaultItem := Item{Overview: Overview{Category: category}}
		if got := vaultItem.OffersLoginFill(); got != want {
			t.Errorf("OffersLoginFill for %q = %v, want %v", category, got, want)
		}
	}
	if server := (Item{Overview: Overview{Category: CategoryServer}}); server.OmitsLoginData() {
		t.Error("a server omits login data")
	}
}
