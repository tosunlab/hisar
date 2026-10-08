package main

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
)

func TestParseInterspersedFlags(t *testing.T) {
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	v := fs.String("v", "Personal", "")
	f := fs.String("f", "", "")
	pos, err := parse(fs, []string{"netflix", "-f", "password", "-v", "Family"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pos, []string{"netflix"}) || *v != "Family" || *f != "password" {
		t.Errorf("pos=%v v=%q f=%q", pos, *v, *f)
	}
}

func TestParseInvite(t *testing.T) {
	server, token, err := parseInvite(" https://tryhisar.com/invite/abc_DEF-123 ")
	if err != nil || server != "https://tryhisar.com" || token != "abc_DEF-123" {
		t.Errorf("got %q %q %v", server, token, err)
	}
	for _, bad := range []string{"", "tryhisar.com/invite/x", "https://tryhisar.com/invite/", "https://tryhisar.com/other/x", "ftp://h/invite/x", "https://h/invite/a/b"} {
		if _, _, err := parseInvite(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestFindItem(t *testing.T) {
	items := []item.Item{
		{ID: "1", Overview: item.Overview{Title: "GitHub"}},
		{ID: "2", Overview: item.Overview{Title: "GitHub Enterprise"}},
		{ID: "3", Overview: item.Overview{Title: "Netflix"}},
	}
	cases := map[string]string{"github": "1", "3": "3", "net": "3", "enterprise": "2"}
	for q, want := range cases {
		it, err := findItem(items, q)
		if err != nil || it.ID != want {
			t.Errorf("%q: %v %v", q, it, err)
		}
	}
	if _, err := findItem(items, "git"); err == nil || !strings.Contains(err.Error(), "matches 2 items") {
		t.Errorf("ambiguous: %v", err)
	}
	if _, err := findItem(items, "bank"); err == nil {
		t.Error("missing item found")
	}
}

func TestFieldValue(t *testing.T) {
	it := &item.Item{
		Overview: item.Overview{Title: "T", Username: "u", URLs: []string{"https://a", "https://b"}},
		Details:  item.Details{Password: "p", Notes: "n", Fields: []item.Field{{Name: "PIN", Value: "1234"}}},
	}
	for name, want := range map[string]string{"title": "T", "Username": "u", "url": "https://a", "password": "p", "notes": "n", "pin": "1234"} {
		if got, ok := fieldValue(it, name); !ok || got != want {
			t.Errorf("%s = %q, %v", name, got, ok)
		}
	}
	if _, ok := fieldValue(it, "missing"); ok {
		t.Error("missing field found")
	}
	if _, ok := fieldValue(&item.Item{}, "url"); ok {
		t.Error("url found on item without URLs")
	}
}

func TestStateRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "state.json")
	t.Setenv("HISAR_STATE", path)

	st, err := loadState()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.requireLogin(); err == nil {
		t.Error("empty state counts as logged in")
	}
	st.Server, st.AccountID, st.Token = "https://x", "ABC123", "tok"
	st.SetSeq("v1", 42)
	st.Pins["B"] = []byte{1, 2}
	if err := st.save(); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(path)
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("state mode = %o", fi.Mode().Perm())
	}
	again, err := loadState()
	if err != nil {
		t.Fatal(err)
	}
	if again.Token != "tok" || again.Seq("v1") != 42 || again.Pins["B"][1] != 2 || again.requireLogin() != nil {
		t.Errorf("reloaded state: %+v", again)
	}
}

func TestSecretKeyFromEnv(t *testing.T) {
	sk, _ := crypt.NewSecretKey("ABC123")
	t.Setenv("HISAR_SECRET_KEY", strings.ToLower(sk.Encode()))
	got, err := loadSecretKey("ABC123")
	if err != nil || got != sk {
		t.Fatalf("loadSecretKey: %v", err)
	}
	if _, err := loadSecretKey("XYZ789"); err == nil {
		t.Error("secret key for other account accepted")
	}
	if err := storeSecretKey(sk); err != nil {
		t.Errorf("store with env override should be a no-op: %v", err)
	}
}
