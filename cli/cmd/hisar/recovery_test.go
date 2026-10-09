package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/proto"
)

func TestParseRecoveryLink(t *testing.T) {
	server, token, err := parseRecoveryLink(" http://127.0.0.1:8080/recover/abc_DEF-123\n")
	if err != nil || server != "http://127.0.0.1:8080" || token != "abc_DEF-123" {
		t.Errorf("got %q %q %v", server, token, err)
	}
	allBad := []string{"", "tryhisar.com/recover/x", "https://tryhisar.com/recover/", "https://tryhisar.com/invite/x", "ftp://h/recover/x", "https://h/recover/a/b"}
	for _, badLink := range allBad {
		if _, _, err := parseRecoveryLink(badLink); err == nil || err.Error() != "invalid recovery link" {
			t.Errorf("%q: %v", badLink, err)
		}
	}
	if _, _, err := parseInvite("https://tryhisar.com/recover/x"); err == nil {
		t.Error("recovery link accepted as an invite")
	}
}

func TestCheckRecoveryPin(t *testing.T) {
	currentState, err := localstate.Load(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := checkRecoveryPin(currentState, []byte{1, 2}); err != nil {
		t.Fatalf("first use: %v", err)
	}
	if err := checkRecoveryPin(currentState, []byte{1, 2}); err != nil {
		t.Errorf("same key again: %v", err)
	}
	if err := checkRecoveryPin(currentState, []byte{3}); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Errorf("changed recovery key accepted: %v", err)
	}
	if string(currentState.AllPin["recovery"]) != string([]byte{1, 2}) {
		t.Errorf("pin replaced: %v", currentState.AllPin)
	}
}

func TestFindPendingRecoveryByEmail(t *testing.T) {
	allPendingRecovery := []proto.PendingRecovery{
		{AccountID: "AAAAAA", Email: "alice@example.com"},
		{AccountID: "BBBBBB", Email: "bob@example.com", AllVault: []proto.RecoveryVault{{WrappedKey: []byte{1}}, {}, {WrappedKey: []byte{2}}}},
	}
	pending, err := findPendingRecoveryByEmail(allPendingRecovery, " Bob@Example.com ")
	if err != nil || pending.AccountID != "BBBBBB" {
		t.Fatalf("%+v %v", pending, err)
	}
	if recoverableCount := countRecoverableVault(pending); recoverableCount != 2 {
		t.Errorf("recoverable vaults = %d", recoverableCount)
	}
	if _, err := findPendingRecoveryByEmail(allPendingRecovery, "carol@example.com"); err == nil || !strings.Contains(err.Error(), "recovery link") {
		t.Errorf("missing recovery: %v", err)
	}
}
