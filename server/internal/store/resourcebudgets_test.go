package store

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tryhisar/hisar/core/apitypes"
)

func TestLegacyOverBudgetInventoryCanReadAndDeleteButNotGrow(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "owner", "owner@example.com")
	rollbackTestMigrationsTo(t, database, 16)
	if _, err := database.pool.Exec(ctx, `INSERT INTO vaults(id,type,enc_name,created_by) SELECT gen_random_uuid(),'shared','x'::bytea,'owner' FROM generate_series(1,1025);
 INSERT INTO vault_access(vault_id,account_id,wrapped_key,role,granted_by) SELECT id,'owner','x'::bytea,'owner','owner' FROM vaults`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	cursor := ""
	count := 0
	pages := 0
	first := ""
	for {
		page, err := database.ReadCollectionPage(ctx, "owner", "device", "/vaults", cursor)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		count += len(page.Records)
		if first == "" && len(page.Records) > 0 {
			var vault apitypes.Vault
			if err := json.Unmarshal(page.Records[0].Value, &vault); err != nil {
				t.Fatal(err)
			}
			first = vault.ID
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if count != 1025 || pages < 2 {
		t.Fatalf("legacy traversal count=%d pages=%d", count, pages)
	}
	create := func() error {
		return database.CreateVault(ctx, &Vault{ID: uuid.NewString(), Type: apitypes.VaultShared, EncName: []byte{1}, CreatedBy: "owner"}, []byte{1})
	}
	if err := create(); !IsResourceBudget(err) {
		t.Fatalf("growth was not refused: %v", err)
	}
	if _, err := database.pool.Exec(ctx, `DELETE FROM vaults WHERE id=$1`, first); err != nil {
		t.Fatal(err)
	}
	if err := create(); !IsResourceBudget(err) {
		t.Fatalf("still-full legacy inventory grew: %v", err)
	}
	if _, err := database.pool.Exec(ctx, `DELETE FROM vaults WHERE id=(SELECT id FROM vaults LIMIT 1)`); err != nil {
		t.Fatal(err)
	}
	if err := create(); err != nil {
		t.Fatalf("freed budget was not reusable: %v", err)
	}
	var actual, charged int
	if err := database.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM vaults WHERE created_by='owner'),(SELECT used FROM resource_usage WHERE resource='vaults' AND scope='owner')`).Scan(&actual, &charged); err != nil {
		t.Fatal(err)
	}
	if actual != 1024 || charged != actual {
		t.Fatalf("rows=%d charged=%d", actual, charged)
	}
	t.Logf("legacy over-budget smoke traversed=%d pages=%d; refused growth, deleted safely, reused one freed slot", count, pages)
}

func TestFirstUsageRowIsCapacityChecked(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "fresh", "fresh@example.com")
	_, err := database.pool.Exec(ctx, `INSERT INTO vaults(id,type,enc_name,created_by) SELECT gen_random_uuid(),'shared','x'::bytea,'fresh' FROM generate_series(1,1100)`)
	if ExceededResource(err) != "vaults" {
		t.Fatalf("one statement created 1,100 vaults for a creator with no usage yet: %v", err)
	}
	transaction, err := database.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transaction.Exec(ctx, `INSERT INTO vaults(id,type,enc_name,created_by) SELECT gen_random_uuid(),'shared','x'::bytea,'fresh' FROM generate_series(1,2000)`); err != nil {
		t.Fatal(err)
	}
	if err := transaction.Commit(ctx); ExceededResource(err) != "vaults" {
		t.Fatalf("a transaction committed 2,000 vaults for a creator with no usage yet: %v", err)
	}
	if _, err := database.pool.Exec(ctx, `INSERT INTO vaults(id,type,enc_name,created_by) SELECT gen_random_uuid(),'shared','x'::bytea,'fresh' FROM generate_series(1,1024)`); err != nil {
		t.Fatalf("a first usage exactly at the budget was refused: %v", err)
	}
	if rows, charged := countQueryRow(t, database, `SELECT count(*) FROM vaults WHERE created_by='fresh'`),
		countQueryRow(t, database, `SELECT used FROM resource_usage WHERE resource='vaults' AND scope='fresh'`); rows != 1024 || charged != rows {
		t.Fatalf("rows=%d charged=%d", rows, charged)
	}
}

func TestNewDeviceTakesThePlaceOfDevicesWithoutALiveSession(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	firstDevice, _ := createTestAccount(t, database, "owner", "owner@example.com")
	if _, err := database.pool.Exec(ctx, `
		WITH ended AS (INSERT INTO devices(id,account_id,name,platform) SELECT gen_random_uuid(),'owner','ended','p' FROM generate_series(1,255) RETURNING id)
		INSERT INTO sessions(token_hash,account_id,device_id,expires_at) SELECT uuid_send(gen_random_uuid()),'owner',id,now()-interval '1 second' FROM ended WHERE random()<0.5`); err != nil {
		t.Fatal(err)
	}
	signIn := func(name string) (*Device, error) {
		device := &Device{ID: uuid.NewString(), AccountID: "owner", Name: name, Platform: "p"}
		return device, database.CreateDeviceSession(ctx, device, &Session{TokenHash: []byte(name), AccountID: "owner", DeviceID: device.ID, ExpiresAt: time.Now().Add(time.Hour)}, testRecordHash())
	}
	if _, err := signIn("new"); err != nil {
		t.Fatalf("sign-in refused while 255 of 256 devices had no live session: %v", err)
	}
	listed, err := database.ListDevices(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || countQueryRow(t, database, `SELECT used FROM resource_usage WHERE resource='devices' AND scope='owner'`) != 2 ||
		!slices.ContainsFunc(listed, func(device Device) bool { return device.ID == firstDevice.ID }) {
		t.Fatalf("after the sign-in the account lists %+v", listed)
	}
	if _, err := database.pool.Exec(ctx, `
		WITH live AS (INSERT INTO devices(id,account_id,name,platform) SELECT gen_random_uuid(),'owner','live','p' FROM generate_series(1,254) RETURNING id)
		INSERT INTO sessions(token_hash,account_id,device_id,expires_at) SELECT uuid_send(gen_random_uuid()),'owner',id,now()+interval '1 hour' FROM live`); err != nil {
		t.Fatal(err)
	}
	refused, err := signIn("one too many")
	if ExceededResource(err) != "devices" {
		t.Fatalf("a 257th live device was accepted: %v", err)
	}
	if countQueryRow(t, database, `SELECT count(*) FROM devices WHERE id=$1`, refused.ID) != 0 ||
		countQueryRow(t, database, `SELECT count(*) FROM devices WHERE account_id='owner' AND revoked_at IS NULL`) != 256 {
		t.Fatal("the refused sign-in left a device or signed one out")
	}
}

func TestCollectionCursorRejectsMutationAndCrossAccountReuse(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "owner", "owner@example.com")
	createTestAccount(t, database, "other", "other@example.com")
	for range 101 {
		createTestVault(t, database, "owner")
	}
	first, err := database.ReadCollectionPage(ctx, "owner", "device", "/vaults", "")
	if err != nil || first.NextCursor == "" {
		t.Fatalf("first page: %v", err)
	}
	// An unrelated account's private write does not invalidate this inventory.
	createTestVault(t, database, "other")
	if _, err := database.ReadCollectionPage(ctx, "owner", "device", "/vaults", first.NextCursor); err != nil {
		t.Fatalf("unrelated scope caused restart: %v", err)
	}
	createTestVault(t, database, "owner")
	if _, err := database.ReadCollectionPage(ctx, "owner", "device", "/vaults", first.NextCursor); !errors.Is(err, ErrCollectionChanged) {
		t.Fatalf("mixed snapshot accepted: %v", err)
	}
	if _, err := database.ReadCollectionPage(ctx, "other", "device", "/vaults", first.NextCursor); !errors.Is(err, ErrConflict) {
		t.Fatalf("cross-account cursor accepted: %v", err)
	}
}
