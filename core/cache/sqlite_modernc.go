//go:build !android || !cgo

package cache

import _ "modernc.org/sqlite"

const sqliteDriver = "sqlite"

func buildSQLiteDataSource(path string) string {
	return "file:" + path + "?_pragma=busy_timeout(5000)"
}
