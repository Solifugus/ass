package dbio

import (
	"testing"

	"github.com/solifugus/ass/table"
)

// TestLoadFilteredProjection pushes a column projection: only the requested
// columns come back, and the request is case-insensitive against the table's
// real column names.
func TestLoadFilteredProjection(t *testing.T) {
	be := newSQLiteBackend(t)
	if err := be.Store(sampleDataset()); err != nil {
		t.Fatalf("seed Store: %v", err)
	}

	ds, ok, err := be.LoadFiltered("customers", table.Selection{Columns: []string{"ID", "Name"}})
	if err != nil || !ok {
		t.Fatalf("LoadFiltered: ok=%v err=%v", ok, err)
	}
	if got := len(ds.Columns); got != 2 {
		t.Fatalf("columns = %d, want 2 (projection)", got)
	}
	if ds.Columns[0].Name != "id" || ds.Columns[1].Name != "name" {
		t.Errorf("columns = %v, want [id name]", []string{ds.Columns[0].Name, ds.Columns[1].Name})
	}
	if ds.NObs() != 3 {
		t.Errorf("nobs = %d, want 3 (projection keeps all rows)", ds.NObs())
	}
}

// TestLoadFilteredWhere pushes a numeric filter with a safe operator: only the
// matching rows come back.
func TestLoadFilteredWhere(t *testing.T) {
	be := newSQLiteBackend(t)
	if err := be.Store(sampleDataset()); err != nil {
		t.Fatalf("seed Store: %v", err)
	}

	f := &table.Filter{Kind: table.FilterCmp, Column: "id", Op: ">", Number: 1}
	ds, ok, err := be.LoadFiltered("customers", table.Selection{Filter: f})
	if err != nil || !ok {
		t.Fatalf("LoadFiltered: ok=%v err=%v", ok, err)
	}
	if ds.NObs() != 2 {
		t.Fatalf("nobs = %d, want 2 (id > 1 keeps id 2,3)", ds.NObs())
	}
	for _, r := range ds.Rows {
		if v := ds.Get(r, "id"); v.Num <= 1 {
			t.Errorf("row id = %v leaked past id > 1 filter", v.Display())
		}
	}
}

// TestLoadFilteredNonNumericNotPushed verifies a filter on a character column is
// NOT pushed (it would risk a value-divergent type coercion): all rows come back
// and the caller filters locally instead.
func TestLoadFilteredNonNumericNotPushed(t *testing.T) {
	be := newSQLiteBackend(t)
	if err := be.Store(sampleDataset()); err != nil {
		t.Fatalf("seed Store: %v", err)
	}

	// name is a character column; a numeric comparison must not be emitted.
	f := &table.Filter{Kind: table.FilterCmp, Column: "name", Op: ">", Number: 1}
	ds, ok, err := be.LoadFiltered("customers", table.Selection{Filter: f})
	if err != nil || !ok {
		t.Fatalf("LoadFiltered: ok=%v err=%v", ok, err)
	}
	if ds.NObs() != 3 {
		t.Errorf("nobs = %d, want 3 (filter on char column not pushed)", ds.NObs())
	}
}

// TestLoadFilteredMissingColumnFallsBack verifies a projection naming an absent
// column falls back to SELECT * (all columns), leaving correctness to the local
// KEEP/DROP pass.
func TestLoadFilteredMissingColumnFallsBack(t *testing.T) {
	be := newSQLiteBackend(t)
	if err := be.Store(sampleDataset()); err != nil {
		t.Fatalf("seed Store: %v", err)
	}

	ds, ok, err := be.LoadFiltered("customers", table.Selection{Columns: []string{"id", "nope"}})
	if err != nil || !ok {
		t.Fatalf("LoadFiltered: ok=%v err=%v", ok, err)
	}
	if len(ds.Columns) != 4 {
		t.Errorf("columns = %d, want 4 (fell back to SELECT *)", len(ds.Columns))
	}
}

// TestBackendImplementsFilterBackend asserts the SQLite backend satisfies the
// optional pushdown interface the runtime type-asserts to.
func TestBackendImplementsFilterBackend(t *testing.T) {
	be := newSQLiteBackend(t)
	if _, ok := interface{}(be).(table.FilterBackend); !ok {
		t.Error("Backend does not implement table.FilterBackend")
	}
}
