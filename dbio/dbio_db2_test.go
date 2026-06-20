//go:build db2

package dbio

import (
	"database/sql"
	"os"
	"testing"

	"github.com/solifugus/ass/table"
)

// TestDB2Integration exercises the real DB2 backend's write path end-to-end. It
// is skipped unless ASS_DB2_DSN is set, e.g. (against the icr.io/db2_community/db2
// container used for local testing):
//
//	ASS_DB2_DSN="HOSTNAME=localhost;PORT=50000;DATABASE=testdb;UID=db2inst1;PWD=ass_test" \
//	    go test -tags db2 ./dbio/ -run TestDB2Integration -v
//
// (Built with -tags db2 and the IBM CLI driver on the loader path; see
// dbio_db2.go.) It writes a dataset through Store (DROP/CREATE/INSERT in one
// transaction), reads it back through Load asserting the SAS value/type mapping —
// VARCHAR, DOUBLE, the DATE round-trip, and NULL/missing handling — then appends
// a row through Append (in-place INSERT, no recreate) and confirms the table grew
// in place.
func TestDB2Integration(t *testing.T) {
	dsn := os.Getenv("ASS_DB2_DSN")
	if dsn == "" {
		t.Skip("set ASS_DB2_DSN to run the DB2 integration test")
	}

	be, err := Open("db2", dsn)
	if err != nil {
		t.Fatalf("backend open: %v", err)
	}
	defer be.Close()

	const tbl = "ass_it_customers"
	// Best-effort cleanup. Store drops-if-exists on the way in, so a leftover table
	// from a prior failed run is harmless; the quoted lowercase name matches how
	// Store/Load reference it.
	defer func() {
		if conn, err := sql.Open("go_ibm_db", dsn); err == nil {
			conn.Exec(`DROP TABLE "` + tbl + `"`)
			conn.Close()
		}
	}()

	ds := table.NewDataset("WORK", tbl)
	ds.AddColumn(table.Column{Name: "id", Kind: table.Numeric})
	ds.AddColumn(table.Column{Name: "name", Kind: table.Character, Length: 20})
	ds.AddColumn(table.Column{Name: "balance", Kind: table.Numeric})
	ds.AddColumn(table.Column{Name: "opened", Kind: table.Numeric, Format: "date9."})
	// 2020-01-15 is SAS day 21929.
	ds.AppendRow(table.Row{"id": table.Num(1), "name": table.Char("Acme"), "balance": table.Num(1234.50), "opened": table.Num(21929)})
	ds.AppendRow(table.Row{"id": table.Num(2), "name": table.Char("Globex"), "balance": table.MissingNum(), "opened": table.MissingNum()})

	if err := be.Store(ds); err != nil {
		t.Fatalf("Store: %v", err)
	}

	got, ok, err := be.Load(tbl)
	if err != nil || !ok {
		t.Fatalf("Load: ok=%v err=%v", ok, err)
	}
	if got.NObs() != 2 {
		t.Fatalf("nobs = %d, want 2", got.NObs())
	}

	r0 := got.Rows[0]
	if v := got.Get(r0, "id"); v.Kind != table.Numeric || v.Num != 1 {
		t.Errorf("id = %v (kind %v), want 1 numeric", v.Display(), v.Kind)
	}
	if v := got.Get(r0, "name"); v.Kind != table.Character || v.Str != "Acme" {
		t.Errorf("name = %q (kind %v), want Acme char", v.Str, v.Kind)
	}
	if v := got.Get(r0, "balance"); v.IsMissing() || v.Num != 1234.50 {
		t.Errorf("balance = %v, want 1234.50", v.Display())
	}
	if v := got.Get(r0, "opened"); v.IsMissing() || v.Num != 21929 {
		t.Errorf("opened = %v, want 21929 (SAS date for 2020-01-15)", v.Display())
	}

	// Row 2: NULL balance and NULL date round-trip to SAS missing.
	r1 := got.Rows[1]
	if !got.Get(r1, "balance").IsMissing() {
		t.Errorf("NULL balance should be missing, got %v", got.Get(r1, "balance").Display())
	}
	if !got.Get(r1, "opened").IsMissing() {
		t.Errorf("NULL date should be missing, got %v", got.Get(r1, "opened").Display())
	}

	// Append a third row in place (PROC APPEND path): not dropped or recreated.
	more := table.NewDataset("WORK", tbl)
	more.Columns = ds.Columns
	more.AppendRow(table.Row{"id": table.Num(3), "name": table.Char("Initech"), "balance": table.Num(999), "opened": table.Num(22000)})
	if err := be.Append(more); err != nil {
		t.Fatalf("Append: %v", err)
	}

	after, ok, err := be.Load(tbl)
	if err != nil || !ok {
		t.Fatalf("Load after append: ok=%v err=%v", ok, err)
	}
	if after.NObs() != 3 {
		t.Fatalf("nobs after append = %d, want 3 (in-place INSERT, not recreate)", after.NObs())
	}
	r2 := after.Rows[2]
	if v := after.Get(r2, "id"); v.Num != 3 {
		t.Errorf("appended id = %v, want 3", v.Display())
	}
	if v := after.Get(r2, "name"); v.Str != "Initech" {
		t.Errorf("appended name = %q, want Initech", v.Str)
	}
	if v := after.Get(r2, "opened"); v.IsMissing() || v.Num != 22000 {
		t.Errorf("appended opened = %v, want 22000", v.Display())
	}
}
