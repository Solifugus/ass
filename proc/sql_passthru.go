//go:build cgo

package proc

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/solifugus/ass/dbio"
	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/table"
)

// PROC SQL pass-through. SAS lets a PROC SQL block reach an external database
// directly, sending that database its own native SQL rather than the in-process
// engine's:
//
//	proc sql;
//	  connect to oracle (connection="oracle://user:pw@host:1521/svc");
//	  create table work.s as
//	    select * from connection to oracle
//	      (select region, count(*) n from sales group by region);
//	  execute (drop table "old") by oracle;
//	  disconnect from oracle;
//	quit;
//
// ASS supports this against any database LIBNAME engine (Postgres/SQL Server/
// Oracle/SQLite). A connection is named either by `connect to <engine> [as
// <alias>] (connection="<conn-string>")` — opening a fresh connection for the
// block — or, more simply, by referring to an already-assigned libref, in which
// case no `connect` is needed: `select ... from connection to <libref> (...)`
// and `execute (...) by <libref>` reuse that libref's connection.
//
// Pass-through SQL is the database's own dialect, sent verbatim; ASS does not
// parse or rewrite it. The result set of a `from connection to` query is brought
// back as a dataset (printed, or saved by a wrapping `create table ... as`).

var (
	connectRe    = regexp.MustCompile(`(?is)^connect\s+to\s+(\w+)(?:\s+as\s+(\w+))?\s*\((.*)\)\s*$`)
	disconnectRe = regexp.MustCompile(`(?is)^disconnect\s+from\s+(\w+)\s*$`)
	executeRe    = regexp.MustCompile(`(?is)^execute\s*\((.*)\)\s*by\s+(\w+)\s*$`)
	fromConnRe   = regexp.MustCompile(`(?is)from\s+connection\s+to\s+(\w+)\s*\((.*)\)\s*$`)
	createAsRe   = regexp.MustCompile(`(?is)^create\s+table\s+(\S+)\s+as\s+`)
	dropTableRe  = regexp.MustCompile(`(?is)^drop\s+table\s+(?:if\s+exists\s+)?([\w.]+)\s*$`)
	connStrRe    = regexp.MustCompile(`(?is)\b(?:connection|dsn|connect)\s*=\s*(?:"([^"]*)"|'([^']*)'|(\S+))`)
)

// passthru tracks the database connections a PROC SQL block opens via `connect
// to`, so they can be reused across statements and closed when the block ends.
type passthru struct {
	lib   *table.Library
	conns map[string]table.SQLBackend // alias (uppercased) -> backend
	owned map[string]*dbio.Backend    // aliases this block opened (and must Close)
}

func newPassthru(lib *table.Library) *passthru {
	return &passthru{
		lib:   lib,
		conns: map[string]table.SQLBackend{},
		owned: map[string]*dbio.Backend{},
	}
}

// closeAll closes every connection this block opened (connections borrowed from
// an assigned libref are owned by the LIBNAME and left open).
func (p *passthru) closeAll() {
	for _, be := range p.owned {
		be.Close()
	}
}

// resolve returns the SQL backend for a connection name: a connection opened by
// `connect to ... as <name>` in this block, or — failing that — an already
// assigned libref of that name.
func (p *passthru) resolve(name string) (table.SQLBackend, error) {
	if b, ok := p.conns[strings.ToUpper(name)]; ok {
		return b, nil
	}
	if b, ok := p.lib.Backend(name); ok {
		if sb, ok := b.(table.SQLBackend); ok {
			return sb, nil
		}
		return nil, fmt.Errorf("library %s does not support pass-through SQL", strings.ToUpper(name))
	}
	return nil, fmt.Errorf("no connection or libref named %q (use `connect to`, or assign it with libname)", name)
}

// handle dispatches a single PROC SQL statement to the pass-through path when it
// is one (connect / disconnect / execute-by / a select drawing `from connection
// to`, or a `drop table` targeting an external libref). It returns handled=false
// for any other statement so the caller runs it on the in-process engine.
func (p *passthru) handle(stmt string, logger *log.Logger) (handled bool, err error) {
	switch {
	case connectRe.MatchString(stmt):
		return true, p.connect(stmt, logger)
	case disconnectRe.MatchString(stmt):
		return true, p.disconnect(stmt, logger)
	case executeRe.MatchString(stmt):
		return true, p.execute(stmt, logger)
	case strings.Contains(strings.ToLower(stmt), "connection to"):
		return true, p.fromConnection(stmt, logger)
	case dropTableRe.MatchString(stmt):
		// Only intercept drops of an external libref's table; a WORK/embedded
		// drop falls through to the in-process engine.
		m := dropTableRe.FindStringSubmatch(stmt)
		if !p.lib.IsExternal(m[1]) {
			return false, nil
		}
		ok, derr := p.lib.DropExternal(m[1])
		if ok && derr == nil {
			logger.Note("Table %s has been dropped.", strings.ToUpper(m[1]))
		}
		return ok, derr
	}
	return false, nil
}

func (p *passthru) connect(stmt string, logger *log.Logger) error {
	m := connectRe.FindStringSubmatch(stmt)
	engine, alias, opts := m[1], m[2], m[3]
	if alias == "" {
		alias = engine
	}
	conn := connectionString(opts)
	if conn == "" {
		return fmt.Errorf("connect to %s: a connection string is required, e.g. connection=\"...\"", engine)
	}
	be, err := dbio.Open(engine, conn)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", engine, err)
	}
	key := strings.ToUpper(alias)
	if old, ok := p.owned[key]; ok {
		old.Close() // a re-connect under the same alias replaces the old one
	}
	p.conns[key] = be
	p.owned[key] = be
	logger.Note("Connection %s established (engine: %s).", key, strings.ToLower(engine))
	return nil
}

func (p *passthru) disconnect(stmt string, logger *log.Logger) error {
	m := disconnectRe.FindStringSubmatch(stmt)
	key := strings.ToUpper(m[1])
	if be, ok := p.owned[key]; ok {
		be.Close()
		delete(p.owned, key)
		delete(p.conns, key)
		logger.Note("Connection %s closed.", key)
	}
	return nil
}

func (p *passthru) execute(stmt string, logger *log.Logger) error {
	m := executeRe.FindStringSubmatch(stmt)
	native, alias := strings.TrimSpace(m[1]), m[2]
	sb, err := p.resolve(alias)
	if err != nil {
		return err
	}
	return sb.ExecSQL(native)
}

func (p *passthru) fromConnection(stmt string, logger *log.Logger) error {
	fc := fromConnRe.FindStringSubmatch(stmt)
	if fc == nil {
		return fmt.Errorf("malformed `from connection to` query")
	}
	alias, native := fc[1], strings.TrimSpace(fc[2])
	sb, err := p.resolve(alias)
	if err != nil {
		return err
	}
	ds, err := sb.QuerySQL(native)
	if err != nil {
		return err
	}
	// A wrapping `create table <tgt> as select ... from connection to ...` saves
	// the result (routed through lib.Store, so a libref-qualified target lands in
	// the right library); a bare select prints its listing.
	if cm := createAsRe.FindStringSubmatch(stmt); cm != nil {
		target := strings.Trim(cm[1], "()")
		if err := p.lib.Store(target, ds); err != nil {
			return err
		}
		logger.Note("Table %s.%s created, with %d rows and %d columns.",
			strings.ToUpper(ds.Lib), strings.ToUpper(ds.Name), ds.NObs(), len(ds.Columns))
		return nil
	}
	fmt.Print(renderListing(ds, printOptions{}))
	return nil
}

// connectionString pulls the connection string out of a `connect to` option list.
// ASS accepts `connection="..."` (a LIBNAME-style connection string), with `dsn=`
// and `connect=` as synonyms; the value may be double-quoted, single-quoted, or
// bare.
func connectionString(opts string) string {
	m := connStrRe.FindStringSubmatch(opts)
	if m == nil {
		return ""
	}
	for _, g := range m[1:] {
		if g != "" {
			return g
		}
	}
	return ""
}
