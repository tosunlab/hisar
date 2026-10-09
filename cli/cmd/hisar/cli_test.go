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
	flags := flag.NewFlagSet("get", flag.ContinueOnError)
	vaultName := flags.String("v", "Personal", "")
	fieldName := flags.String("f", "", "")
	allPositional, err := parse(flags, []string{"netflix", "-f", "password", "-v", "Family"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(allPositional, []string{"netflix"}) || *vaultName != "Family" || *fieldName != "password" {
		t.Errorf("pos=%v v=%q f=%q", allPositional, *vaultName, *fieldName)
	}
}

func TestParseInvite(t *testing.T) {
	server, token, err := parseInvite(" https://tryhisar.com/invite/abc_DEF-123 ")
	if err != nil || server != "https://tryhisar.com" || token != "abc_DEF-123" {
		t.Errorf("got %q %q %v", server, token, err)
	}
	allBad := []string{"", "tryhisar.com/invite/x", "https://tryhisar.com/invite/", "https://tryhisar.com/other/x", "ftp://h/invite/x", "https://h/invite/a/b"}
	for _, badLink := range allBad {
		if _, _, err := parseInvite(badLink); err == nil {
			t.Errorf("%q accepted", badLink)
		}
	}
}

func TestFindItem(t *testing.T) {
	allItem := []item.Item{
		{ID: "1", Overview: item.Overview{Title: "GitHub"}},
		{ID: "2", Overview: item.Overview{Title: "GitHub Enterprise"}},
		{ID: "3", Overview: item.Overview{Title: "Netflix"}},
	}
	allCase := map[string]string{"github": "1", "3": "3", "net": "3", "enterprise": "2"}
	for query, want := range allCase {
		vaultItem, err := findItem(allItem, query)
		if err != nil || vaultItem.ID != want {
			t.Errorf("%q: %v %v", query, vaultItem, err)
		}
	}
	if _, err := findItem(allItem, "git"); err == nil || !strings.Contains(err.Error(), "matches 2 items") {
		t.Errorf("ambiguous: %v", err)
	}
	if _, err := findItem(allItem, "bank"); err == nil {
		t.Error("missing item found")
	}
}

func TestGetFieldValue(t *testing.T) {
	vaultItem := &item.Item{
		Overview: item.Overview{Title: "T", Username: "u", URLs: []string{"https://a", "https://b"}},
		Details:  item.Details{Password: "p", Notes: "n", Fields: []item.Field{{Name: "PIN", Value: "1234"}}},
	}
	allCase := map[string]string{"title": "T", "Username": "u", "url": "https://a", "password": "p", "notes": "n", "pin": "1234"}
	for name, want := range allCase {
		if got, ok := getFieldValue(vaultItem, name); !ok || got != want {
			t.Errorf("%s = %q, %v", name, got, ok)
		}
	}
	if _, ok := getFieldValue(vaultItem, "missing"); ok {
		t.Error("missing field found")
	}
	if _, ok := getFieldValue(&item.Item{}, "url"); ok {
		t.Error("url found on item without URLs")
	}
}

func TestLoadStateUsesEnvironmentPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	t.Setenv("HISAR_STATE", path)

	currentState, err := loadState()
	if err != nil {
		t.Fatal(err)
	}
	if err := requireLogin(currentState); err == nil || !strings.Contains(err.Error(), "hisar login") {
		t.Errorf("requireLogin on empty state: %v", err)
	}
	currentState.AccountID, currentState.Token = "ABC123", "tok"
	if err := currentState.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("state not written to HISAR_STATE: %v", err)
	}
	reloadedState, err := loadState()
	if err != nil || requireLogin(reloadedState) != nil {
		t.Errorf("reloaded state: %+v %v", reloadedState, err)
	}
}

func TestLoadSecretKeyExplainsRecovery(t *testing.T) {
	secretKey, _ := crypt.GenerateSecretKey("ABC123")
	t.Setenv("HISAR_SECRET_KEY", secretKey.Encode())
	if _, err := loadSecretKey("XYZ789"); err == nil || !strings.Contains(err.Error(), "HISAR_SECRET_KEY") {
		t.Errorf("error should explain how to recover: %v", err)
	}
}
