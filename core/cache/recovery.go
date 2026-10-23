package cache

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
)

// RecoveryJournal is independent of the resettable offline cache. Values must already be encrypted.
// SQLite supplies durable transactions and cross-process serialization on desktop and mobile.
type RecoveryJournal struct{ database *sql.DB }

func OpenRecoveryJournal(path string) (*RecoveryJournal, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	err = file.Chmod(0o600)
	file.Close()
	if err != nil {
		return nil, err
	}
	database, err := sql.Open(sqliteDriver, buildSQLiteDataSource(path))
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	for _, query := range []string{`PRAGMA synchronous = FULL`, `CREATE TABLE IF NOT EXISTS pending_recovery (id TEXT PRIMARY KEY, metadata BLOB NOT NULL, envelope BLOB NOT NULL)`} {
		if _, err := database.Exec(query); err != nil {
			database.Close()
			return nil, err
		}
	}
	return &RecoveryJournal{database: database}, nil
}

func (j *RecoveryJournal) Close() error { return j.database.Close() }

func (j *RecoveryJournal) Create(id string, metadata, envelope []byte) error {
	if len(metadata) > 64*1024 || len(envelope) > 1024*1024 {
		return errors.New("cache: pending recovery record is too large")
	}
	_, err := j.database.Exec(`INSERT INTO pending_recovery(id, metadata, envelope) SELECT ?, ?, ? WHERE (SELECT count(*) FROM pending_recovery) < 100`, id, metadata, envelope)
	if err != nil {
		return err
	}
	var exists bool
	if err := j.database.QueryRow(`SELECT EXISTS (SELECT 1 FROM pending_recovery WHERE id = ?)`, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errors.New("cache: resolve a pending recovery operation before starting another")
	}
	return nil
}

func (j *RecoveryJournal) Load(id string) (metadata, envelope []byte, err error) {
	err = j.database.QueryRow(`SELECT metadata, envelope FROM pending_recovery WHERE id = ?`, id).Scan(&metadata, &envelope)
	return
}

func (j *RecoveryJournal) ListMetadata() ([][]byte, error) {
	rows, err := j.database.Query(`SELECT metadata FROM pending_recovery ORDER BY id LIMIT 101`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	metadata := [][]byte{}
	for rows.Next() {
		var value []byte
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		metadata = append(metadata, value)
	}
	if len(metadata) > 100 {
		return nil, errors.New("cache: too many pending recovery records")
	}
	return metadata, rows.Err()
}

func (j *RecoveryJournal) UpdateMetadata(id string, expected, metadata []byte) error {
	if len(metadata) > 64*1024 {
		return errors.New("cache: pending recovery metadata is too large")
	}
	result, err := j.database.Exec(`UPDATE pending_recovery SET metadata = ? WHERE id = ? AND metadata = ?`, metadata, id, expected)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count != 1 {
		return errors.New("cache: recovery operation changed; review it again")
	}
	return err
}

func (j *RecoveryJournal) Remove(id string, expected []byte) error {
	result, err := j.database.Exec(`DELETE FROM pending_recovery WHERE id = ? AND metadata = ?`, id, expected)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count != 1 {
		return errors.New("cache: recovery operation changed; review it again")
	}
	return err
}
