package dbio

import (
	"database/sql"
	"os"
	"testing"

	"github.com/solifugus/ass/table"
)

// TestPostgresIntegration exercises the real Postgres backend end-to-end. It is
// skipped unless ASS_PG_DSN is set, e.g.:
//
//	ASS_PG_DSN="postgres://user:pass@localhost:5432/dbname?sslmode=disable" \
//	    go test ./dbio/ -run TestPostgresIntegration -v
//
// It creates a throwaway table, reads it through the LIBNAME backend, asserts the
// SAS value mapping, and drops the table.
func TestPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("ASS_PG_DSN")
	if dsn == "" {
		t.Skip("set ASS_PG_DSN to run the Postgres integration test")
	}

	setup, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer setup.Close()
	const tbl = "ass_it_customers"
	exec := func(q string) {
		if _, err := setup.Exec(q); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	exec("DROP TABLE IF EXISTS " + tbl)
	exec("CREATE TABLE " + tbl + " (id int, name text, balance numeric(10,2), opened date, vip boolean)")
	exec("INSERT INTO " + tbl + " VALUES (1,'Acme',1234.50,'2020-01-15',true),(2,'Globex',NULL,NULL,false)")
	defer exec("DROP TABLE IF EXISTS " + tbl)

	be, err := Open("postgres", dsn)
	if err != nil {
		t.Fatalf("backend open: %v", err)
	}
	defer be.Close()

	ds, ok, err := be.Load(tbl)
	if err != nil || !ok {
		t.Fatalf("Load: ok=%v err=%v", ok, err)
	}
	if ds.NObs() != 2 {
		t.Fatalf("nobs = %d, want 2", ds.NObs())
	}
	// Row 1: numeric id, char name, numeric balance, date opened (SAS day 21929
	// for 2020-01-15), boolean vip -> 1.
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
	if v := ds.Get(r0, "vip"); v.Num != 1 {
		t.Errorf("vip = %v, want 1", v.Display())
	}
	// Row 2: NULL balance/opened -> SAS missing.
	r1 := ds.Rows[1]
	if !ds.Get(r1, "balance").IsMissing() {
		t.Errorf("NULL balance should be missing, got %v", ds.Get(r1, "balance").Display())
	}
	if !ds.Get(r1, "opened").IsMissing() {
		t.Errorf("NULL date should be missing, got %v", ds.Get(r1, "opened").Display())
	}
}
