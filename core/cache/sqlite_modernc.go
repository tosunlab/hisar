//go:build !android || !cgo

package cache

import (
	"errors"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

const sqliteDriver = "sqlite"

func buildSQLiteDataSource(path string) string {
	return "file:" + path + "?_pragma=busy_timeout(5000)"
}

// IsCorrupt reports SQLite corruption or an invalid database format, not an I/O or access failure.
func IsCorrupt(err error) bool {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	code := sqliteErr.Code() & 0xff
	return code == sqlite3.SQLITE_CORRUPT || code == sqlite3.SQLITE_NOTADB
}
