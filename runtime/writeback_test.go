//go:build cgo

package runtime

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/parser"
	"github.com/solifugus/ass/table"
)

// TestDataStepWriteBackSQLite exercises the full ETL write path end-to-end: a
// LIBNAME bound to a SQLite database, a DATA step writing into it (`data db.out;
// set ...;`), and a later step reading it back. The SQLite engine needs cgo
// (so does the whole build), making this verifiable without any external DB.
func TestDataStepWriteBackSQLite(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "etl.db")
	src := `
libname db sqlite "` + dbPath + `";

data src;
  input id name $ amt;
  datalines;
1 Acme 100
2 Globex 250
3 Initech 50
;
run;

data db.orders;
  set src;
  where amt >= 100;
run;

data roundtrip;
  set db.orders;
run;
`
	prog := parser.New(src).ParseProgram()
	lib := table.NewLibrary()
	var b strings.Builder
	if err := RunProgram(prog, lib, log.New(&b)); err != nil {
		t.Fatalf("RunProgram error: %v", err)
	}

	// db.orders is written to the database, NOT the WORK store.
	if _, ok := lib.Get("orders"); ok {
		t.Errorf("ORDERS should live in the external library, not WORK")
	}

	// Reading it back through SET db.orders reconstructs the filtered rows.
	rt, ok := lib.Get("roundtrip")
	if !ok {
		t.Fatalf("ROUNDTRIP not built")
	}
	if rt.NObs() != 2 {
		t.Fatalf("roundtrip nobs = %d, want 2 (amt>=100)", rt.NObs())
	}
	if v := rt.Get(rt.Rows[0], "id"); v.Num != 1 {
		t.Errorf("row 0 id = %v, want 1", v.Display())
	}
	if v := rt.Get(rt.Rows[0], "name"); v.Str != "Acme" {
		t.Errorf("row 0 name = %q, want Acme", v.Str)
	}
	if v := rt.Get(rt.Rows[1], "amt"); v.Num != 250 {
		t.Errorf("row 1 amt = %v, want 250", v.Display())
	}

	// The write logged a NOTE attributing the dataset to the external libref.
	if out := b.String(); !strings.Contains(out, "DB.ORDERS") {
		t.Errorf("expected a NOTE for DB.ORDERS; log:\n%s", out)
	}
}

// TestProcSortOutToDB verifies PROC SORT can write its OUT= dataset to an
// external LIBNAME engine (`proc sort data=work out=db.sorted; ...`). The sorted
// rows must land in the database (not WORK) and read back in order.
func TestProcSortOutToDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "sort.db")
	src := `
libname db sqlite "` + dbPath + `";

data people;
  input id name $;
  datalines;
3 Carol
1 Alice
2 Bob
;
run;

proc sort data=people out=db.sorted;
  by id;
run;

data back;
  set db.sorted;
run;
`
	prog := parser.New(src).ParseProgram()
	lib := table.NewLibrary()
	var b strings.Builder
	if err := RunProgram(prog, lib, log.New(&b)); err != nil {
		t.Fatalf("RunProgram error: %v", err)
	}

	if _, ok := lib.Get("sorted"); ok {
		t.Errorf("SORTED should live in the external library, not WORK")
	}
	back, ok := lib.Get("back")
	if !ok {
		t.Fatalf("BACK not built")
	}
	if back.NObs() != 3 {
		t.Fatalf("back nobs = %d, want 3", back.NObs())
	}
	wantNames := []string{"Alice", "Bob", "Carol"} // sorted by id 1,2,3
	for i, want := range wantNames {
		if got := back.Get(back.Rows[i], "name").Str; got != want {
			t.Errorf("row %d name = %q, want %q", i, got, want)
		}
	}
	if out := b.String(); !strings.Contains(out, "DB.SORTED") {
		t.Errorf("expected a NOTE for DB.SORTED; log:\n%s", out)
	}
}

// TestProcSQLCreateTableToDB verifies PROC SQL `create table db.x as select ...`
// materializes the result into an external LIBNAME engine rather than WORK.
func TestProcSQLCreateTableToDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "sql.db")
	src := `
libname db sqlite "` + dbPath + `";

data sales;
  input region $ amt;
  datalines;
east 100
west 200
east 50
;
run;

proc sql;
  create table db.totals as
    select region, sum(amt) as total
    from sales
    group by region
    order by region;
quit;

data back;
  set db.totals;
run;
`
	prog := parser.New(src).ParseProgram()
	lib := table.NewLibrary()
	var b strings.Builder
	if err := RunProgram(prog, lib, log.New(&b)); err != nil {
		t.Fatalf("RunProgram error: %v", err)
	}

	if _, ok := lib.Get("totals"); ok {
		t.Errorf("TOTALS should live in the external library, not WORK")
	}
	back, ok := lib.Get("back")
	if !ok {
		t.Fatalf("BACK not built")
	}
	if back.NObs() != 2 {
		t.Fatalf("back nobs = %d, want 2 (one row per region)", back.NObs())
	}
	// Ordered by region: east (100+50=150), west (200).
	if r := back.Get(back.Rows[0], "region").Str; r != "east" {
		t.Errorf("row 0 region = %q, want east", r)
	}
	if v := back.Get(back.Rows[0], "total"); v.Num != 150 {
		t.Errorf("east total = %v, want 150", v.Display())
	}
	if v := back.Get(back.Rows[1], "total"); v.Num != 200 {
		t.Errorf("west total = %v, want 200", v.Display())
	}
	if out := b.String(); !strings.Contains(out, "DB.TOTALS") {
		t.Errorf("expected a NOTE for DB.TOTALS; log:\n%s", out)
	}
}

// TestDataStepWriteBackReadOnly verifies a clear error when a DATA step targets a
// read-only library (a base/directory .sas7bdat libref does not implement write).
func TestDataStepWriteBackReadOnly(t *testing.T) {
	dir := t.TempDir()
	src := `
libname ro "` + dir + `";

data src;
  x = 1;
run;

data ro.cant;
  set src;
run;
`
	prog := parser.New(src).ParseProgram()
	lib := table.NewLibrary()
	err := RunProgram(prog, lib, log.New(&strings.Builder{}))
	if err == nil {
		t.Fatal("expected an error writing to a read-only library")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Errorf("error = %v, want a read-only message", err)
	}
}
