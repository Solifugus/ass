//go:build !cgo

package proc

// PROC SQL is backed by an embedded SQLite engine (github.com/mattn/go-sqlite3),
// which requires CGo. In a pure-Go build (CGO_ENABLED=0) — used for maximally
// portable, statically linked binaries (e.g. for s390x/LinuxONE across RHEL,
// SLES, and Debian) — that engine is compiled out, so PROC SQL is unavailable.
// We still register the "sql" PROC name so the program fails with a clear,
// actionable message rather than the generic "unknown PROC" error.

import (
	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/table"
)

func init() { Register("sql", sqlProc{}) }

type sqlProc struct{}

func (sqlProc) Run(lib *table.Library, step *ast.ProcStep, logger *log.Logger) error {
	logger.Error("PROC SQL is not available in this build: it requires the embedded " +
		"SQLite engine, which needs a CGo build (build with CGO_ENABLED=1).")
	return nil
}
