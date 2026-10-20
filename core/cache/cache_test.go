package cache

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/tryhisar/hisar/core/apitypes"
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

func TestOpenMakesAnExistingFilePrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	testCache, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	testCache.Close()
	if fileInfo, err := os.Stat(path); err != nil || fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("mode: %v %v", fileInfo, err)
	}
}

func TestAccountRoundTrip(t *testing.T) {
	testCache, _ := openTestCache(t)
	if account, err := testCache.LoadAccount("ABC123"); err != nil || account != nil {
		t.Fatalf("empty cache: %v %v", account, err)
	}
	account := &apitypes.Account{AccountID: "ABC123", Email: "alice@example.com", Salt: []byte{1, 2}, EncKeyset: []byte{3, 4}}
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

func TestAccountPreferencesRoundTrip(t *testing.T) {
	testCache, _ := openTestCache(t)
	if preferences, err := testCache.LoadAccountPreferences("ABC123"); err != nil || preferences != nil {
		t.Fatalf("empty cache: %v %v", preferences, err)
	}
	for revision := int64(1); revision <= 2; revision++ {
		if err := testCache.SaveAccountPreferences("ABC123", &apitypes.AccountPreferences{Revision: revision, Ciphertext: []byte{byte(revision)}}); err != nil {
			t.Fatal(err)
		}
	}
	preferences, err := testCache.LoadAccountPreferences("ABC123")
	if err != nil || preferences.Revision != 2 || string(preferences.Ciphertext) != "\x02" {
		t.Errorf("loaded: %+v %v", preferences, err)
	}
	if other, _ := testCache.LoadAccountPreferences("XYZ789"); other != nil {
		t.Error("another account ID returned cached preferences")
	}
}

func TestApplyAllChangeAndSeq(t *testing.T) {
	testCache, _ := openTestCache(t)
	if seq, err := testCache.GetSeq("vault-1"); err != nil || seq != 0 {
		t.Fatalf("seq of unknown vault: %d %v", seq, err)
	}
	if err := testCache.ApplyChanges("vault-1", []apitypes.Item{
		{ID: "item-a", Revision: "r1", Seq: 1, Overview: []byte("a1")},
		{ID: "item-b", Revision: "r1", Seq: 2, Overview: []byte("b1")},
	}, 2); err != nil {
		t.Fatal(err)
	}
	if err := testCache.ApplyChanges("vault-1", []apitypes.Item{
		{ID: "item-a", Revision: "r2", Seq: 3, Overview: []byte("a2")},
		{ID: "item-b", Seq: 4, Deleted: true},
		{ID: "item-c", Seq: 5, Deleted: true},
	}, 5); err != nil {
		t.Fatal(err)
	}
	items, err := testCache.ListItems("vault-1")
	if err != nil || len(items) != 1 || items[0].Revision != "r2" || string(items[0].Overview) != "a2" {
		t.Fatalf("items: %+v %v", items, err)
	}
	if seq, _ := testCache.GetSeq("vault-1"); seq != 5 {
		t.Errorf("seq = %d", seq)
	}
	if otherItems, _ := testCache.ListItems("vault-2"); len(otherItems) != 0 {
		t.Errorf("items leaked into another vault: %v", otherItems)
	}
}

func TestReplaceAllVaultForgetsRemovedVaults(t *testing.T) {
	testCache, _ := openTestCache(t)
	if err := testCache.ApplyChanges("vault-new", []apitypes.Item{{ID: "item-x"}}, 1); err != nil {
		t.Fatal(err)
	}
	if err := testCache.ReplaceVaults([]apitypes.Vault{{ID: "vault-1", Role: "owner"}, {ID: "vault-2", Role: "reader"}}); err != nil {
		t.Fatal(err)
	}
	if err := testCache.ApplyChanges("vault-2", []apitypes.Item{{ID: "item-y"}}, 7); err != nil {
		t.Fatal(err)
	}
	if items, _ := testCache.ListItems("vault-new"); len(items) != 0 {
		t.Errorf("items of an unlisted vault kept: %v", items)
	}
	if err := testCache.ReplaceVaults([]apitypes.Vault{{ID: "vault-1", Role: "editor"}}); err != nil {
		t.Fatal(err)
	}
	vaults, err := testCache.ListVaults()
	if err != nil || len(vaults) != 1 || vaults[0].ID != "vault-1" || vaults[0].Role != "editor" {
		t.Fatalf("vaults: %+v %v", vaults, err)
	}
	if items, _ := testCache.ListItems("vault-2"); len(items) != 0 {
		t.Errorf("items of a removed vault kept: %v", items)
	}
	if seq, _ := testCache.GetSeq("vault-2"); seq != 0 {
		t.Errorf("seq of a removed vault kept: %d", seq)
	}
}

func TestResetVault(t *testing.T) {
	testCache, _ := openTestCache(t)
	if err := testCache.ApplyChanges("vault-1", []apitypes.Item{{ID: "item-a"}}, 9); err != nil {
		t.Fatal(err)
	}
	if err := testCache.ResetVault("vault-1"); err != nil {
		t.Fatal(err)
	}
	items, _ := testCache.ListItems("vault-1")
	seq, _ := testCache.GetSeq("vault-1")
	if len(items) != 0 || seq != 0 {
		t.Errorf("after reset: %d items, seq %d", len(items), seq)
	}
}

func TestReopenKeepsDataAndRemoveDeletesIt(t *testing.T) {
	testCache, path := openTestCache(t)
	if err := testCache.ApplyChanges("vault-1", []apitypes.Item{{ID: "item-a"}}, 3); err != nil {
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
	if items, err := testCache.ListItems("vault-1"); err != nil || len(items) != 0 {
		t.Errorf("items after schema change: %v %v", items, err)
	}
}

func TestWebsiteIconRoundTrip(t *testing.T) {
	testCache, _ := openTestCache(t)
	if data, err := testCache.LoadWebsiteIcon("lookup-1"); err != nil || data != nil {
		t.Fatalf("empty cache: %v %v", data, err)
	}
	for _, sealed := range [][]byte{{1, 2}, {3}} {
		if err := testCache.SaveWebsiteIcon("lookup-1", sealed); err != nil {
			t.Fatal(err)
		}
	}
	if data, err := testCache.LoadWebsiteIcon("lookup-1"); err != nil || len(data) != 1 || data[0] != 3 {
		t.Fatalf("loaded %v, %v", data, err)
	}
	if err := testCache.ClearWebsiteIcons(); err != nil {
		t.Fatal(err)
	}
	if data, _ := testCache.LoadWebsiteIcon("lookup-1"); data != nil {
		t.Errorf("after clearing: %v", data)
	}
}
