//go:build cgo

package dbio

// SQLite LIBNAME engine. SQLite is a single-file (or in-memory) SQL database, so
// `libname db sqlite "/path/to/file.db";` binds a libref to one database file
// and its tables read/write as datasets. The driver (github.com/mattn/go-sqlite3)
// is CGo-only, so this engine is registered only in cgo builds — matching the
// project's existing CGo requirement for PROC SQL. CGO_ENABLED=0 builds simply
// omit it (Open reports a clear "not supported" error).

import _ "github.com/mattn/go-sqlite3"

func init() {
	engineDriver["sqlite"] = "sqlite3"
	engineDriver["sqlite3"] = "sqlite3"
}
