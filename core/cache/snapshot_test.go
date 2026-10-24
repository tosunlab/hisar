package cache

import (
	"github.com/tryhisar/hisar/core/apitypes"
	"testing"
)

func TestCompletedSnapshotSurvivesInterruptedTraversalAndRestart(t *testing.T) {
	c, path := openTestCache(t)
	record := apitypes.Vault{ID: "v", KeyGeneration: 1}
	if err := c.ReplaceVaults([]apitypes.Vault{record}, nil); err != nil {
		t.Fatal(err)
	}
	checkpoint, err := c.GetCheckpoint("v")
	if err != nil || checkpoint.HasCompletedSync {
		t.Fatalf("metadata is not sync evidence: %+v %v", checkpoint, err)
	}
	staged, err := c.BeginSync("v", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := staged.Apply([]apitypes.Item{{ID: "healthy", Revision: "r1"}}); err != nil {
		t.Fatal(err)
	}
	if err := staged.Commit(1, &record); err != nil {
		t.Fatal(err)
	}
	previous, err := c.GetCheckpoint("v")
	if err != nil || !previous.HasCompletedSync {
		t.Fatalf("missing published checkpoint: %+v %v", previous, err)
	}
	staged, err = c.BeginSync("v", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := staged.Apply([]apitypes.Item{{ID: "partial", Revision: "r2"}}); err != nil {
		t.Fatal(err)
	}
	staged.Rollback()
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	items, err := reopened.ListItems("v")
	if err != nil || len(items) != 1 || items[0].ID != "healthy" {
		t.Fatalf("partial traversal replaced usable snapshot: %+v %v", items, err)
	}
	current, err := reopened.GetCheckpoint("v")
	if err != nil || current != previous {
		t.Fatalf("checkpoint changed on interrupted traversal: %+v %v", current, err)
	}
}

func TestEmptyTerminalSnapshotAndLegacyWrites(t *testing.T) {
	c, _ := openTestCache(t)
	staged, err := c.BeginSync("empty", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := staged.Commit(0, &apitypes.Vault{ID: "empty", KeyGeneration: 1}); err != nil {
		t.Fatal(err)
	}
	checkpoint, err := c.GetCheckpoint("empty")
	if err != nil || !checkpoint.HasCompletedSync || checkpoint.CompletedSequence != 0 {
		t.Fatalf("empty completion: %+v %v", checkpoint, err)
	}
	if err := c.ApplyChanges("empty", []apitypes.Item{{ID: "old-client-write"}}, 1); err != nil {
		t.Fatal(err)
	}
	checkpoint, err = c.GetCheckpoint("empty")
	if err != nil || checkpoint.HasCompletedSync {
		t.Fatalf("legacy write kept false provenance: %+v %v", checkpoint, err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyCacheCiphertextIsNotDeletedOrDeclaredComplete(t *testing.T) {
	c, path := openTestCache(t)
	if err := c.ApplyChanges("legacy", []apitypes.Item{{ID: "only-copy", Details: []byte("ciphertext")}}, 55); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	items, err := reopened.ListItems("legacy")
	if err != nil || len(items) != 1 || string(items[0].Details) != "ciphertext" {
		t.Fatalf("legacy copy lost: %+v %v", items, err)
	}
	checkpoint, err := reopened.GetCheckpoint("legacy")
	if err != nil || checkpoint.HasCompletedSync {
		t.Fatalf("positive legacy sequence proved completion: %+v %v", checkpoint, err)
	}
}

func TestRotatedRecordWaitsForTheCiphertextItsKeyOpens(t *testing.T) {
	c, path := openTestCache(t)
	first := apitypes.Vault{ID: "v", WrappedKey: []byte("first key"), KeyGeneration: 1}
	if err := c.ReplaceVaults([]apitypes.Vault{first}, nil); err != nil {
		t.Fatal(err)
	}
	staged, err := c.BeginSync("v", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := staged.Apply([]apitypes.Item{{ID: "sealed with the first key", Revision: "r1"}}); err != nil {
		t.Fatal(err)
	}
	if err := staged.Commit(1, &first); err != nil {
		t.Fatal(err)
	}
	second := apitypes.Vault{ID: "v", WrappedKey: []byte("second key"), KeyGeneration: 2}
	if err := c.ReplaceVaults([]apitypes.Vault{second}, nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	c, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	vaults, err := c.ListVaults()
	if err != nil || len(vaults) != 1 || string(vaults[0].WrappedKey) != "first key" {
		t.Fatalf("the key of the published items was replaced before their replacement: %+v %v", vaults, err)
	}
	if checkpoint, err := c.GetCheckpoint("v"); err != nil || !checkpoint.HasCompletedSync || checkpoint.KeyGeneration != 1 {
		t.Fatalf("published checkpoint lost: %+v %v", checkpoint, err)
	}
	staged, err = c.BeginSync("v", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := staged.Apply([]apitypes.Item{{ID: "sealed with the second key", Revision: "r2"}}); err != nil {
		t.Fatal(err)
	}
	if err := staged.Commit(2, &second); err != nil {
		t.Fatal(err)
	}
	vaults, err = c.ListVaults()
	if err != nil || len(vaults) != 1 || string(vaults[0].WrappedKey) != "second key" {
		t.Fatalf("the replacement's key was not published with it: %+v %v", vaults, err)
	}
	items, err := c.ListItems("v")
	if err != nil || len(items) != 1 || items[0].ID != "sealed with the second key" {
		t.Fatalf("items: %+v %v", items, err)
	}
	if checkpoint, err := c.GetCheckpoint("v"); err != nil || checkpoint.KeyGeneration != 2 {
		t.Fatalf("checkpoint: %+v %v", checkpoint, err)
	}
}

func TestUnverifiedRecordStaysAKnownOmissionUntilReplaced(t *testing.T) {
	c, path := openTestCache(t)
	good := apitypes.Vault{ID: "good", EncName: []byte("readable")}
	if err := c.ReplaceVaults([]apitypes.Vault{good}, nil); err != nil {
		t.Fatal(err)
	}
	damaged := apitypes.Vault{ID: "good", EncName: []byte("damaged")}
	unseen := apitypes.Vault{ID: "unseen", WrappedKey: []byte("unopenable")}
	if err := c.ReplaceVaults(nil, []apitypes.Vault{damaged, unseen}); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	c, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	vaults, err := c.ListVaults()
	if err != nil || len(vaults) != 2 || string(vaults[0].EncName) != "readable" || vaults[1].ID != "unseen" {
		t.Fatalf("an unverified record replaced a verified one or was forgotten: %+v %v", vaults, err)
	}
	if err := c.ReplaceVaults([]apitypes.Vault{good}, nil); err != nil {
		t.Fatal(err)
	}
	if vaults, err := c.ListVaults(); err != nil || len(vaults) != 1 || vaults[0].ID != "good" {
		t.Fatalf("a vault the server stopped listing stayed: %+v %v", vaults, err)
	}
}
