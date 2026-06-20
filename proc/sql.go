package proc

import (
	"fmt"
	"strings"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/sql"
	"github.com/solifugus/ass/table"
)

func init() { Register("sql", sqlProc{}) }

// sqlProc implements PROC SQL by loading the library into an in-memory SQLite
// database and running the block's statements: a bare SELECT prints its result
// listing; CREATE TABLE ... AS SELECT writes a new dataset to the library.
type sqlProc struct{}

func (sqlProc) Run(lib *table.Library, step *ast.ProcStep, logger *log.Logger) error {
	eng, err := sql.NewEngine(lib)
	if err != nil {
		logger.Error("PROC SQL: %v", err)
		return nil
	}
	defer eng.Close()

	pt := newPassthru(lib)
	defer pt.closeAll()

	for _, stmt := range splitStatements(step.RawBody) {
		// Pass-through statements (connect/disconnect/execute-by, a select drawing
		// `from connection to`, or a drop targeting an external libref) are routed
		// to the bound database rather than the in-process engine.
		if handled, err := pt.handle(stmt, logger); handled {
			if err != nil {
				logger.Error("PROC SQL: %v", err)
			}
			continue
		}
		low := strings.ToLower(stmt)
		switch {
		case strings.HasPrefix(low, "select"):
			ds, err := eng.Query(stmt)
			if err != nil {
				logger.Error("PROC SQL: %v", err)
				continue
			}
			fmt.Print(renderListing(ds, printOptions{}))

		case strings.HasPrefix(low, "create table"):
			name := createdTableName(stmt)
			// A libref-qualified target (e.g. `create table db.sorted as ...`)
			// can't be created directly in the SQLite engine (the dot reads as
			// schema.table) and may belong to an external LIBNAME engine. Build it
			// under a safe temp name, then route the result through lib.Store, which
			// writes it to the bound Backend or to WORK as appropriate.
			if dot := strings.LastIndex(name, "."); dot >= 0 {
				member := name[dot+1:]
				tmp := "_sqlct_" + member
				exec := strings.Replace(stmt, name, tmp, 1)
				if err := eng.Exec(exec); err != nil {
					logger.Error("PROC SQL: %v", err)
					continue
				}
				if err := eng.Save(lib, tmp); err != nil {
					logger.Error("PROC SQL: %v", err)
					continue
				}
				ds, ok := lib.Get(tmp)
				lib.Delete(tmp)
				if !ok {
					continue
				}
				if err := lib.Store(name, ds); err != nil {
					logger.Error("PROC SQL: %v", err)
					continue
				}
				logger.Note("Table %s.%s created, with %d rows and %d columns.",
					strings.ToUpper(ds.Lib), strings.ToUpper(ds.Name), ds.NObs(), len(ds.Columns))
				continue
			}
			if err := eng.Exec(stmt); err != nil {
				logger.Error("PROC SQL: %v", err)
				continue
			}
			if name != "" {
				if err := eng.Save(lib, name); err != nil {
					logger.Error("PROC SQL: %v", err)
					continue
				}
				if ds, ok := lib.Get(name); ok {
					logger.Note("Table WORK.%s created, with %d rows and %d columns.",
						strings.ToUpper(name), ds.NObs(), len(ds.Columns))
				}
			}

		default:
			if err := eng.Exec(stmt); err != nil {
				logger.Error("PROC SQL: %v", err)
			}
		}
	}
	return nil
}

// splitStatements splits a PROC SQL body into individual statements on
// semicolons, trimming whitespace and dropping empties. (Semicolons inside
// string literals are not handled yet — see the Phase 8 progress notes.)
func splitStatements(body string) []string {
	var out []string
	for _, s := range strings.Split(body, ";") {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// createdTableName extracts NAME from "create table NAME as select ...".
func createdTableName(stmt string) string {
	fields := strings.Fields(stmt)
	// fields: [create table NAME as ...]
	if len(fields) >= 3 && strings.EqualFold(fields[0], "create") && strings.EqualFold(fields[1], "table") {
		return strings.Trim(fields[2], "()")
	}
	return ""
}
