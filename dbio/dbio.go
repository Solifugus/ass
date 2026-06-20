// Package dbio implements external library engines (LIBNAME backends) backed by
// relational databases through Go's database/sql. A SAS `libname` statement
// binds a libref to one of these; the DATA step and PROCs then read its tables
// as datasets, mirroring SAS/ACCESS LIBNAME-engine behavior.
//
// Reads materialize a table into an in-memory table.Dataset (eager load).
// Writes (Store) are also supported, so a libref bound to a database engine can
// be a DATA-step output target (`data pg.orders; ...`), replacing the table.
// Streaming for large tables remains future work.
package dbio

import (
	gosql "database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/solifugus/ass/table"

	// Pure-Go database/sql drivers. DB2 (CGo + IBM CLI driver,
	// github.com/ibmdb/go_ibm_db) is registered only under the `db2` build tag
	// (see dbio_db2.go) so the default build needs neither CGo nor the CLI driver.
	_ "github.com/jackc/pgx/v5/stdlib"  // "pgx"
	_ "github.com/microsoft/go-mssqldb" // "sqlserver"
	_ "github.com/sijms/go-ora/v2"      // "oracle"
)

// sasEpoch is SAS day/second 0: 1960-01-01 (UTC), matching package formats.
var sasEpoch = time.Date(1960, 1, 1, 0, 0, 0, 0, time.UTC)

// engineDriver maps a SAS LIBNAME engine name to its registered database/sql
// driver. Engines whose drivers are not built in are absent (Open reports a
// clear error).
var engineDriver = map[string]string{
	"postgres":   "pgx",
	"postgresql": "pgx",
	"sqlserver":  "sqlserver",
	"mssql":      "sqlserver",
	"oracle":     "oracle",
}

// Backend is a database-backed LIBNAME engine. It implements table.Backend.
type Backend struct {
	db     *gosql.DB
	engine string
}

// Open connects to a database for the given SAS engine name and connection
// string and returns a read-only LIBNAME backend.
func Open(engine, connection string) (*Backend, error) {
	driver, ok := engineDriver[strings.ToLower(engine)]
	if !ok {
		return nil, fmt.Errorf("LIBNAME engine %q is not supported (built-in: postgres, sqlserver, oracle; sqlite when built with cgo; db2 when built with -tags db2)", engine)
	}
	db, err := gosql.Open(driver, connection)
	if err != nil {
		return nil, fmt.Errorf("connect (%s): %w", engine, err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect (%s): %w", engine, err)
	}
	return &Backend{db: db, engine: strings.ToLower(engine)}, nil
}

// Close releases the database connection pool.
func (b *Backend) Close() error { return b.db.Close() }

// Load materializes a table (member) as a dataset via `SELECT * FROM <member>`.
// ok is false when the member does not exist (or the query fails in a way that
// indicates absence is left to the caller — here any error is returned).
func (b *Backend) Load(member string) (*table.Dataset, bool, error) {
	if !safeIdent(member) {
		return nil, false, fmt.Errorf("invalid table name %q", member)
	}
	rows, err := b.db.Query("SELECT * FROM " + quoteIdent(b.engine, member))
	if err != nil {
		if isMissingTableErr(err) {
			return nil, false, nil // member absent: report not-found, not an error
		}
		return nil, false, fmt.Errorf("read %s: %w", member, err)
	}
	defer rows.Close()

	ds, err := scanResult(rows, member)
	if err != nil {
		return nil, false, err
	}
	return ds, true, nil
}

// LoadFiltered materializes a member like Load, but pushes a column projection
// and/or row filter into the SELECT so the database returns less data (the
// table.FilterBackend optimization for dataset-option `keep=`/`where=`). It is
// strictly value-safe: the projection is mapped to the table's actual column
// names (so case differences across engines don't matter) and bails to SELECT *
// if any requested column is absent; the filter is only emitted for columns the
// table reports as numeric, using operators (=, >, >=) whose SQL row selection
// matches SAS exactly. Anything not safely translatable is simply not pushed —
// the caller re-applies the full dataset options locally regardless.
func (b *Backend) LoadFiltered(member string, sel table.Selection) (*table.Dataset, bool, error) {
	if !safeIdent(member) {
		return nil, false, fmt.Errorf("invalid table name %q", member)
	}
	qname := quoteIdent(b.engine, member)

	// Fetch the table's real column names + kinds to map the projection and to
	// validate the filter. If this metadata probe fails (e.g. table missing), fall
	// back to a plain SELECT *; the main query below reports not-found/errors.
	meta, metaErr := b.columnMeta(member)

	colList := "*"
	if metaErr == nil && len(sel.Columns) > 0 {
		if names, ok := mapColumns(sel.Columns, meta); ok {
			parts := make([]string, len(names))
			for i, n := range names {
				parts[i] = quoteIdent(b.engine, n)
			}
			colList = strings.Join(parts, ", ")
		}
	}

	where := ""
	if metaErr == nil && sel.Filter != nil {
		byName := map[string]colMeta{}
		for _, m := range meta {
			byName[strings.ToLower(m.Name)] = m
		}
		if sql, ok := b.renderFilter(sel.Filter, byName); ok {
			where = " WHERE " + sql
		}
	}

	rows, err := b.db.Query("SELECT " + colList + " FROM " + qname + where)
	if err != nil {
		if isMissingTableErr(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read %s: %w", member, err)
	}
	defer rows.Close()
	ds, err := scanResult(rows, member)
	if err != nil {
		return nil, false, err
	}
	return ds, true, nil
}

// colMeta is a table column's name (in the database's own case) and inferred SAS
// kind, used to map a projection and validate a pushed filter.
type colMeta struct {
	Name string
	Kind table.Kind
}

// columnMeta probes a member's columns without fetching rows (SELECT * ...
// WHERE 1=0), returning each column's real name and inferred SAS kind.
func (b *Backend) columnMeta(member string) ([]colMeta, error) {
	rows, err := b.db.Query("SELECT * FROM " + quoteIdent(b.engine, member) + " WHERE 1=0")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cts, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	out := make([]colMeta, len(cts))
	for i, ct := range cts {
		out[i] = colMeta{Name: ct.Name(), Kind: sasKind(ct.DatabaseTypeName())}
	}
	return out, nil
}

// mapColumns resolves the requested (case-insensitive) projection names to the
// table's actual column names. ok is false if any requested column is not present
// (so the caller falls back to SELECT * — local KEEP/DROP still does the right
// thing).
func mapColumns(want []string, meta []colMeta) (names []string, ok bool) {
	byName := map[string]string{}
	for _, m := range meta {
		byName[strings.ToLower(m.Name)] = m.Name
	}
	names = make([]string, 0, len(want))
	for _, w := range want {
		actual, found := byName[strings.ToLower(w)]
		if !found {
			return nil, false
		}
		names = append(names, actual)
	}
	return names, true
}

// renderFilter turns a dialect-neutral Filter into this engine's SQL. ok is false
// if the filter references any column the table does not report as numeric (the
// whole filter is then dropped rather than risk a type-coerced, value-divergent
// comparison) — the caller filters locally instead.
func (b *Backend) renderFilter(f *table.Filter, byName map[string]colMeta) (string, bool) {
	switch f.Kind {
	case table.FilterAnd, table.FilterOr:
		parts := make([]string, 0, len(f.Sub))
		for _, sub := range f.Sub {
			s, ok := b.renderFilter(sub, byName)
			if !ok {
				return "", false
			}
			parts = append(parts, s)
		}
		if len(parts) == 0 {
			return "", false
		}
		joiner := " AND "
		if f.Kind == table.FilterOr {
			joiner = " OR "
		}
		return "(" + strings.Join(parts, joiner) + ")", true
	case table.FilterCmp:
		m, found := byName[strings.ToLower(f.Column)]
		if !found || m.Kind != table.Numeric {
			return "", false
		}
		lit := strconv.FormatFloat(f.Number, 'g', -1, 64)
		return quoteIdent(b.engine, m.Name) + " " + f.Op + " " + lit, true
	}
	return "", false
}

// QuerySQL runs a native (dialect-specific) query against the database and
// returns the result set as a dataset — the PROC SQL pass-through read path
// (`select ... from connection to <engine> (<native query>)`). The SQL is passed
// through verbatim; column kinds and date/datetime formats are inferred from the
// driver's reported types exactly as Load does.
func (b *Backend) QuerySQL(query string) (*table.Dataset, error) {
	rows, err := b.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("pass-through query: %w", err)
	}
	defer rows.Close()
	return scanResult(rows, "_passthru_")
}

// ExecSQL runs a native no-result statement (DDL/DML) against the database — the
// PROC SQL pass-through write path (`execute (<native sql>) by <engine>`). The
// SQL is passed through verbatim.
func (b *Backend) ExecSQL(query string) error {
	if _, err := b.db.Exec(query); err != nil {
		return fmt.Errorf("pass-through execute: %w", err)
	}
	return nil
}

// scanResult builds a dataset from an open *sql.Rows, inferring SAS column kinds
// and date/datetime formats from the driver's reported column types. Shared by
// Load and QuerySQL.
func scanResult(rows *gosql.Rows, name string) (*table.Dataset, error) {
	colTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	ds := table.NewDataset("", name)
	kinds := make([]table.Kind, len(colTypes))
	for i, ct := range colTypes {
		kinds[i] = sasKind(ct.DatabaseTypeName())
		col := table.Column{Name: ct.Name(), Kind: kinds[i]}
		if isDateType(ct.DatabaseTypeName()) {
			col.Format = "date9."
		} else if isTimestampType(ct.DatabaseTypeName()) {
			col.Format = "datetime."
		}
		ds.AddColumn(col)
	}

	for rows.Next() {
		holders := make([]interface{}, len(colTypes))
		for i := range holders {
			holders[i] = new(interface{})
		}
		if err := rows.Scan(holders...); err != nil {
			return nil, err
		}
		row := make(table.Row, len(colTypes))
		for i, ct := range colTypes {
			v := *(holders[i].(*interface{}))
			row[strings.ToLower(ct.Name())] = toValue(v, kinds[i], ct.DatabaseTypeName())
		}
		ds.AppendRow(row)
	}
	return ds, rows.Err()
}

// Store writes a dataset to the database as a table (LIBNAME engine as a
// DATA-step / PROC output target). It replaces any existing table of the same
// name: the whole operation — DROP, CREATE, and all INSERTs — runs in a single
// transaction so a failure leaves the prior table intact. SAS numeric columns
// map to the engine's double type (date/datetime-formatted numerics map to the
// engine's DATE/TIMESTAMP type and are converted from SAS day/second numbers);
// character columns map to a text type sized by the column length. Missing
// values become SQL NULL.
func (b *Backend) Store(ds *table.Dataset) error {
	if !safeIdent(ds.Name) {
		return fmt.Errorf("invalid table name %q", ds.Name)
	}
	qname := quoteIdent(b.engine, ds.Name)

	tx, err := b.db.Begin()
	if err != nil {
		return fmt.Errorf("write %s: %w", ds.Name, err)
	}
	defer tx.Rollback() // no-op after a successful Commit

	if err := b.dropIfExists(tx, qname); err != nil {
		return fmt.Errorf("replace %s: %w", ds.Name, err)
	}
	if _, err := tx.Exec(b.createTableSQL(qname, ds.Columns)); err != nil {
		return fmt.Errorf("create %s: %w", ds.Name, err)
	}
	if err := b.insertRows(tx, qname, ds); err != nil {
		return err
	}
	return tx.Commit()
}

// Append inserts the dataset's rows into an existing table without dropping or
// recreating it — SAS PROC APPEND / `mod`-style incremental load. Like Store the
// inserts run in a single transaction (a failure leaves the table unchanged) and
// values map through storeArg identically. The table must already exist; PROC
// APPEND creates a missing BASE= via Store before reaching this path.
func (b *Backend) Append(ds *table.Dataset) error {
	if !safeIdent(ds.Name) {
		return fmt.Errorf("invalid table name %q", ds.Name)
	}
	qname := quoteIdent(b.engine, ds.Name)

	tx, err := b.db.Begin()
	if err != nil {
		return fmt.Errorf("append %s: %w", ds.Name, err)
	}
	defer tx.Rollback() // no-op after a successful Commit

	if err := b.insertRows(tx, qname, ds); err != nil {
		return err
	}
	return tx.Commit()
}

// dropIfExists drops a table if it is present, swallowing a "table does not
// exist" error. Most engines support `DROP TABLE IF EXISTS`, but DB2 (and older
// Oracle) do not, so for those a plain DROP is issued and a missing-table error
// is treated as success. Used by Store's replace step.
func (b *Backend) dropIfExists(tx *gosql.Tx, qname string) error {
	switch b.engine {
	case "db2":
		if _, err := tx.Exec("DROP TABLE " + qname); err != nil {
			if isMissingTableErr(err) {
				return nil
			}
			return err
		}
		return nil
	default:
		_, err := tx.Exec("DROP TABLE IF EXISTS " + qname)
		return err
	}
}

// Drop removes a table (member) from the database — the engine path for PROC SQL
// `drop table <libref>.<member>`. A missing table is reported as an error by the
// driver, matching SQL's DROP semantics.
func (b *Backend) Drop(member string) error {
	if !safeIdent(member) {
		return fmt.Errorf("invalid table name %q", member)
	}
	if _, err := b.db.Exec("DROP TABLE " + quoteIdent(b.engine, member)); err != nil {
		return fmt.Errorf("drop %s: %w", member, err)
	}
	return nil
}

// insertRows prepares one INSERT and executes it for every row, mapping SAS
// values to bound args via storeArg. Shared by Store and Append.
func (b *Backend) insertRows(tx *gosql.Tx, qname string, ds *table.Dataset) error {
	cols := make([]string, len(ds.Columns))
	for i, c := range ds.Columns {
		cols[i] = quoteIdent(b.engine, c.Name)
	}
	insert := "INSERT INTO " + qname + " (" + strings.Join(cols, ", ") + ") VALUES (" +
		b.placeholders(len(ds.Columns)) + ")"
	stmt, err := tx.Prepare(insert)
	if err != nil {
		return fmt.Errorf("prepare insert into %s: %w", ds.Name, err)
	}
	defer stmt.Close()

	args := make([]interface{}, len(ds.Columns))
	for _, row := range ds.Rows {
		for i, c := range ds.Columns {
			args[i] = storeArg(ds.Get(row, c.Name), c)
		}
		if _, err := stmt.Exec(args...); err != nil {
			return fmt.Errorf("insert into %s: %w", ds.Name, err)
		}
	}
	return nil
}

// createTableSQL builds a CREATE TABLE statement mapping each SAS column to an
// engine-appropriate column type.
func (b *Backend) createTableSQL(qname string, cols []table.Column) string {
	defs := make([]string, len(cols))
	for i, c := range cols {
		defs[i] = quoteIdent(b.engine, c.Name) + " " + b.columnType(c)
	}
	return "CREATE TABLE " + qname + " (" + strings.Join(defs, ", ") + ")"
}

// columnType maps a SAS column to the engine's SQL type. Character columns use a
// variable-width text type sized by the column's length (falling back to a wide
// default); date/datetime-formatted numerics use the engine's date/timestamp
// type; all other numerics use the engine's double-precision float type.
func (b *Backend) columnType(c table.Column) string {
	if c.Kind == table.Character {
		n := c.Length
		if n <= 0 {
			n = 255
		}
		switch b.engine {
		case "sqlserver", "mssql":
			return fmt.Sprintf("NVARCHAR(%d)", n)
		case "oracle":
			return fmt.Sprintf("VARCHAR2(%d)", n)
		default: // postgres, sqlite
			return fmt.Sprintf("VARCHAR(%d)", n)
		}
	}
	isDate, isDatetime := temporalFormat(c.Format)
	if isDate {
		return "DATE"
	}
	if isDatetime {
		switch b.engine {
		case "sqlserver", "mssql":
			return "DATETIME2"
		default: // postgres, oracle, sqlite
			return "TIMESTAMP"
		}
	}
	switch b.engine {
	case "sqlserver", "mssql":
		return "FLOAT"
	case "oracle":
		return "BINARY_DOUBLE"
	case "db2":
		return "DOUBLE"
	case "sqlite", "sqlite3":
		return "REAL"
	default: // postgres
		return "DOUBLE PRECISION"
	}
}

// placeholders returns a comma-separated list of n bind placeholders in the
// engine's dialect (Postgres/Oracle are positional; SQL Server names them; others
// use ?).
func (b *Backend) placeholders(n int) string {
	parts := make([]string, n)
	for i := 0; i < n; i++ {
		switch b.engine {
		case "postgres", "postgresql":
			parts[i] = fmt.Sprintf("$%d", i+1)
		case "oracle":
			parts[i] = fmt.Sprintf(":%d", i+1)
		case "sqlserver", "mssql":
			parts[i] = fmt.Sprintf("@p%d", i+1)
		default: // sqlite and any ?-style driver
			parts[i] = "?"
		}
	}
	return strings.Join(parts, ", ")
}

// storeArg converts a SAS value to the Go value bound into an INSERT, honoring
// the column's type and (for numerics) any date/datetime format. Missing values
// become nil (SQL NULL).
func storeArg(v table.Value, c table.Column) interface{} {
	if v.IsMissing() {
		return nil
	}
	if c.Kind == table.Character {
		return v.Str
	}
	if isDate, isDatetime := temporalFormat(c.Format); isDate {
		return sasEpoch.AddDate(0, 0, int(v.Num))
	} else if isDatetime {
		return sasEpoch.Add(time.Duration(v.Num * float64(time.Second)))
	}
	return v.Num
}

// temporalFormat classifies a SAS format name as a date or datetime format (so a
// numeric column storing SAS day/second counts can be written to a real SQL
// DATE/TIMESTAMP column). Width/decimal suffixes are ignored.
func temporalFormat(format string) (isDate, isDatetime bool) {
	f := strings.ToLower(strings.TrimSpace(format))
	f = strings.TrimRight(f, "0123456789.")
	switch f {
	case "datetime":
		return false, true
	case "date", "mmddyy", "ddmmyy", "yymmdd", "worddate", "weekdate", "monyy", "yymmddd":
		return true, false
	}
	return false, false
}

// toValue converts a scanned database value to a SAS table.Value, honoring the
// column's inferred kind. NULL becomes the typed missing value.
func toValue(v interface{}, kind table.Kind, dbType string) table.Value {
	if v == nil {
		if kind == table.Character {
			return table.MissingChar()
		}
		return table.MissingNum()
	}
	switch x := v.(type) {
	case bool:
		if x {
			return table.Num(1)
		}
		return table.Num(0)
	case int64:
		return table.Num(float64(x))
	case float64:
		return table.Num(x)
	case time.Time:
		if isDateType(dbType) {
			return table.Num(float64(int(x.Sub(sasEpoch).Hours()) / 24))
		}
		return table.Num(x.Sub(sasEpoch).Seconds())
	case []byte:
		s := string(x)
		if kind == table.Numeric {
			return numericFromString(s)
		}
		return table.Char(s)
	case string:
		if kind == table.Numeric {
			return numericFromString(x)
		}
		return table.Char(x)
	default:
		// Fall back to the default string rendering.
		return table.Char(fmt.Sprintf("%v", x))
	}
}

// numericFromString parses a numeric value delivered as text (some drivers return
// DECIMAL/NUMERIC as strings to preserve precision).
func numericFromString(s string) table.Value {
	s = strings.TrimSpace(s)
	if s == "" {
		return table.MissingNum()
	}
	var f float64
	if _, err := fmt.Sscanf(s, "%g", &f); err != nil {
		return table.MissingNum()
	}
	return table.Num(f)
}

// sasKind maps a database column type name to a SAS kind. Anything textual is
// Character; everything else (numbers, dates as numeric SAS dates, booleans) is
// Numeric.
func sasKind(dbType string) table.Kind {
	if isTextType(dbType) {
		return table.Character
	}
	return table.Numeric
}

func upper(s string) string { return strings.ToUpper(s) }

func isTextType(dbType string) bool {
	t := upper(dbType)
	for _, s := range []string{"CHAR", "TEXT", "CLOB", "STRING", "NVARCHAR", "VARCHAR", "NCHAR", "UUID", "JSON", "XML"} {
		if strings.Contains(t, s) {
			return true
		}
	}
	return false
}

func isDateType(dbType string) bool {
	t := upper(dbType)
	return strings.Contains(t, "DATE") && !strings.Contains(t, "DATETIME") && !strings.Contains(t, "TIMESTAMP")
}

func isTimestampType(dbType string) bool {
	t := upper(dbType)
	return strings.Contains(t, "TIMESTAMP") || strings.Contains(t, "DATETIME")
}

// isMissingTableErr reports whether a query error means the table does not exist
// (as opposed to a connection or syntax failure), so a Load of an absent member
// can report not-found rather than an error. The messages are matched across the
// built-in engines: SQLite ("no such table"), Postgres ("does not exist"), SQL
// Server ("Invalid object name"), Oracle ("ORA-00942"), and DB2 ("SQL0204N" /
// SQLSTATE 42704 "undefined name").
func isMissingTableErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, s := range []string{"no such table", "does not exist", "invalid object name", "ora-00942", "sql0204n", "42704", "undefined name"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// safeIdent guards the table name interpolated into SELECT (driver placeholders
// can't parameterize identifiers). Allows letters, digits, underscore, and a
// single schema-qualifying dot.
func safeIdent(name string) bool {
	if name == "" {
		return false
	}
	dots := 0
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
		case r == '.':
			dots++
		default:
			return false
		}
	}
	return dots <= 1
}

// quoteIdent quotes an identifier (and its optional schema qualifier) using the
// engine's quoting convention.
func quoteIdent(engine, name string) string {
	open, close := `"`, `"`
	if engine == "sqlserver" || engine == "mssql" {
		open, close = "[", "]"
	}
	parts := strings.Split(name, ".")
	for i, p := range parts {
		parts[i] = open + p + close
	}
	return strings.Join(parts, ".")
}
