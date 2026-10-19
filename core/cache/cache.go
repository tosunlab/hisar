// Package cache keeps the server's encrypted account, vault and item records in a local SQLite file for offline use,
// and the device's sealed website icons.
package cache

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/tryhisar/hisar/core/apitypes"
)

const schemaVersion = 1

var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS accounts (account_id TEXT PRIMARY KEY, data BLOB NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS vaults (vault_id TEXT PRIMARY KEY, data BLOB, seq INTEGER NOT NULL DEFAULT 0)`,
	`CREATE TABLE IF NOT EXISTS items (vault_id TEXT NOT NULL, item_id TEXT NOT NULL, data BLOB NOT NULL, PRIMARY KEY (vault_id, item_id))`,
	`CREATE TABLE IF NOT EXISTS account_preferences (account_id TEXT PRIMARY KEY, data BLOB NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS website_icons (lookup TEXT PRIMARY KEY, data BLOB NOT NULL)`,
}

var dropStatements = []string{`DROP TABLE IF EXISTS accounts`, `DROP TABLE IF EXISTS vaults`, `DROP TABLE IF EXISTS items`, `DROP TABLE IF EXISTS account_preferences`, `DROP TABLE IF EXISTS website_icons`}

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
	database, err := sql.Open(sqliteDriver, buildSQLiteDataSource(path))
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
	statements := schemaStatements
	if currentVersion != schemaVersion {
		statements = append(append([]string{}, dropStatements...), schemaStatements...)
		statements = append(statements, fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion))
	}
	for _, statement := range statements {
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
	var errs []error
	for _, suffix := range []string{"", "-journal", "-wal", "-shm"} {
		if err := os.Remove(path + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
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

// SaveAccountPreferences keeps the account's sealed preferences record as the server returned it.
func (c *Cache) SaveAccountPreferences(accountID string, preferences *apitypes.AccountPreferences) error {
	data, err := json.Marshal(preferences)
	if err != nil {
		return err
	}
	_, err = c.database.Exec(`INSERT INTO account_preferences (account_id, data) VALUES (?, ?) ON CONFLICT (account_id) DO UPDATE SET data = excluded.data`, accountID, data)
	return err
}

// SaveWebsiteIcon keeps a sealed website icon record under its lookup key (crypt.WebsiteIconKeys).
func (c *Cache) SaveWebsiteIcon(lookup string, sealed []byte) error {
	_, err := c.database.Exec(`INSERT INTO website_icons (lookup, data) VALUES (?, ?) ON CONFLICT (lookup) DO UPDATE SET data = excluded.data`, lookup, sealed)
	return err
}

// LoadWebsiteIcon returns nil when no record is cached.
func (c *Cache) LoadWebsiteIcon(lookup string) ([]byte, error) {
	var data []byte
	err := c.database.QueryRow(`SELECT data FROM website_icons WHERE lookup = ?`, lookup).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return data, err
}

// ClearWebsiteIcons drops every cached website icon.
func (c *Cache) ClearWebsiteIcons() error {
	_, err := c.database.Exec(`DELETE FROM website_icons`)
	return err
}

// LoadAccountPreferences returns nil when no record is cached.
func (c *Cache) LoadAccountPreferences(accountID string) (*apitypes.AccountPreferences, error) {
	var data []byte
	err := c.database.QueryRow(`SELECT data FROM account_preferences WHERE account_id = ?`, accountID).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	preferences := &apitypes.AccountPreferences{}
	if err := json.Unmarshal(data, preferences); err != nil {
		return nil, err
	}
	return preferences, nil
}

// ReplaceVaults stores the server's vault list and forgets vaults, with their items, that are no longer in it.
func (c *Cache) ReplaceVaults(vaults []apitypes.Vault) error {
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
	for index := range vaults {
		data, err := json.Marshal(&vaults[index])
		if err != nil {
			return err
		}
		if _, err := transaction.Exec(`INSERT INTO vaults (vault_id, data) VALUES (?, ?) ON CONFLICT (vault_id) DO UPDATE SET data = excluded.data`, vaults[index].ID, data); err != nil {
			return err
		}
		if _, err := transaction.Exec(`INSERT OR IGNORE INTO listed_vaults (vault_id) VALUES (?)`, vaults[index].ID); err != nil {
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

func (c *Cache) ListVaults() ([]apitypes.Vault, error) {
	rows, err := c.database.Query(`SELECT data FROM vaults WHERE data IS NOT NULL ORDER BY vault_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	vaults := []apitypes.Vault{}
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var vault apitypes.Vault
		if err := json.Unmarshal(data, &vault); err != nil {
			return nil, err
		}
		vaults = append(vaults, vault)
	}
	return vaults, rows.Err()
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

// ApplyChanges stores one page of changes and the seq it reaches in one transaction; deleted items are removed.
func (c *Cache) ApplyChanges(vaultID string, items []apitypes.Item, nextSeq int64) error {
	transaction, err := c.database.Begin()
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	for index := range items {
		if items[index].Deleted {
			if _, err := transaction.Exec(`DELETE FROM items WHERE vault_id = ? AND item_id = ?`, vaultID, items[index].ID); err != nil {
				return err
			}
			continue
		}
		data, err := json.Marshal(&items[index])
		if err != nil {
			return err
		}
		if _, err := transaction.Exec(`INSERT INTO items (vault_id, item_id, data) VALUES (?, ?, ?) ON CONFLICT (vault_id, item_id) DO UPDATE SET data = excluded.data`, vaultID, items[index].ID, data); err != nil {
			return err
		}
	}
	if _, err := transaction.Exec(`INSERT INTO vaults (vault_id, seq) VALUES (?, ?) ON CONFLICT (vault_id) DO UPDATE SET seq = excluded.seq`, vaultID, nextSeq); err != nil {
		return err
	}
	return transaction.Commit()
}

func (c *Cache) ListItems(vaultID string) ([]apitypes.Item, error) {
	rows, err := c.database.Query(`SELECT data FROM items WHERE vault_id = ? ORDER BY item_id`, vaultID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []apitypes.Item{}
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var protoItem apitypes.Item
		if err := json.Unmarshal(data, &protoItem); err != nil {
			return nil, err
		}
		items = append(items, protoItem)
	}
	return items, rows.Err()
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
