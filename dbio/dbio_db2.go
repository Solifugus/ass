//go:build db2

package dbio

// DB2 LIBNAME engine. `libname db db2 "HOSTNAME=host;PORT=50000;DATABASE=db;UID=user;PWD=pw";`
// binds a libref to an IBM Db2 database; its tables read/write as datasets like
// any other dbio engine.
//
// The driver (github.com/ibmdb/go_ibm_db) is CGo and links against IBM's CLI
// driver (clidriver), a native library the driver's installer downloads. Because
// that pulls a large platform-specific blob and a C toolchain, DB2 is gated
// behind its own `db2` build tag — a plain `go build`/`go test` (even with CGo on
// for SQLite/PROC SQL) does not compile this file, so it needs neither the CLI
// driver nor the go_ibm_db module. Build the DB2 engine in with:
//
//	go build -tags db2 ./...
//
// and ensure the CLI driver is on the loader path (the go_ibm_db install sets
// CGO_LDFLAGS / LD_LIBRARY_PATH against its downloaded clidriver/lib).

import _ "github.com/ibmdb/go_ibm_db"

func init() {
	engineDriver["db2"] = "go_ibm_db"
}
