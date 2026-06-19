// Package dbio implements external library engines (LIBNAME backends) backed by
// relational databases through Go's database/sql. A SAS `libname` statement
// binds a libref to one of these; the DATA step and PROCs then read its tables
// as datasets, mirroring SAS/ACCESS LIBNAME-engine behavior.
//
// Read-only for now: tables are materialized into in-memory table.Dataset values
// (eager load). Streaming for large tables and write-back are future work.
package dbio

import (
	gosql "database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/solifugus/ass/table"

	// Pure-Go database/sql drivers. DB2 (CGo, github.com/ibmdb/go_ibm_db) is not
	// included here so the build stays CGO_ENABLED=0-friendly.
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
		return nil, fmt.Errorf("LIBNAME engine %q is not supported (built-in: postgres, sqlserver, oracle)", engine)
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
		return nil, false, fmt.Errorf("read %s: %w", member, err)
	}
	defer rows.Close()

	colTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, false, err
	}
	ds := table.NewDataset("", member)
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
			return nil, false, err
		}
		row := make(table.Row, len(colTypes))
		for i, ct := range colTypes {
			v := *(holders[i].(*interface{}))
			row[strings.ToLower(ct.Name())] = toValue(v, kinds[i], ct.DatabaseTypeName())
		}
		ds.AppendRow(row)
	}
	return ds, true, rows.Err()
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
