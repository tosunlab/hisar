package cache

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/tryhisar/hisar/core/proto"
)

func openTestCache(t *testing.T) (*Cache, string) {
	path := filepath.Join(t.TempDir(), "cache.db")
	testCache, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testCache.Close() })
	return testCache, path
}

func TestOpenCreatesPrivateFile(t *testing.T) {
	_, path := openTestCache(t)
	fileInfo, err := os.Stat(path)
	if err != nil || fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("mode: %v %v", fileInfo, err)
	}
}

func TestAccountRoundTrip(t *testing.T) {
	testCache, _ := openTestCache(t)
	if account, err := testCache.LoadAccount("ABC123"); err != nil || account != nil {
		t.Fatalf("empty cache: %v %v", account, err)
	}
	account := &proto.Account{AccountID: "ABC123", Email: "alice@example.com", Salt: []byte{1, 2}, EncKeyset: []byte{3, 4}}
	if err := testCache.SaveAccount(account); err != nil {
		t.Fatal(err)
	}
	account.Email = "alice@tryhisar.com"
	if err := testCache.SaveAccount(account); err != nil {
		t.Fatal(err)
	}
	loadedAccount, err := testCache.LoadAccount("ABC123")
	if err != nil || loadedAccount.Email != "alice@tryhisar.com" || string(loadedAccount.EncKeyset) != "\x03\x04" {
		t.Errorf("loaded: %+v %v", loadedAccount, err)
	}
	if otherAccount, _ := testCache.LoadAccount("XYZ789"); otherAccount != nil {
		t.Error("another account ID returned a cached account")
	}
}

func TestApplyAllChangeAndSeq(t *testing.T) {
	testCache, _ := openTestCache(t)
	if seq, err := testCache.GetSeq("vault-1"); err != nil || seq != 0 {
		t.Fatalf("seq of unknown vault: %d %v", seq, err)
	}
	if err := testCache.ApplyAllChange("vault-1", []proto.Item{
		{ID: "item-a", Revision: "r1", Seq: 1, Overview: []byte("a1")},
		{ID: "item-b", Revision: "r1", Seq: 2, Overview: []byte("b1")},
	}, 2); err != nil {
		t.Fatal(err)
	}
	if err := testCache.ApplyAllChange("vault-1", []proto.Item{
		{ID: "item-a", Revision: "r2", Seq: 3, Overview: []byte("a2")},
		{ID: "item-b", Seq: 4, Deleted: true},
		{ID: "item-c", Seq: 5, Deleted: true},
	}, 5); err != nil {
		t.Fatal(err)
	}
	allItem, err := testCache.ListAllItem("vault-1")
	if err != nil || len(allItem) != 1 || allItem[0].Revision != "r2" || string(allItem[0].Overview) != "a2" {
		t.Fatalf("items: %+v %v", allItem, err)
	}
	if seq, _ := testCache.GetSeq("vault-1"); seq != 5 {
		t.Errorf("seq = %d", seq)
	}
	if allOtherItem, _ := testCache.ListAllItem("vault-2"); len(allOtherItem) != 0 {
		t.Errorf("items leaked into another vault: %v", allOtherItem)
	}
}

func TestReplaceAllVaultForgetsRemovedVaults(t *testing.T) {
	testCache, _ := openTestCache(t)
	if err := testCache.ApplyAllChange("vault-new", []proto.Item{{ID: "item-x"}}, 1); err != nil {
		t.Fatal(err)
	}
	if err := testCache.ReplaceAllVault([]proto.Vault{{ID: "vault-1", Role: "owner"}, {ID: "vault-2", Role: "reader"}}); err != nil {
		t.Fatal(err)
	}
	if err := testCache.ApplyAllChange("vault-2", []proto.Item{{ID: "item-y"}}, 7); err != nil {
		t.Fatal(err)
	}
	if allItem, _ := testCache.ListAllItem("vault-new"); len(allItem) != 0 {
		t.Errorf("items of an unlisted vault kept: %v", allItem)
	}
	if err := testCache.ReplaceAllVault([]proto.Vault{{ID: "vault-1", Role: "editor"}}); err != nil {
		t.Fatal(err)
	}
	allVault, err := testCache.ListAllVault()
	if err != nil || len(allVault) != 1 || allVault[0].ID != "vault-1" || allVault[0].Role != "editor" {
		t.Fatalf("vaults: %+v %v", allVault, err)
	}
	if allItem, _ := testCache.ListAllItem("vault-2"); len(allItem) != 0 {
		t.Errorf("items of a removed vault kept: %v", allItem)
	}
	if seq, _ := testCache.GetSeq("vault-2"); seq != 0 {
		t.Errorf("seq of a removed vault kept: %d", seq)
	}
}

func TestResetVault(t *testing.T) {
	testCache, _ := openTestCache(t)
	if err := testCache.ApplyAllChange("vault-1", []proto.Item{{ID: "item-a"}}, 9); err != nil {
		t.Fatal(err)
	}
	if err := testCache.ResetVault("vault-1"); err != nil {
		t.Fatal(err)
	}
	allItem, _ := testCache.ListAllItem("vault-1")
	seq, _ := testCache.GetSeq("vault-1")
	if len(allItem) != 0 || seq != 0 {
		t.Errorf("after reset: %d items, seq %d", len(allItem), seq)
	}
}

func TestReopenKeepsDataAndRemoveDeletesIt(t *testing.T) {
	testCache, path := openTestCache(t)
	if err := testCache.ApplyAllChange("vault-1", []proto.Item{{ID: "item-a"}}, 3); err != nil {
		t.Fatal(err)
	}
	testCache.Close()
	reopenedCache, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if seq, _ := reopenedCache.GetSeq("vault-1"); seq != 3 {
		t.Errorf("seq after reopen = %d", seq)
	}
	reopenedCache.Close()
	if err := Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("cache file still exists: %v", err)
	}
	if err := Remove(path); err != nil {
		t.Errorf("removing a missing cache: %v", err)
	}
}

func TestOpenEmptiesOtherSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	database, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{`CREATE TABLE items (old TEXT)`, `INSERT INTO items VALUES ('x')`, `PRAGMA user_version = 99`} {
		if _, err := database.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	database.Close()
	testCache, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer testCache.Close()
	if allItem, err := testCache.ListAllItem("vault-1"); err != nil || len(allItem) != 0 {
		t.Errorf("items after schema change: %v %v", allItem, err)
	}
}
