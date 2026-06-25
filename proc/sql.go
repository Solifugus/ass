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

	// Tracks external source members already loaded into the engine this block,
	// keyed by their private alias, so a table referenced by several statements is
	// loaded once.
	loaded := map[string]bool{}

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
			q, err := rewriteSources(stmt, lib, eng, loaded)
			if err != nil {
				logger.Error("PROC SQL: %v", err)
				continue
			}
			ds, err := eng.Query(q)
			if err != nil {
				logger.Error("PROC SQL: %v", err)
				continue
			}
			emitListing(logger, ds, printOptions{}, "Query Result")

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
				exec, err := rewriteSources(exec, lib, eng, loaded)
				if err != nil {
					logger.Error("PROC SQL: %v", err)
					continue
				}
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
			exec, err := rewriteSources(stmt, lib, eng, loaded)
			if err != nil {
				logger.Error("PROC SQL: %v", err)
				continue
			}
			if err := eng.Exec(exec); err != nil {
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

// rewriteSources makes qualified table references in a SQL statement resolvable
// by the in-process SQLite engine. A WORK-qualified name (`work.x`) is reduced to
// its member (WORK datasets are loaded by member name); a name qualified with an
// external LIBNAME engine (`pg.orders`) is loaded from that backend into the
// engine under a private alias and rewritten to it. References whose qualifier is
// neither WORK nor an assigned libref are left untouched — those are table-alias
// column qualifiers (`t.col`), not tables. Single-quoted string literals are
// copied verbatim so qualifier-looking text inside them is never rewritten.
func rewriteSources(stmt string, lib *table.Library, eng *sql.Engine, loaded map[string]bool) (string, error) {
	var b strings.Builder
	var firstErr error
	i, n := 0, len(stmt)
	for i < n {
		c := stmt[i]
		if c == '\'' { // copy a string literal verbatim ('' is an escaped quote)
			j := i + 1
			for j < n {
				if stmt[j] == '\'' {
					if j+1 < n && stmt[j+1] == '\'' {
						j += 2
						continue
					}
					j++
					break
				}
				j++
			}
			b.WriteString(stmt[i:j])
			i = j
			continue
		}
		if isIdentStart(c) {
			j := i
			for j < n && isIdentPart(stmt[j]) {
				j++
			}
			word := stmt[i:j]
			if j < n && stmt[j] == '.' && j+1 < n && isIdentStart(stmt[j+1]) {
				k := j + 1
				for k < n && isIdentPart(stmt[k]) {
					k++
				}
				member := stmt[j+1 : k]
				if repl, ok, err := resolveQualified(word, member, lib, eng, loaded); ok {
					b.WriteString(repl)
					i = k
					continue
				} else if err != nil && firstErr == nil {
					firstErr = err
				}
			}
			b.WriteString(word)
			i = j
			continue
		}
		b.WriteByte(c)
		i++
	}
	return b.String(), firstErr
}

// resolveQualified maps a libref.member reference to the table name the engine
// should use. ok is false (with the reference left as-is) when libref is not a
// known library, so alias.column qualifiers pass through unchanged.
func resolveQualified(libref, member string, lib *table.Library, eng *sql.Engine, loaded map[string]bool) (repl string, ok bool, err error) {
	ref := strings.ToUpper(libref)
	if ref == "WORK" {
		return member, true, nil
	}
	if _, isBackend := lib.Backend(ref); !isBackend {
		return "", false, nil
	}
	alias := "_extsrc_" + strings.ToLower(ref) + "_" + strings.ToLower(member)
	if loaded[alias] {
		return alias, true, nil
	}
	ds, found, err := lib.Resolve(libref + "." + member)
	if err != nil {
		return "", false, fmt.Errorf("loading %s.%s: %w", libref, member, err)
	}
	if !found {
		return "", false, fmt.Errorf("table %s.%s not found", libref, member)
	}
	if err := eng.AddTable(alias, ds); err != nil {
		return "", false, fmt.Errorf("loading %s.%s: %w", libref, member, err)
	}
	loaded[alias] = true
	return alias, true, nil
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentPart(c byte) bool { return isIdentStart(c) || (c >= '0' && c <= '9') }

// createdTableName extracts NAME from "create table NAME as select ...".
func createdTableName(stmt string) string {
	fields := strings.Fields(stmt)
	// fields: [create table NAME as ...]
	if len(fields) >= 3 && strings.EqualFold(fields[0], "create") && strings.EqualFold(fields[1], "table") {
		return strings.Trim(fields[2], "()")
	}
	return ""
}
