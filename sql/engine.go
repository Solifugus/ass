package sql

import (
	gosql "database/sql"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3" // CGo SQLite driver registered as "sqlite3"
	"github.com/solifugus/ass/table"
)

// Engine is an in-memory SQLite database loaded with the datasets of a library,
// used to execute PROC SQL. SAS dataset and variable names are case-insensitive;
// identifiers are left unquoted so SQLite's own case-insensitive matching
// applies.
type Engine struct {
	db *gosql.DB
}

// NewEngine opens an in-memory SQLite database and loads every dataset in the
// library into a table of the same name.
func NewEngine(lib *table.Library) (*Engine, error) {
	db, err := gosql.Open("sqlite3", ":memory:")
	if err != nil {
		return nil, err
	}
	e := &Engine{db: db}
	for _, key := range lib.Names() {
		ds, ok := lib.Get(key)
		if !ok {
			continue
		}
		if err := e.load(ds); err != nil {
			e.Close()
			return nil, fmt.Errorf("loading %s: %w", ds.Name, err)
		}
	}
	return e, nil
}

// Close releases the database.
func (e *Engine) Close() error { return e.db.Close() }

// Exec runs a statement that returns no rows (CREATE TABLE, INSERT, ...).
func (e *Engine) Exec(query string) error {
	_, err := e.db.Exec(query)
	return err
}

// AddTable loads a dataset into the engine under an explicit table name,
// independent of ds.Name. PROC SQL uses it to bring source tables (members of an
// external LIBNAME engine) into the in-process database on demand.
func (e *Engine) AddTable(name string, ds *table.Dataset) error { return e.loadAs(name, ds) }

// load creates and populates a SQLite table named after the dataset.
func (e *Engine) load(ds *table.Dataset) error { return e.loadAs(ds.Name, ds) }

// loadAs creates and populates a SQLite table with the given name from a dataset.
// Numeric columns map to REAL and character columns to TEXT; a numeric missing
// becomes SQL NULL.
func (e *Engine) loadAs(name string, ds *table.Dataset) error {
	if len(ds.Columns) == 0 {
		return nil
	}
	defs := make([]string, len(ds.Columns))
	for i, c := range ds.Columns {
		sqlType := "REAL"
		if c.Kind == table.Character {
			sqlType = "TEXT"
		}
		defs[i] = c.Name + " " + sqlType
	}
	if _, err := e.db.Exec(fmt.Sprintf("CREATE TABLE %s (%s)", name, strings.Join(defs, ", "))); err != nil {
		return err
	}

	placeholders := strings.TrimRight(strings.Repeat("?, ", len(ds.Columns)), ", ")
	insert := fmt.Sprintf("INSERT INTO %s VALUES (%s)", name, placeholders)
	for _, r := range ds.Rows {
		args := make([]any, len(ds.Columns))
		for i, c := range ds.Columns {
			v := ds.Get(r, c.Name)
			switch {
			case v.IsMissing() && c.Kind != table.Character:
				args[i] = nil
			case c.Kind == table.Character:
				args[i] = v.Str
			default:
				args[i] = v.Num
			}
		}
		if _, err := e.db.Exec(insert, args...); err != nil {
			return err
		}
	}
	return nil
}

// Query runs a SELECT and returns the result as a dataset. Column types are
// inferred from the returned values (a string/[]byte value makes the column
// character; otherwise it is numeric), since SQLite types are dynamic.
func (e *Engine) Query(query string) (*table.Dataset, error) {
	rows, err := e.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	colNames, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	n := len(colNames)

	var data [][]any
	for rows.Next() {
		cells := make([]any, n)
		ptrs := make([]any, n)
		for i := range cells {
			ptrs[i] = &cells[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		data = append(data, cells)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	kinds := make([]table.Kind, n) // default Numeric
	for j := 0; j < n; j++ {
		for _, row := range data {
			if _, isStr := row[j].(string); isStr {
				kinds[j] = table.Character
				break
			}
			if _, isBytes := row[j].([]byte); isBytes {
				kinds[j] = table.Character
				break
			}
		}
	}

	ds := table.NewDataset("", "_sql_result_")
	for j, name := range colNames {
		ds.AddColumn(table.Column{Name: name, Kind: kinds[j]})
	}
	for _, row := range data {
		r := make(table.Row, n)
		for j, name := range colNames {
			r[strings.ToLower(name)] = toValue(row[j], kinds[j])
		}
		ds.AppendRow(r)
	}
	return ds, nil
}

// Save reads a SQLite table back into the library under the given name (used
// after CREATE TABLE ... AS SELECT).
func (e *Engine) Save(lib *table.Library, name string) error {
	ds, err := e.Query("SELECT * FROM " + name)
	if err != nil {
		return err
	}
	ds.Name = name
	lib.Put(ds)
	return nil
}

// toValue converts a scanned SQLite value to a table.Value of the given kind.
func toValue(x any, kind table.Kind) table.Value {
	switch v := x.(type) {
	case nil:
		if kind == table.Character {
			return table.MissingChar()
		}
		return table.MissingNum()
	case int64:
		return table.Num(float64(v))
	case float64:
		return table.Num(v)
	case bool:
		if v {
			return table.Num(1)
		}
		return table.Num(0)
	case string:
		return table.Char(v)
	case []byte:
		return table.Char(string(v))
	default:
		return table.Char(fmt.Sprint(v))
	}
}
