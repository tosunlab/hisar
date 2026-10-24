package cache

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/snapshot"
)

var snapshotSchema = []string{
	`CREATE TABLE IF NOT EXISTS sync_checkpoints (vault_id TEXT PRIMARY KEY, seq INTEGER NOT NULL, completed_at TEXT NOT NULL, generation INTEGER NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS observed_vaults (vault_id TEXT PRIMARY KEY, data BLOB NOT NULL)`,
	`CREATE TRIGGER IF NOT EXISTS invalidate_items_insert AFTER INSERT ON items BEGIN DELETE FROM sync_checkpoints WHERE vault_id = NEW.vault_id; END`,
	`CREATE TRIGGER IF NOT EXISTS invalidate_items_update AFTER UPDATE ON items BEGIN DELETE FROM sync_checkpoints WHERE vault_id = OLD.vault_id OR vault_id = NEW.vault_id; END`,
	`CREATE TRIGGER IF NOT EXISTS invalidate_items_delete AFTER DELETE ON items BEGIN DELETE FROM sync_checkpoints WHERE vault_id = OLD.vault_id; END`,
	`CREATE TRIGGER IF NOT EXISTS invalidate_vault_seq AFTER UPDATE OF seq ON vaults BEGIN DELETE FROM sync_checkpoints WHERE vault_id = NEW.vault_id; END`,
	`CREATE TRIGGER IF NOT EXISTS invalidate_vault_delete AFTER DELETE ON vaults BEGIN DELETE FROM sync_checkpoints WHERE vault_id = OLD.vault_id; END`,
	`CREATE TRIGGER IF NOT EXISTS invalidate_vault_generation AFTER UPDATE OF data ON vaults WHEN json_extract(OLD.data, '$.keyGeneration') IS NOT json_extract(NEW.data, '$.keyGeneration') BEGIN DELETE FROM sync_checkpoints WHERE vault_id = NEW.vault_id; END`,
}

func (c *Cache) GetCheckpoint(vaultID string) (snapshot.Checkpoint, error) {
	var checkpoint snapshot.Checkpoint
	err := c.database.QueryRow(`SELECT seq, completed_at, generation FROM sync_checkpoints WHERE vault_id = ?`, vaultID).Scan(&checkpoint.CompletedSequence, &checkpoint.CompletedAt, &checkpoint.KeyGeneration)
	if errors.Is(err, sql.ErrNoRows) {
		return checkpoint, nil
	}
	checkpoint.HasCompletedSync = err == nil
	return checkpoint, err
}

// stagedSync owns unpublished changes. Closing or process death rolls them back.
type stagedSync struct {
	transaction *sql.Tx
	vaultID     string
}

func (c *Cache) BeginSync(vaultID string, reset bool) (snapshot.Staging, error) {
	transaction, err := c.database.Begin()
	if err != nil {
		return nil, err
	}
	sync := &stagedSync{transaction: transaction, vaultID: vaultID}
	if reset {
		if _, err := transaction.Exec(`DELETE FROM items WHERE vault_id = ?`, vaultID); err != nil {
			transaction.Rollback()
			return nil, err
		}
	}
	return sync, nil
}

func (s *stagedSync) Apply(items []apitypes.Item) error {
	for index := range items {
		record := &items[index]
		if record.Deleted {
			if _, err := s.transaction.Exec(`DELETE FROM items WHERE vault_id = ? AND item_id = ?`, s.vaultID, record.ID); err != nil {
				return err
			}
			continue
		}
		data, err := json.Marshal(record)
		if err != nil {
			return err
		}
		if _, err := s.transaction.Exec(`INSERT INTO items (vault_id, item_id, data) VALUES (?, ?, ?) ON CONFLICT (vault_id, item_id) DO UPDATE SET data = excluded.data`, s.vaultID, record.ID, data); err != nil {
			return err
		}
	}
	return nil
}

// Commit publishes the staged items, the vault record whose key they are sealed with and the completion checkpoint
// together. A record of the cached key generation leaves the cached one, which ReplaceVaults keeps current, in place.
func (s *stagedSync) Commit(sequence int64, vault *apitypes.Vault) error {
	if vault.ID != s.vaultID {
		return errors.New("cache: the published record belongs to another vault")
	}
	data, err := json.Marshal(vault)
	if err != nil {
		return err
	}
	if _, err := s.transaction.Exec(`INSERT INTO vaults (vault_id, data, seq) VALUES (?, ?, ?) ON CONFLICT (vault_id) DO UPDATE SET seq = excluded.seq,
		data = CASE WHEN vaults.data IS NULL OR json_extract(vaults.data, '$.keyGeneration') IS NOT json_extract(excluded.data, '$.keyGeneration') THEN excluded.data ELSE vaults.data END`,
		s.vaultID, data, sequence); err != nil {
		return err
	}
	if _, err := s.transaction.Exec(`INSERT INTO sync_checkpoints (vault_id, seq, completed_at, generation) VALUES (?, ?, ?, ?) ON CONFLICT (vault_id) DO UPDATE SET seq = excluded.seq, completed_at = excluded.completed_at, generation = excluded.generation`, s.vaultID, sequence, time.Now().UTC().Format(time.RFC3339Nano), vault.KeyGeneration); err != nil {
		return err
	}
	return s.transaction.Commit()
}

func (s *stagedSync) Rollback() { s.transaction.Rollback() }
