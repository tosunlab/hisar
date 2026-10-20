package store

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/tryhisar/hisar/core/apitypes"
	"testing"
	"time"
)

func TestConcurrentRotationsKeepSharedActorVersionsOrdered(t *testing.T) {
	database := createTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for _, id := range []string{"OWNER1", "OWNER2", "AAAAAA", "BBBBBB"} {
		createTestAccount(t, database, id, id+"@example.com")
	}
	first := createTestVault(t, database, "OWNER1")
	second := createTestVault(t, database, "OWNER2")
	for _, vaultID := range []string{first, second} {
		if _, err := database.pool.Exec(ctx, `INSERT INTO vault_access(vault_id,account_id,wrapped_key,role,granted_by) VALUES($1,'AAAAAA','x','reader','OWNER1'),($1,'BBBBBB','x','reader','OWNER1')`, vaultID); err != nil {
			t.Fatal(err)
		}
	}
	barrier, err := database.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer barrier.Rollback(context.Background())
	if _, err := barrier.Exec(ctx, `SELECT pg_advisory_xact_lock(17101),pg_advisory_xact_lock(17102)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.pool.Exec(ctx, fmt.Sprintf(`CREATE FUNCTION rotation_resource_barrier() RETURNS trigger LANGUAGE plpgsql AS $$
 BEGIN
  IF NEW.vault_id='%s'::uuid AND NEW.account_id='BBBBBB' THEN PERFORM pg_advisory_xact_lock(17101); END IF;
  IF NEW.vault_id='%s'::uuid AND NEW.account_id='AAAAAA' THEN PERFORM pg_advisory_xact_lock(17102); END IF;
  RETURN NEW;
 END $$;
 CREATE TRIGGER rotation_resource_barrier BEFORE UPDATE ON vault_access FOR EACH ROW EXECUTE FUNCTION rotation_resource_barrier()`, first, second)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		database.pool.Exec(context.Background(), `DROP TRIGGER rotation_resource_barrier ON vault_access; DROP FUNCTION rotation_resource_barrier()`)
	})
	access := func(id, role string) Member { return Member{AccountID: id, Role: role, WrappedKey: []byte{2}} }
	done := make(chan error, 2)
	go func() {
		done <- database.RotateVault(ctx, VaultRotation{VaultID: first, RotatedBy: "OWNER1", KeyGeneration: 1, EncName: []byte{2}, Accesses: []Member{access("AAAAAA", "reader"), access("BBBBBB", "reader"), access("OWNER1", "owner")}})
	}()
	go func() {
		done <- database.RotateVault(ctx, VaultRotation{VaultID: second, RotatedBy: "OWNER2", KeyGeneration: 1, EncName: []byte{2}, Accesses: []Member{access("BBBBBB", "reader"), access("AAAAAA", "reader"), access("OWNER2", "owner")}})
	}()
	for deadline := time.Now().Add(8 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		var waiting int
		if err := database.pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity a WHERE datname=current_database() AND query LIKE '%UPDATE vault_access SET wrapped_key%' AND EXISTS(SELECT 1 FROM pg_locks l WHERE l.pid=a.pid AND l.locktype='advisory' AND NOT l.granted)`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the two real rotations did not reach opposite second-member barriers")
		}
	}
	if err := barrier.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := <-done; err != nil {
			t.Fatalf("concurrent rotation: %v", err)
		}
	}
	if countQueryRow(t, database, `SELECT count(*) FROM vaults WHERE id=ANY($1::uuid[]) AND key_generation=2`, []string{first, second}) != 2 {
		t.Fatal("a rotation did not activate")
	}
	if countQueryRow(t, database, `SELECT count(*) FROM resource_pending`) != 0 {
		t.Fatal("committed policy queue remained")
	}
	t.Log("two actual different-vault rotations committed after opposite A/B member updates")
}

func TestCollectionIncarnationSurvivesBackingReclamation(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "owner", "owner@example.com")
	vaultID := uuid.NewString()
	itemID := uuid.NewString()
	create := func() {
		if err := database.CreateVault(ctx, &Vault{ID: vaultID, Type: apitypes.VaultShared, EncName: []byte{1}, CreatedBy: "owner"}, []byte{1}); err != nil {
			t.Fatal(err)
		}
		if _, err := database.pool.Exec(ctx, `INSERT INTO items(vault_id,id,revision,seq,overview,details,updated_by) VALUES($1,$2,gen_random_uuid(),1,'x','x','owner')`, vaultID, itemID); err != nil {
			t.Fatal(err)
		}
		if _, err := database.pool.Exec(ctx, `INSERT INTO item_history(vault_id,id,revision,deleted,overview,details,updated_at,updated_by) SELECT $1,$2,gen_random_uuid(),false,'x','x',now(),'owner' FROM generate_series(1,101)`, vaultID, itemID); err != nil {
			t.Fatal(err)
		}
	}
	create()
	path := "/vaults/" + vaultID + "/items/" + itemID + "/history"
	first, err := database.ReadCollectionPage(ctx, "owner", "device", path, "")
	if err != nil || first.NextCursor == "" {
		t.Fatalf("first history page: %v", err)
	}
	if _, err := database.pool.Exec(ctx, `DELETE FROM vaults WHERE id=$1`, vaultID); err != nil {
		t.Fatal(err)
	}
	if countQueryRow(t, database, `SELECT count(*) FROM collection_versions WHERE scope=$1`, "v/"+vaultID) != 0 {
		t.Fatal("retired version backing was retained")
	}
	create()
	if _, err := database.ReadCollectionPage(ctx, "owner", "device", path, first.NextCursor); !errors.Is(err, ErrCollectionChanged) {
		t.Fatalf("same-count delete/recreate accepted an old cursor: %v", err)
	}
}

func TestHistoryBudgetKeepsOverBudgetHistoryAndDeletion(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "owner", "owner@example.com")
	vaultID := createTestVault(t, database, "owner")
	current := Item{ID: uuid.NewString(), Revision: uuid.NewString(), Overview: []byte{1}, Details: []byte{1}, UpdatedBy: "owner"}
	if _, err := database.PutItem(ctx, vaultID, "", 1, &current); err != nil {
		t.Fatal(err)
	}
	if _, err := database.pool.Exec(ctx, `INSERT INTO item_history(vault_id,id,revision,deleted,overview,details,updated_at,updated_by)
 SELECT $1,$2,gen_random_uuid(),false,'x',repeat('x',1048576)::bytea,now(),'owner' FROM generate_series(1,1024)`, vaultID, current.ID); err != nil {
		t.Fatal(err)
	}
	grown := Item{ID: current.ID, Revision: uuid.NewString(), Overview: []byte{2}, Details: []byte{2}, UpdatedBy: "owner"}
	if _, err := database.PutItem(ctx, vaultID, current.Revision, 1, &grown); !IsResourceBudget(err) {
		t.Fatalf("over-budget growth accepted: %v", err)
	}
	if countQueryRow(t, database, `SELECT count(*) FROM item_history WHERE vault_id=$1`, vaultID) != 1025 {
		t.Fatal("growth refusal trimmed or added history")
	}
	page, err := database.ReadCollectionPage(ctx, "owner", "device", "/vaults/"+vaultID+"/items/"+current.ID+"/history", "")
	if err != nil || page.NextCursor == "" || len(page.Records) == 0 {
		t.Fatalf("legacy history unreadable: %v", err)
	}
	deletion := Item{ID: current.ID, Revision: uuid.NewString(), Deleted: true, Overview: []byte{1}, Details: []byte{1}, UpdatedBy: "owner"}
	if _, err := database.PutItem(ctx, vaultID, current.Revision, 1, &deletion); err != nil {
		t.Fatalf("recovery deletion refused: %v", err)
	}
	repeated := deletion
	repeated.Revision = uuid.NewString()
	if _, err := database.PutItem(ctx, vaultID, deletion.Revision, 1, &repeated); !IsResourceBudget(err) {
		t.Fatalf("repeated deletion obtained free history growth: %v", err)
	}
	if countQueryRow(t, database, `SELECT count(*) FROM item_history WHERE vault_id=$1`, vaultID) != 1026 || countQueryRow(t, database, `SELECT count(*) FROM resource_pending`) != 0 {
		t.Fatal("failed write leaked queue state or changed history")
	}
	t.Log("1 GiB-plus legal-sized legacy revisions stayed readable; growth refused; one recovery deletion remained available")
}

func TestHistoryPruningDoesNotChangeAnActiveWriterBaseline(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "owner", "owner@example.com")
	vaultID := createTestVault(t, database, "owner")
	old := Item{ID: uuid.NewString(), Revision: uuid.NewString(), Overview: []byte{1}, Details: []byte{1}, UpdatedBy: "owner"}
	if _, err := database.PutItem(ctx, vaultID, "", 1, &old); err != nil {
		t.Fatal(err)
	}
	current := old
	current.Revision = uuid.NewString()
	if _, err := database.PutItem(ctx, vaultID, old.Revision, 1, &current); err != nil {
		t.Fatal(err)
	}
	if _, err := database.pool.Exec(ctx, `UPDATE item_history SET updated_at=now()-interval '2 days' WHERE vault_id=$1 AND revision=$2`, vaultID, old.Revision); err != nil {
		t.Fatal(err)
	}
	writer, err := database.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback(ctx)
	if _, err := writer.Exec(ctx, `SELECT id FROM vaults WHERE id=$1 FOR UPDATE`, vaultID); err != nil {
		t.Fatal(err)
	}
	if pruned, err := database.PruneHistory(ctx, time.Hour); err != nil || pruned != 0 {
		t.Fatalf("pruning changed the active writer baseline: %d %v", pruned, err)
	}
	if err := writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if pruned, err := database.PruneHistory(ctx, time.Hour); err != nil || pruned != 1 {
		t.Fatalf("idle vault did not prune normally: %d %v", pruned, err)
	}
}
