package dbio

// SQLite LIBNAME engine. SQLite is a single-file (or in-memory) SQL database, so
// `libname db sqlite "/path/to/file.db";` binds a libref to one database file
// and its tables read/write as datasets. The driver (modernc.org/sqlite) is
// pure Go, so this engine is always available — no CGo or C compiler required.

import _ "modernc.org/sqlite"

func init() {
	engineDriver["sqlite"] = "sqlite"
	engineDriver["sqlite3"] = "sqlite"
}
