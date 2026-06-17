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

	for _, stmt := range splitStatements(step.RawBody) {
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
			if err := eng.Exec(stmt); err != nil {
				logger.Error("PROC SQL: %v", err)
				continue
			}
			name := createdTableName(stmt)
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
