package crypt

import "testing"

func TestImportScopeBindsDestinationAndMemberRoles(t *testing.T) {
	members := []ImportMember{{AccountID: "bob", Role: "reader"}, {AccountID: "alice", Role: "owner"}}
	scope := ImportScope("vault-a", members)
	if scope != "e57ec3e7c306e3397456b2bbee848d205d6d15ac095f141bfb3c4c35756e54b1" {
		t.Fatalf("scope format changed: %s", scope)
	}
	if members[0].AccountID != "bob" {
		t.Fatal("scope calculation mutated caller order")
	}
	if scope != ImportScope("vault-a", []ImportMember{members[1], members[0]}) {
		t.Fatal("member list order changed the scope")
	}
	if scope == ImportScope("vault-b", members) {
		t.Fatal("scope did not bind destination")
	}
	members[0].Role = "editor"
	if scope == ImportScope("vault-a", members) {
		t.Fatal("scope did not bind member role")
	}
	members[0].Role = "reader"
	members = append(members, ImportMember{AccountID: "carol", Role: "reader"})
	if scope == ImportScope("vault-a", members) {
		t.Fatal("scope did not bind membership")
	}
}
