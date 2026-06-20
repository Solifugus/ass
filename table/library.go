package table

import (
	"fmt"
	"strings"
)

// Backend is an external library engine (e.g. a relational database) assigned to
// a libref via a LIBNAME statement. Load materializes one member (table) as a
// dataset; ok is false when the member does not exist.
type Backend interface {
	Load(member string) (ds *Dataset, ok bool, err error)
}

// WriteBackend is a Backend that can also receive datasets — a LIBNAME engine
// usable as a DATA-step (or PROC) output target, e.g. `data pg.orders; ...`.
// Store replaces any existing member of the same name. A Backend is writable
// only if it implements this; read-only engines simply do not.
type WriteBackend interface {
	Backend
	Store(ds *Dataset) error
}

// AppendBackend is a WriteBackend that can also add rows to an existing member
// without replacing it (an INSERT-only / `mod`-style write), the engine path for
// PROC APPEND's incremental load. A WriteBackend that does not implement it is
// appended via load-combine-replace through Store instead.
type AppendBackend interface {
	WriteBackend
	Append(ds *Dataset) error
}

// SQLBackend is a Backend that can run native SQL directly against the underlying
// database — the engine path for PROC SQL pass-through (`connect to` /
// `execute ... by` / `select ... from connection to`). QuerySQL runs a query and
// returns its result set as a dataset; ExecSQL runs a no-result statement
// (DDL/DML). The SQL text is the database's own dialect, passed through verbatim.
type SQLBackend interface {
	Backend
	QuerySQL(query string) (*Dataset, error)
	ExecSQL(query string) error
}

// DropBackend is a Backend that can drop one of its members — the engine path for
// routing `proc sql; drop table <libref>.<member>;` to the bound database instead
// of the embedded SQLite engine.
type DropBackend interface {
	Backend
	Drop(member string) error
}

// Selection is a dialect-neutral pushdown request: a column projection and/or a
// row filter that a FilterBackend may apply while loading a member, so the data
// source returns less data. It is an optimization only — the caller still applies
// the full dataset options locally, so a backend may apply none, some, or all of
// a Selection and the result is identical either way (see FilterBackend).
type Selection struct {
	Columns []string // original column names to project; nil/empty = all columns
	Filter  *Filter  // row filter to push down; nil = no filter
}

// IsEmpty reports whether a Selection requests no pushdown at all.
func (s Selection) IsEmpty() bool { return len(s.Columns) == 0 && s.Filter == nil }

// FilterKind tags a Filter node as a boolean combinator or a leaf comparison.
type FilterKind int

const (
	FilterAnd FilterKind = iota // all Sub must hold
	FilterOr                    // any Sub must hold
	FilterCmp                   // a leaf: Column Op Number
)

// Filter is a small dialect-neutral boolean tree over column-vs-numeric-literal
// comparisons — deliberately limited to the subset whose SQL row selection
// provably matches SAS's. Only the operators "=", ">", ">=" appear at a leaf:
// SAS orders a missing value below every number, so these three exclude missing
// exactly as SQL's NULL handling excludes NULL, making the pushed predicate a
// faithful (not merely a superset) filter. Operators that keep missing in SAS
// ("<", "<=", "ne") are never represented here, so they are never pushed.
// Backends render a Filter to their own SQL dialect and validate that each
// referenced column is numeric before using it.
type Filter struct {
	Kind   FilterKind
	Sub    []*Filter // for FilterAnd / FilterOr
	Column string    // for FilterCmp: the (case-insensitive) column name
	Op     string    // for FilterCmp: one of "=", ">", ">="
	Number float64   // for FilterCmp: the numeric literal
}

// FilterBackend is an optional Backend that can apply a Selection (column
// projection and/or row filter) while loading, pushing the work to the data
// source instead of materializing the whole table. A Backend that does not
// implement it is loaded in full via Load and filtered locally. Because the
// caller always re-applies the dataset options locally, a FilterBackend is free
// to honor a Selection partially (or ignore it) without affecting correctness —
// it only affects how much data is transferred.
type FilterBackend interface {
	Backend
	LoadFiltered(member string, sel Selection) (ds *Dataset, ok bool, err error)
}

// Library models a SAS session's libraries. The unnamed in-memory store is WORK
// (where steps pass datasets to one another); additional librefs may be bound to
// external Backends via Assign (the LIBNAME statement). Names are
// case-insensitive (uppercased), as in SAS.
type Library struct {
	datasets map[string]*Dataset // the WORK in-memory store
	backends map[string]Backend  // libref (uppercased) -> external engine

	// Formats holds user-defined formats created by PROC FORMAT during the run.
	// It is scoped to the library so definitions never leak between programs.
	Formats *FormatCatalog
}

// NewLibrary creates an empty library (WORK only).
func NewLibrary() *Library {
	return &Library{
		datasets: make(map[string]*Dataset),
		backends: make(map[string]Backend),
		Formats:  NewFormatCatalog(),
	}
}

// Assign binds a libref to an external Backend (the LIBNAME statement).
func (l *Library) Assign(libref string, b Backend) {
	l.backends[strings.ToUpper(libref)] = b
}

// Unassign removes a libref binding (`libname <ref> clear;`). WORK cannot be
// unassigned here.
func (l *Library) Unassign(libref string) {
	delete(l.backends, strings.ToUpper(libref))
}

// IsExternal reports whether a (possibly qualified) name refers to a member of a
// libref bound to an external Backend.
func (l *Library) IsExternal(name string) bool {
	_, ok := l.backends[strings.ToUpper(librefOf(name))]
	return ok
}

// Backend returns the external Backend bound to a libref (case-insensitive), or
// false if the libref is unassigned. Used by PROC SQL pass-through to reach an
// already-assigned libref's database connection without re-specifying it.
func (l *Library) Backend(libref string) (Backend, bool) {
	b, ok := l.backends[strings.ToUpper(libref)]
	return b, ok
}

// Resolve returns the dataset for a (possibly qualified) name. A name qualified
// with a libref bound to an external Backend is loaded from that backend;
// everything else resolves to the WORK store. Unlike Get, it can perform I/O and
// so returns an error.
func (l *Library) Resolve(name string) (*Dataset, bool, error) {
	if ref := strings.ToUpper(librefOf(name)); ref != "" {
		if b, ok := l.backends[ref]; ok {
			return b.Load(datasetKey(name))
		}
	}
	ds, ok := l.Get(name)
	return ds, ok, nil
}

// ResolveFiltered resolves a (possibly qualified) name like Resolve, but when the
// name is bound to a FilterBackend it pushes the given Selection (column
// projection and/or row filter) to the source so less data is transferred. For
// the WORK store or a backend that is not a FilterBackend it behaves exactly like
// Resolve (the Selection is ignored and the caller applies it locally). It never
// changes results — pushdown is a transfer optimization layered beneath the
// caller's own dataset-option pass.
func (l *Library) ResolveFiltered(name string, sel Selection) (*Dataset, bool, error) {
	if ref := strings.ToUpper(librefOf(name)); ref != "" {
		if b, ok := l.backends[ref]; ok {
			if fb, ok := b.(FilterBackend); ok && !sel.IsEmpty() {
				return fb.LoadFiltered(datasetKey(name), sel)
			}
			return b.Load(datasetKey(name))
		}
	}
	ds, ok := l.Get(name)
	return ds, ok, nil
}

// Put stores (or replaces) a dataset under its name.
func (l *Library) Put(ds *Dataset) {
	l.datasets[strings.ToUpper(ds.Name)] = ds
}

// StoreExternal routes a dataset to an external Backend when its name is
// qualified with a libref bound to one. handled is true when the name belongs to
// an external library (whether or not the write succeeded); when false, the
// caller should fall back to the WORK store. An external library that is not
// writable yields a clear error. On success ds.Lib/ds.Name are set to the
// resolved libref and member so callers can log an accurate NOTE.
func (l *Library) StoreExternal(name string, ds *Dataset) (handled bool, err error) {
	ref := strings.ToUpper(librefOf(name))
	if ref == "" {
		return false, nil
	}
	b, ok := l.backends[ref]
	if !ok {
		return false, nil
	}
	wb, ok := b.(WriteBackend)
	if !ok {
		return true, fmt.Errorf("library %s is read-only; cannot write member %s", ref, datasetKey(name))
	}
	ds.Lib, ds.Name = ref, datasetKey(name)
	return true, wb.Store(ds)
}

// Store routes a dataset to the destination named by `name`: a libref-qualified
// name bound to a writable Backend is written there (replace semantics, via
// StoreExternal); anything else is stored in the WORK in-memory store under the
// name's member component. On success ds.Lib/ds.Name reflect the resolved
// destination so callers can log an accurate NOTE. It is the write counterpart of
// Resolve and the single routing point PROCs (PROC SORT out=, PROC SQL create
// table) and the DATA step share.
func (l *Library) Store(name string, ds *Dataset) error {
	handled, err := l.StoreExternal(name, ds)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	ds.Name = datasetKey(name)
	l.Put(ds)
	return nil
}

// AppendExternal routes an append to an external Backend when name is qualified
// with a libref bound to one. Like StoreExternal, handled reports whether the
// name belongs to an external library (whether or not the write succeeded). A
// backend implementing AppendBackend appends rows in place; a plain WriteBackend
// is loaded, has the rows combined, and is replaced; a read-only library yields a
// clear error. On success ds.Lib/ds.Name reflect the resolved destination.
func (l *Library) AppendExternal(name string, ds *Dataset) (handled bool, err error) {
	ref := strings.ToUpper(librefOf(name))
	if ref == "" {
		return false, nil
	}
	b, ok := l.backends[ref]
	if !ok {
		return false, nil
	}
	member := datasetKey(name)
	ds.Lib, ds.Name = ref, member
	if ab, ok := b.(AppendBackend); ok {
		return true, ab.Append(ds)
	}
	wb, ok := b.(WriteBackend)
	if !ok {
		return true, fmt.Errorf("library %s is read-only; cannot append member %s", ref, member)
	}
	existing, found, lerr := b.Load(member)
	if lerr != nil {
		return true, lerr
	}
	if found {
		existing.Lib, existing.Name = ref, member
		existing.Rows = append(existing.Rows, ds.Rows...)
		return true, wb.Store(existing)
	}
	return true, wb.Store(ds)
}

// Append adds ds's rows to the member named by name, creating it if absent: an
// external AppendBackend member is appended in place (INSERT-only), an external
// plain WriteBackend is load-combine-replaced, and a WORK member has the rows
// appended in memory. On success ds.Lib/ds.Name reflect the resolved
// destination. It is the append counterpart of Store, the routing point PROC
// APPEND shares.
func (l *Library) Append(name string, ds *Dataset) error {
	handled, err := l.AppendExternal(name, ds)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	member := datasetKey(name)
	if existing, ok := l.Get(member); ok {
		existing.Rows = append(existing.Rows, ds.Rows...)
		ds.Lib, ds.Name = existing.Lib, existing.Name
		return nil
	}
	ds.Name = member
	l.Put(ds)
	return nil
}

// DropExternal routes a table drop to an external Backend when name is qualified
// with a libref bound to one. handled reports whether the name belongs to an
// external library (whether or not the drop succeeded); when false the caller
// should fall back to the WORK store / embedded engine. A backend that cannot
// drop yields a clear error. It is the drop counterpart of StoreExternal, used by
// PROC SQL to route `drop table <libref>.<member>`.
func (l *Library) DropExternal(name string) (handled bool, err error) {
	ref := strings.ToUpper(librefOf(name))
	if ref == "" {
		return false, nil
	}
	b, ok := l.backends[ref]
	if !ok {
		return false, nil
	}
	db, ok := b.(DropBackend)
	if !ok {
		return true, fmt.Errorf("library %s does not support DROP", ref)
	}
	return true, db.Drop(datasetKey(name))
}

// Get retrieves a dataset by name (case-insensitive). A name may be qualified
// as "lib.name"; the library component is currently ignored (everything lives
// in one in-memory library).
func (l *Library) Get(name string) (*Dataset, bool) {
	ds, ok := l.datasets[strings.ToUpper(datasetKey(name))]
	return ds, ok
}

// Delete removes a dataset by name (case-insensitive). It is a no-op if absent.
func (l *Library) Delete(name string) {
	delete(l.datasets, strings.ToUpper(datasetKey(name)))
}

// Has reports whether a dataset with the given name exists.
func (l *Library) Has(name string) bool {
	_, ok := l.Get(name)
	return ok
}

// Names returns the stored dataset names (uppercased), unordered.
func (l *Library) Names() []string {
	names := make([]string, 0, len(l.datasets))
	for k := range l.datasets {
		names = append(names, k)
	}
	return names
}

// datasetKey extracts the dataset (member) component from a possibly-qualified
// name ("work.people" -> "people").
func datasetKey(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i+1:]
	}
	return name
}

// librefOf extracts the library reference from a qualified name ("pg.customers"
// -> "pg"), or "" when the name is unqualified.
func librefOf(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[:i]
	}
	return ""
}
