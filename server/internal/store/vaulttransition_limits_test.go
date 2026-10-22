package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tryhisar/hisar/core/apitypes"
)

func TestStagedRotationBatchesMoreThanOneThousandGrants(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	vaultID := createTestVault(t, database, "AAAAAA")
	rollbackTestMigrationsTo(t, database, 11)
	const recipientCount = 5001
	_, err := database.pool.Exec(ctx, `INSERT INTO accounts(id,email,name,role,argon2_params,salt,opaque_record,enc_keyset,kem_pub,sign_pub,kem_sig)
	 SELECT 'member-'||n,'member-'||n||'@example.com',name,role,argon2_params,salt,opaque_record,enc_keyset,kem_pub,sign_pub,kem_sig
	 FROM accounts CROSS JOIN generate_series(1,$1::int) n WHERE id='AAAAAA'`, recipientCount)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.pool.Exec(ctx, `INSERT INTO vault_access(vault_id,account_id,role,wrapped_key,granted_by)
	 SELECT $1,'member-'||n,'reader','old'::bytea,'AAAAAA' FROM generate_series(1,$2::int) n`, vaultID, recipientCount); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	op, err := database.BeginVaultTransition(ctx, vaultID, "AAAAAA", &apitypes.BeginVaultTransitionRequest{ID: uuid.NewString(), Kind: "rotate", KeyGeneration: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.StageVaultAccess(ctx, vaultID, "AAAAAA", op.ID, []apitypes.RotatedAccess{{AccountID: "AAAAAA", Role: "owner", WrappedKey: []byte("new-owner")}}); err != nil {
		t.Fatal(err)
	}
	for start := 1; start <= recipientCount; start += 100 {
		batch := []apitypes.RotatedAccess{}
		for member := start; member < min(start+100, recipientCount+1); member++ {
			batch = append(batch, apitypes.RotatedAccess{AccountID: fmt.Sprintf("member-%d", member), Role: "reader", WrappedKey: bytes.Repeat([]byte{7}, 4096)})
		}
		if err := database.StageVaultAccess(ctx, vaultID, "AAAAAA", op.ID, batch); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.CommitVaultTransition(ctx, vaultID, "AAAAAA", op.ID, []byte("new-name")); err != nil {
		t.Fatal(err)
	}
	members, err := database.ListMembers(ctx, vaultID)
	if err != nil || len(members) != recipientCount+1 {
		t.Fatalf("batched grants were truncated: %d %v", len(members), err)
	}
}

func TestStagedRotationHasNoWholeVaultItemCountLimit(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	vaultID := createTestVault(t, database, "AAAAAA")
	if _, err := database.pool.Exec(ctx, `INSERT INTO items(vault_id,id,revision,seq,overview,details,updated_by)
	 SELECT $1,gen_random_uuid(),gen_random_uuid(),n,'old-o'::bytea,'old-d'::bytea,'AAAAAA' FROM generate_series(1,50001) n`, vaultID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.pool.Exec(ctx, `UPDATE vaults SET seq=50001 WHERE id=$1`, vaultID); err != nil {
		t.Fatal(err)
	}
	op, err := database.BeginVaultTransition(ctx, vaultID, "AAAAAA", &apitypes.BeginVaultTransitionRequest{ID: uuid.NewString(), Kind: "rotate", KeyGeneration: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.pool.Exec(ctx, `INSERT INTO vault_transition_items(transition_id,id,base_revision,revision,deleted,overview,details)
	 SELECT $1,id,revision,gen_random_uuid(),deleted,'new-o'::bytea,'new-d'::bytea FROM items WHERE vault_id=$2`, op.ID, vaultID); err != nil {
		t.Fatal(err)
	}
	if err := database.StageVaultAccess(ctx, vaultID, "AAAAAA", op.ID, []apitypes.RotatedAccess{{AccountID: "AAAAAA", Role: "owner", WrappedKey: []byte("new-owner")}}); err != nil {
		t.Fatal(err)
	}
	receipt, err := database.CommitVaultTransition(ctx, vaultID, "AAAAAA", op.ID, []byte("new-name"))
	if err != nil || receipt.ItemCount != 50001 {
		t.Fatalf("accepted large item set could not rotate: %+v %v", receipt, err)
	}
}

func TestExpiredTransitionCleanupNeverDeletesUserRecords(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	vaultID := createTestVault(t, database, "AAAAAA")
	item := &Item{ID: uuid.NewString(), Revision: uuid.NewString(), Overview: []byte("old-o"), Details: []byte("old-d"), UpdatedBy: "AAAAAA"}
	if _, err := database.PutItem(ctx, vaultID, "", 1, item); err != nil {
		t.Fatal(err)
	}
	op, err := database.BeginVaultTransition(ctx, vaultID, "AAAAAA", &apitypes.BeginVaultTransitionRequest{ID: uuid.NewString(), Kind: "rotate", KeyGeneration: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.pool.Exec(ctx, `INSERT INTO vault_transition_items(transition_id,id,base_revision,revision,deleted,overview,details)
	 SELECT $1,gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),false,'o'::bytea,'d'::bytea FROM generate_series(1,1001)`, op.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.pool.Exec(ctx, `UPDATE vault_transitions SET expires_at=now()-interval '1 second' WHERE id=$1`, op.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CommitVaultTransition(ctx, vaultID, "AAAAAA", op.ID, []byte("new-name")); !errors.Is(err, ErrConflict) {
		t.Fatalf("expired staging committed: %v", err)
	}
	hold, err := database.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer hold.Rollback(ctx)
	if _, err := hold.Exec(ctx, `SELECT 1 FROM vault_transitions WHERE id=$1 FOR UPDATE`, op.ID); err != nil {
		t.Fatal(err)
	}
	sweep, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := database.CleanupVaultTransitions(sweep); err != nil {
		t.Fatalf("cleanup waited on an operation holding its lease: %v", err)
	}
	var heldCount int
	if err := database.pool.QueryRow(ctx, `SELECT count(*) FROM vault_transition_items WHERE transition_id=$1`, op.ID).Scan(&heldCount); err != nil || heldCount != 1001 {
		t.Fatalf("cleanup removed staging while its owner held the operation lock: %d %v", heldCount, err)
	}
	if err := hold.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err := database.CleanupVaultTransitions(ctx); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := database.pool.QueryRow(ctx, `SELECT count(*) FROM vault_transition_items WHERE transition_id=$1`, op.ID).Scan(&remaining); err != nil || remaining != 1 {
		t.Fatalf("cleanup was not bounded: %d %v", remaining, err)
	}
	current, _, _, err := database.ListChanges(ctx, vaultID, 0, 100, 1<<20)
	if err != nil || len(current) != 1 || current[0].Revision != item.Revision || string(current[0].Details) != "old-d" {
		t.Fatalf("cleanup touched user data: %+v %v", current, err)
	}
	if err := database.CleanupVaultTransitions(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := database.GetVaultTransition(ctx, vaultID, "AAAAAA", op.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired staging header remains after cleanup: %v", err)
	}
}

func TestStagingCannotInflateAcceptedCiphertextWithoutBound(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	vaultID := createTestVault(t, database, "AAAAAA")
	item := &Item{ID: uuid.NewString(), Revision: uuid.NewString(), Overview: []byte("o"), Details: []byte("d"), UpdatedBy: "AAAAAA"}
	if _, err := database.PutItem(ctx, vaultID, "", 1, item); err != nil {
		t.Fatal(err)
	}
	op, err := database.BeginVaultTransition(ctx, vaultID, "AAAAAA", &apitypes.BeginVaultTransitionRequest{ID: uuid.NewString(), Kind: "rotate", KeyGeneration: 1})
	if err != nil {
		t.Fatal(err)
	}
	staged := apitypes.RotatedItem{ID: item.ID, BaseRevision: item.Revision, Revision: uuid.NewString(), Overview: []byte("o"), Details: bytes.Repeat([]byte{7}, 5000)}
	if err := database.StageVaultItems(ctx, vaultID, "AAAAAA", op.ID, []apitypes.RotatedItem{staged}); !errors.Is(err, ErrResourceBudget) {
		t.Fatalf("staging inflated tiny accepted data: %v", err)
	}
	staged.Details = []byte("d")
	if err := database.StageVaultItems(ctx, vaultID, "AAAAAA", op.ID, []apitypes.RotatedItem{staged}); err != nil {
		t.Fatal(err)
	}
	if err := database.StageVaultItems(ctx, vaultID, "AAAAAA", op.ID, []apitypes.RotatedItem{staged}); err != nil {
		t.Fatalf("identical retry consumed staging capacity twice: %v", err)
	}
	if err := database.StageVaultAccess(ctx, vaultID, "AAAAAA", op.ID, []apitypes.RotatedAccess{{AccountID: "AAAAAA", Role: "owner", WrappedKey: bytes.Repeat([]byte{8}, 4096)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CommitVaultTransition(ctx, vaultID, "AAAAAA", op.ID, []byte("new-name")); err != nil {
		t.Fatalf("valid replacement failed after rejected inflation: %v", err)
	}
}

func TestInterruptedStagingNeverBlocksItsOwnersNextBegin(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	owners := []string{"AAAAAA", "BBBBBB", "CCCCCC"}
	for _, accountID := range owners {
		createTestAccount(t, database, accountID, accountID+"@example.com")
	}
	vaultID := createTestVault(t, database, "AAAAAA")
	for _, accountID := range owners[1:] {
		if err := database.SetAccess(ctx, vaultID, Member{AccountID: accountID, Role: "owner", WrappedKey: []byte("old"), GrantedBy: "AAAAAA"}); err != nil {
			t.Fatal(err)
		}
	}
	item := &Item{ID: uuid.NewString(), Revision: uuid.NewString(), Overview: []byte("o"), Details: []byte("d"), UpdatedBy: "AAAAAA"}
	if _, err := database.PutItem(ctx, vaultID, "", 1, item); err != nil {
		t.Fatal(err)
	}
	begin := func(actorID string) (*apitypes.VaultTransition, error) {
		return database.BeginVaultTransition(ctx, vaultID, actorID, &apitypes.BeginVaultTransitionRequest{ID: uuid.NewString(), Kind: "rotate", KeyGeneration: 1})
	}
	stage := func(actorID string, op *apitypes.VaultTransition) error {
		return database.StageVaultItems(ctx, vaultID, actorID, op.ID, []apitypes.RotatedItem{
			{ID: item.ID, BaseRevision: item.Revision, Revision: uuid.NewString(), Overview: []byte("o"), Details: []byte("d")}})
	}
	var lost *apitypes.VaultTransition
	for range 2 {
		op, err := begin("AAAAAA")
		if err != nil {
			t.Fatalf("an app that lost its earlier operation could not begin again: %v", err)
		}
		if err := stage("AAAAAA", op); err != nil {
			t.Fatal(err)
		}
		lost = op
	}
	current, err := begin("AAAAAA")
	if err != nil {
		t.Fatalf("two interrupted operations blocked their owner: %v", err)
	}
	if err := stage("AAAAAA", lost); !errors.Is(err, ErrConflict) {
		t.Fatalf("a superseded operation kept staging: %v", err)
	}
	if _, err := begin("BBBBBB"); err != nil {
		t.Fatal(err)
	}
	if _, err := begin("CCCCCC"); !errors.Is(err, ErrTransitionLimit) {
		t.Fatalf("a third owner's begin was admitted beside two other owners' live staging: %v", err)
	}
	if current, err = begin("AAAAAA"); err != nil {
		t.Fatalf("another owner's staging blocked replacing this owner's own operation: %v", err)
	}
	if err := stage("AAAAAA", current); err != nil {
		t.Fatal(err)
	}
	accesses := []apitypes.RotatedAccess{}
	for _, accountID := range owners {
		accesses = append(accesses, apitypes.RotatedAccess{AccountID: accountID, Role: "owner", WrappedKey: []byte("new")})
	}
	if err := database.StageVaultAccess(ctx, vaultID, "AAAAAA", current.ID, accesses); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CommitVaultTransition(ctx, vaultID, "AAAAAA", current.ID, []byte("new-name")); err != nil {
		t.Fatalf("the latest operation could not commit: %v", err)
	}
}

func TestEndedOperationsNeverExhaustAVaultsAdmission(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "AAAAAA", "a@example.com")
	vaultID := createTestVault(t, database, "AAAAAA")
	begin := func(generation int64) *apitypes.VaultTransition {
		t.Helper()
		op, err := database.BeginVaultTransition(ctx, vaultID, "AAAAAA", &apitypes.BeginVaultTransitionRequest{ID: uuid.NewString(), Kind: "rotate", KeyGeneration: generation})
		if err != nil {
			t.Fatalf("begin at generation %d: %v", generation, err)
		}
		return op
	}
	generation := int64(1)
	for range 70 {
		op := begin(generation)
		if err := database.StageVaultAccess(ctx, vaultID, "AAAAAA", op.ID, []apitypes.RotatedAccess{{AccountID: "AAAAAA", Role: "owner", WrappedKey: []byte("new")}}); err != nil {
			t.Fatal(err)
		}
		if _, err := database.CommitVaultTransition(ctx, vaultID, "AAAAAA", op.ID, []byte("new-name")); err != nil {
			t.Fatal(err)
		}
		generation++
	}
	for range 70 {
		if err := database.AbortVaultTransition(ctx, vaultID, "AAAAAA", begin(generation).ID); err != nil {
			t.Fatal(err)
		}
	}
	begin(generation)
}
