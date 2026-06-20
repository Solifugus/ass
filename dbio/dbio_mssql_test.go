package dbio

import (
	"database/sql"
	"os"
	"testing"

	"github.com/solifugus/ass/table"
)

// TestSQLServerIntegration exercises the real SQL Server backend's write-back
// path end-to-end. It is skipped unless ASS_MSSQL_DSN is set, e.g.:
//
//	ASS_MSSQL_DSN="sqlserver://ass:PASS@192.168.122.178:1433?database=assdb&encrypt=disable" \
//	    go test ./dbio/ -run TestSQLServerIntegration -v
//
// It writes a dataset through Store (CREATE + INSERT in one transaction), reads
// it back through Load, asserts the SAS value/type mapping — including the DATE
// round-trip and NULL/missing handling — and drops the table. The whole table is
// created by ASS's own write path, so this validates the new DATA-step-to-DB
// target against a live SQL Server, not just SQLite.
func TestSQLServerIntegration(t *testing.T) {
	dsn := os.Getenv("ASS_MSSQL_DSN")
	if dsn == "" {
		t.Skip("set ASS_MSSQL_DSN to run the SQL Server integration test")
	}

	be, err := Open("sqlserver", dsn)
	if err != nil {
		t.Fatalf("backend open: %v", err)
	}
	defer be.Close()

	const tbl = "ass_it_customers"
	// Best-effort cleanup via a direct connection (Store also drops-if-exists on
	// the way in, so a leftover table from a prior failed run is harmless).
	defer func() {
		if conn, err := sql.Open("sqlserver", dsn); err == nil {
			conn.Exec("DROP TABLE IF EXISTS " + tbl)
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
}
