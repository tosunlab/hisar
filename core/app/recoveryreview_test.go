package app

import (
	"encoding/json"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"testing"
)

func injectJudgeBadItem(t *testing.T, f unlockedTestService) {
	t.Helper()
	f.apiServer.mutex.Lock()
	defer f.apiServer.mutex.Unlock()
	f.apiServer.seq++
	f.apiServer.itemsByVaultID[f.personalID]["judge-bad"] = apitypes.Item{ID: "judge-bad", Revision: "reviewed-r1", Seq: f.apiServer.seq, Overview: []byte("bad"), Details: []byte("bad")}
}

func TestJudgeHealthyItemsAndIssueRemainVisible(t *testing.T) {
	f := createUnlockedTestService(t)
	healthy, err := f.service.SaveItem(f.personalID, ItemDetail{Title: "Judge healthy", Password: "test-only"})
	if err != nil {
		t.Fatal(err)
	}
	injectJudgeBadItem(t, f)
	items, err := f.service.ListItems(f.personalID)
	if err != nil {
		t.Fatalf("one corrupt record disabled healthy UI records: %v", err)
	}
	encoded, _ := json.Marshal(items)
	var records []map[string]any
	json.Unmarshal(encoded, &records)
	foundHealthy, foundIssue := false, false
	for _, r := range records {
		if r["id"] == healthy.ID {
			foundHealthy = true
		}
		if r["id"] == "judge-bad" && r["readIssue"] != nil {
			foundIssue = true
		}
	}
	if !foundHealthy || !foundIssue {
		t.Fatalf("healthy=%v explicitIssue=%v: %s", foundHealthy, foundIssue, encoded)
	}
}

func TestJudgeUnreadableItemDeleteTupleAndConcurrentRevision(t *testing.T) {
	f := createUnlockedTestService(t)
	healthy, err := f.service.SaveItem(f.personalID, ItemDetail{Title: "Keep me"})
	if err != nil {
		t.Fatal(err)
	}
	injectJudgeBadItem(t, f)
	if _, err = f.service.ListItems(f.personalID); err != nil {
		t.Fatal(err)
	}
	for _, tuple := range [][3]string{{f.familyID, "judge-bad", "reviewed-r1"}, {f.personalID, healthy.ID, "reviewed-r1"}, {f.personalID, "judge-bad", "wrong"}} {
		if err = f.service.DeleteUnreadableItem(tuple[0], tuple[1], tuple[2]); err == nil {
			t.Fatalf("deleted unreviewed tuple %v", tuple)
		}
	}
	f.apiServer.mutex.Lock()
	r := f.apiServer.itemsByVaultID[f.personalID]["judge-bad"]
	r.Revision = "changed-r2"
	f.apiServer.seq++
	r.Seq = f.apiServer.seq
	f.apiServer.itemsByVaultID[f.personalID]["judge-bad"] = r
	f.apiServer.mutex.Unlock()
	if err = f.service.DeleteUnreadableItem(f.personalID, "judge-bad", "reviewed-r1"); err == nil {
		t.Fatal("deleted concurrent revision without review")
	}
	if _, err = f.service.ListItems(f.personalID); err != nil {
		t.Fatal(err)
	}
	if err = f.service.DeleteUnreadableItem(f.personalID, "judge-bad", "changed-r2"); err != nil {
		t.Fatal(err)
	}
	f.apiServer.mutex.Lock()
	defer f.apiServer.mutex.Unlock()
	if !f.apiServer.itemsByVaultID[f.personalID]["judge-bad"].Deleted {
		t.Fatal("reviewed exact revision not deleted")
	}
	if f.apiServer.itemsByVaultID[f.personalID][healthy.ID].Deleted {
		t.Fatal("healthy item deleted")
	}
}

func judgeCorruptFamilyName(t *testing.T, f unlockedTestService) []byte {
	t.Helper()
	f.apiServer.mutex.Lock()
	defer f.apiServer.mutex.Unlock()
	for i := range f.apiServer.vaults {
		if f.apiServer.vaults[i].ID == f.familyID {
			old := append([]byte{}, f.apiServer.vaults[i].EncName...)
			f.apiServer.vaults[i].EncName = []byte("bad-name")
			return old
		}
	}
	t.Fatal("family missing")
	return nil
}

func TestJudgeStaleVaultIssueCannotDeleteRepairedVault(t *testing.T) {
	f := createUnlockedTestService(t)
	if _, err := f.service.SaveItem(f.familyID, ItemDetail{Title: "Important healthy item"}); err != nil {
		t.Fatal(err)
	}
	old := judgeCorruptFamilyName(t, f)
	summaries, err := f.service.ListVaults()
	if err != nil {
		t.Fatal(err)
	}
	revision := ""
	for _, s := range summaries {
		for _, i := range s.ReadIssues {
			if i.VaultID == f.familyID {
				revision = i.Revision
			}
		}
	}
	f.apiServer.mutex.Lock()
	for i := range f.apiServer.vaults {
		if f.apiServer.vaults[i].ID == f.familyID {
			f.apiServer.vaults[i].EncName = old
		}
	}
	f.apiServer.mutex.Unlock()
	if err := f.service.DeleteUnreadableVault(f.familyID, revision); err == nil {
		t.Fatal("stale unreadable issue deleted repaired healthy vault and every item without another review")
	}
}

func TestJudgeStaleVaultIssueCannotOverwriteRepairedName(t *testing.T) {
	f := createUnlockedTestService(t)
	judgeCorruptFamilyName(t, f)
	if _, err := f.service.ListVaults(); err != nil {
		t.Fatal(err)
	}
	f.service.mutex.Lock()
	v := f.service.vaultsByID[f.familyID]
	name, err := crypt.SealVaultName(v.Key, v.ID, []byte("Other device repaired"))
	f.service.mutex.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	f.apiServer.mutex.Lock()
	for i := range f.apiServer.vaults {
		if f.apiServer.vaults[i].ID == f.familyID {
			f.apiServer.vaults[i].EncName = name
		}
	}
	f.apiServer.mutex.Unlock()
	if _, err := f.service.RenameVault(f.familyID, "Stale recovery overwrite"); err == nil {
		t.Fatal("repair action overwrote concurrent healthy name without reviewing current record")
	}
}
