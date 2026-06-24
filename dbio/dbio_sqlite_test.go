package dbio

import (
	"path/filepath"
	"testing"

	"github.com/solifugus/ass/table"
)

// newSQLiteBackend opens a SQLite LIBNAME backend on a throwaway database file.
func newSQLiteBackend(t *testing.T) *Backend {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rt.db")
	be, err := Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite backend: %v", err)
	}
	t.Cleanup(func() { be.Close() })
	return be
}

// sampleDataset is a 3-row table exercising numeric, character, date, and missing
// values — the cases write-back must round-trip.
func sampleDataset() *table.Dataset {
	ds := table.NewDataset("WORK", "customers")
	ds.AddColumn(table.Column{Name: "id", Kind: table.Numeric})
	ds.AddColumn(table.Column{Name: "name", Kind: table.Character, Length: 20})
	ds.AddColumn(table.Column{Name: "balance", Kind: table.Numeric})
	ds.AddColumn(table.Column{Name: "opened", Kind: table.Numeric, Format: "date9."})
	// 2020-01-15 is SAS day 21929.
	ds.AppendRow(table.Row{"id": table.Num(1), "name": table.Char("Acme"), "balance": table.Num(1234.50), "opened": table.Num(21929)})
	ds.AppendRow(table.Row{"id": table.Num(2), "name": table.Char("Globex"), "balance": table.MissingNum(), "opened": table.MissingNum()})
	ds.AppendRow(table.Row{"id": table.Num(3), "name": table.MissingChar(), "balance": table.Num(-0.5), "opened": table.Num(0)})
	return ds
}

func TestSQLiteStoreLoadRoundTrip(t *testing.T) {
	be := newSQLiteBackend(t)
	if err := be.Store(sampleDataset()); err != nil {
		t.Fatalf("Store: %v", err)
	}

	ds, ok, err := be.Load("customers")
	if err != nil || !ok {
		t.Fatalf("Load: ok=%v err=%v", ok, err)
	}
	if ds.NObs() != 3 {
		t.Fatalf("nobs = %d, want 3", ds.NObs())
	}

	r0 := ds.Rows[0]
	if v := ds.Get(r0, "id"); v.Kind != table.Numeric || v.Num != 1 {
		t.Errorf("id = %v (kind %v), want 1 numeric", v.Display(), v.Kind)
	}
	if v := ds.Get(r0, "name"); v.Kind != table.Character || v.Str != "Acme" {
		t.Errorf("name = %q (kind %v), want Acme char", v.Str, v.Kind)
	}
	if v := ds.Get(r0, "balance"); v.IsMissing() || v.Num != 1234.50 {
		t.Errorf("balance = %v, want 1234.50", v.Display())
	}
	if v := ds.Get(r0, "opened"); v.IsMissing() || v.Num != 21929 {
		t.Errorf("opened = %v, want 21929 (SAS date for 2020-01-15)", v.Display())
	}

	// Row 2: missing numeric balance and missing date round-trip to SAS missing.
	r1 := ds.Rows[1]
	if !ds.Get(r1, "balance").IsMissing() {
		t.Errorf("NULL balance should be missing, got %v", ds.Get(r1, "balance").Display())
	}
	if !ds.Get(r1, "opened").IsMissing() {
		t.Errorf("NULL date should be missing, got %v", ds.Get(r1, "opened").Display())
	}

	// Row 3: missing character round-trips to missing; opened day 0 -> 1960-01-01.
	r2 := ds.Rows[2]
	if !ds.Get(r2, "name").IsMissing() {
		t.Errorf("missing name should be missing, got %q", ds.Get(r2, "name").Str)
	}
	if v := ds.Get(r2, "opened"); v.IsMissing() || v.Num != 0 {
		t.Errorf("opened = %v, want 0 (SAS epoch)", v.Display())
	}
}

// TestSQLiteStoreReplaces verifies a second Store of the same member replaces the
// table rather than appending (SAS dataset-replacement semantics).
func TestSQLiteStoreReplaces(t *testing.T) {
	be := newSQLiteBackend(t)
	if err := be.Store(sampleDataset()); err != nil {
		t.Fatalf("first Store: %v", err)
	}

	smaller := table.NewDataset("WORK", "customers")
	smaller.AddColumn(table.Column{Name: "id", Kind: table.Numeric})
	smaller.AppendRow(table.Row{"id": table.Num(99)})
	if err := be.Store(smaller); err != nil {
		t.Fatalf("second Store: %v", err)
	}

	ds, ok, err := be.Load("customers")
	if err != nil || !ok {
		t.Fatalf("Load: ok=%v err=%v", ok, err)
	}
	if ds.NObs() != 1 {
		t.Fatalf("nobs = %d, want 1 (replaced, not appended)", ds.NObs())
	}
	if len(ds.Columns) != 1 {
		t.Fatalf("columns = %d, want 1 (replaced schema)", len(ds.Columns))
	}
	if v := ds.Get(ds.Rows[0], "id"); v.Num != 99 {
		t.Errorf("id = %v, want 99", v.Display())
	}
}
