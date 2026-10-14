package main

import (
	"bufio"
	"strings"
	"testing"
)

func TestFindInvitationID(t *testing.T) {
	invitationIDs := []string{"a1b2c3d4-1111", "a1b2ffff-2222", "e5f6a7b8-3333"}
	for query, expected := range map[string]string{"e5": "e5f6a7b8-3333", "a1b2c3": "a1b2c3d4-1111", " A1B2F ": "a1b2ffff-2222", "e5f6a7b8-3333": "e5f6a7b8-3333"} {
		if invitationID, err := findInvitationID(invitationIDs, query); err != nil || invitationID != expected {
			t.Errorf("%q: %q %v", query, invitationID, err)
		}
	}
	if _, err := findInvitationID(invitationIDs, "a1b2"); err == nil || !strings.Contains(err.Error(), "more than one") {
		t.Errorf("ambiguous: %v", err)
	}
	if _, err := findInvitationID(invitationIDs, "zz"); err == nil || !strings.Contains(err.Error(), "no invitation") {
		t.Errorf("unknown: %v", err)
	}
	if _, err := findInvitationID(nil, "a1"); err == nil || !strings.Contains(err.Error(), "no invitations") {
		t.Errorf("none: %v", err)
	}
	originalReader := stdinReader
	defer func() { stdinReader = originalReader }()
	stdinReader = bufio.NewReader(strings.NewReader("\n"))
	if invitationID, err := findInvitationID([]string{"e5f6a7b8-3333"}, ""); err != nil || invitationID != "e5f6a7b8-3333" {
		t.Errorf("default for the only invitation: %q %v", invitationID, err)
	}
}

func TestParseInvitationArgument(t *testing.T) {
	if action, query, err := parseInvitationArgument(nil); action != nil || query != "" || err != nil {
		t.Errorf("list: %v %q %v", action, query, err)
	}
	if action, query, err := parseInvitationArgument([]string{"cancel", "e5f6"}); err != nil || action.name != "cancel" || !action.isSent || query != "e5f6" {
		t.Errorf("cancel: %+v %q %v", action, query, err)
	}
	if action, _, err := parseInvitationArgument([]string{"accept"}); err != nil || action.name != "accept" || action.isSent {
		t.Errorf("accept: %+v %v", action, err)
	}
	if _, _, err := parseInvitationArgument([]string{"accept", "a", "b"}); err != errUsage {
		t.Errorf("too many: %v", err)
	}
}

func TestGetShareArgumentPromptsForMissingFields(t *testing.T) {
	originalReader := stdinReader
	defer func() { stdinReader = originalReader }()
	stdinReader = bufio.NewReader(strings.NewReader("Family\n Bob@Example.com \n"))
	vaultName, email, err := getShareArgument(nil, "reader")
	if err != nil || vaultName != "Family" || email != "bob@example.com" {
		t.Errorf("prompted: %q %q %v", vaultName, email, err)
	}
	if _, _, err := getShareArgument([]string{"Family", "bob@example.com"}, "admin"); err == nil {
		t.Error("invalid role accepted")
	}
	if _, _, err := getShareArgument([]string{"Family", "bob@example.com", "extra"}, "reader"); err != errUsage {
		t.Errorf("extra argument: %v", err)
	}
}
