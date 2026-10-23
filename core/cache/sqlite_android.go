//go:build android && cgo

package cache

import (
	"errors"

	"github.com/mattn/go-sqlite3"
)

// Android opens SQLite through its own C library: on x86_64 the app sandbox kills a process that makes the old system
// calls modernc.org/libc uses (lstat, stat, access), while bionic makes the *at calls the sandbox allows.
const sqliteDriver = "sqlite3"

func buildSQLiteDataSource(path string) string {
	return "file:" + path + "?_busy_timeout=5000"
}

// IsCorrupt reports SQLite corruption or an invalid database format, not an I/O or access failure.
func IsCorrupt(err error) bool {
	var sqliteErr sqlite3.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	return sqliteErr.Code == sqlite3.ErrCorrupt || sqliteErr.Code == sqlite3.ErrNotADB
}
