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
