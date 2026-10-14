// Package cache keeps the server's encrypted account, vault and item records in a local SQLite file for offline use.
package cache

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	_ "modernc.org/sqlite"

	"github.com/tryhisar/hisar/core/apitypes"
)

const schemaVersion = 1

var allSchemaStatement = []string{
	`CREATE TABLE IF NOT EXISTS accounts (account_id TEXT PRIMARY KEY, data BLOB NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS vaults (vault_id TEXT PRIMARY KEY, data BLOB, seq INTEGER NOT NULL DEFAULT 0)`,
	`CREATE TABLE IF NOT EXISTS items (vault_id TEXT NOT NULL, item_id TEXT NOT NULL, data BLOB NOT NULL, PRIMARY KEY (vault_id, item_id))`,
}

var allDropStatement = []string{`DROP TABLE IF EXISTS accounts`, `DROP TABLE IF EXISTS vaults`, `DROP TABLE IF EXISTS items`}

type Cache struct {
	database *sql.DB
}

// Open creates the file with mode 0600 if needed; a file from another schema version is emptied.
func Open(path string) (*Cache, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	file.Close()
	database, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	if err := prepareSchema(database); err != nil {
		database.Close()
		return nil, fmt.Errorf("cache %s: %w", path, err)
	}
	return &Cache{database: database}, nil
}

func prepareSchema(database *sql.DB) error {
	var currentVersion int
	if err := database.QueryRow(`PRAGMA user_version`).Scan(&currentVersion); err != nil {
		return err
	}
	allStatement := allSchemaStatement
	if currentVersion != schemaVersion {
		allStatement = append(append([]string{}, allDropStatement...), allSchemaStatement...)
		allStatement = append(allStatement, fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion))
	}
	for _, statement := range allStatement {
		if _, err := database.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func (c *Cache) Close() error {
	return c.database.Close()
}

// Remove deletes the cache file and SQLite's side files.
func Remove(path string) error {
	var allErr []error
	for _, suffix := range []string{"", "-journal", "-wal", "-shm"} {
		if err := os.Remove(path + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			allErr = append(allErr, err)
		}
	}
	return errors.Join(allErr...)
}

func (c *Cache) SaveAccount(account *apitypes.Account) error {
	data, err := json.Marshal(account)
	if err != nil {
		return err
	}
	_, err = c.database.Exec(`INSERT INTO accounts (account_id, data) VALUES (?, ?) ON CONFLICT (account_id) DO UPDATE SET data = excluded.data`, account.AccountID, data)
	return err
}

// LoadAccount returns nil when the account is not cached.
func (c *Cache) LoadAccount(accountID string) (*apitypes.Account, error) {
	var data []byte
	err := c.database.QueryRow(`SELECT data FROM accounts WHERE account_id = ?`, accountID).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	account := &apitypes.Account{}
	if err := json.Unmarshal(data, account); err != nil {
		return nil, err
	}
	return account, nil
}

// ReplaceAllVault stores the server's vault list and forgets vaults, with their items, that are no longer in it.
func (c *Cache) ReplaceAllVault(allVault []apitypes.Vault) error {
	transaction, err := c.database.Begin()
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if _, err := transaction.Exec(`CREATE TEMP TABLE IF NOT EXISTS listed_vaults (vault_id TEXT PRIMARY KEY)`); err != nil {
		return err
	}
	if _, err := transaction.Exec(`DELETE FROM listed_vaults`); err != nil {
		return err
	}
	for index := range allVault {
		data, err := json.Marshal(&allVault[index])
		if err != nil {
			return err
		}
		if _, err := transaction.Exec(`INSERT INTO vaults (vault_id, data) VALUES (?, ?) ON CONFLICT (vault_id) DO UPDATE SET data = excluded.data`, allVault[index].ID, data); err != nil {
			return err
		}
		if _, err := transaction.Exec(`INSERT OR IGNORE INTO listed_vaults (vault_id) VALUES (?)`, allVault[index].ID); err != nil {
			return err
		}
	}
	if _, err := transaction.Exec(`DELETE FROM items WHERE vault_id NOT IN (SELECT vault_id FROM listed_vaults)`); err != nil {
		return err
	}
	if _, err := transaction.Exec(`DELETE FROM vaults WHERE vault_id NOT IN (SELECT vault_id FROM listed_vaults)`); err != nil {
		return err
	}
	return transaction.Commit()
}

func (c *Cache) ListAllVault() ([]apitypes.Vault, error) {
	allRow, err := c.database.Query(`SELECT data FROM vaults WHERE data IS NOT NULL ORDER BY vault_id`)
	if err != nil {
		return nil, err
	}
	defer allRow.Close()
	allVault := []apitypes.Vault{}
	for allRow.Next() {
		var data []byte
		if err := allRow.Scan(&data); err != nil {
			return nil, err
		}
		var vault apitypes.Vault
		if err := json.Unmarshal(data, &vault); err != nil {
			return nil, err
		}
		allVault = append(allVault, vault)
	}
	return allVault, allRow.Err()
}

// GetSeq returns the vault seq the cached items are complete up to, 0 when nothing is cached.
func (c *Cache) GetSeq(vaultID string) (int64, error) {
	var seq int64
	err := c.database.QueryRow(`SELECT seq FROM vaults WHERE vault_id = ?`, vaultID).Scan(&seq)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return seq, err
}

// ApplyAllChange stores one page of changes and the seq it reaches in one transaction; deleted items are removed.
func (c *Cache) ApplyAllChange(vaultID string, allItem []apitypes.Item, nextSeq int64) error {
	transaction, err := c.database.Begin()
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	for index := range allItem {
		if allItem[index].Deleted {
			if _, err := transaction.Exec(`DELETE FROM items WHERE vault_id = ? AND item_id = ?`, vaultID, allItem[index].ID); err != nil {
				return err
			}
			continue
		}
		data, err := json.Marshal(&allItem[index])
		if err != nil {
			return err
		}
		if _, err := transaction.Exec(`INSERT INTO items (vault_id, item_id, data) VALUES (?, ?, ?) ON CONFLICT (vault_id, item_id) DO UPDATE SET data = excluded.data`, vaultID, allItem[index].ID, data); err != nil {
			return err
		}
	}
	if _, err := transaction.Exec(`INSERT INTO vaults (vault_id, seq) VALUES (?, ?) ON CONFLICT (vault_id) DO UPDATE SET seq = excluded.seq`, vaultID, nextSeq); err != nil {
		return err
	}
	return transaction.Commit()
}

func (c *Cache) ListAllItem(vaultID string) ([]apitypes.Item, error) {
	allRow, err := c.database.Query(`SELECT data FROM items WHERE vault_id = ? ORDER BY item_id`, vaultID)
	if err != nil {
		return nil, err
	}
	defer allRow.Close()
	allItem := []apitypes.Item{}
	for allRow.Next() {
		var data []byte
		if err := allRow.Scan(&data); err != nil {
			return nil, err
		}
		var protoItem apitypes.Item
		if err := json.Unmarshal(data, &protoItem); err != nil {
			return nil, err
		}
		allItem = append(allItem, protoItem)
	}
	return allItem, allRow.Err()
}

// ResetVault drops a vault's items and seq so the next sync downloads everything again.
func (c *Cache) ResetVault(vaultID string) error {
	transaction, err := c.database.Begin()
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if _, err := transaction.Exec(`DELETE FROM items WHERE vault_id = ?`, vaultID); err != nil {
		return err
	}
	if _, err := transaction.Exec(`UPDATE vaults SET seq = 0 WHERE vault_id = ?`, vaultID); err != nil {
		return err
	}
	return transaction.Commit()
}
