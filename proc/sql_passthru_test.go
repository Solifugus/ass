package proc_test

import (
	"path/filepath"
	"testing"
)

// These drive PROC SQL pass-through end-to-end through the real pipeline
// (parser -> runtime -> PROC SQL -> dbio SQLite backend) using a throwaway
// SQLite file, so no database server is needed. SQLite is the dbio engine that
// is CGo-only, hence the build tag.

// TestPassthroughAssignedLibref reuses an already-assigned libref's connection:
// no `connect to` is needed — `from connection to <libref>` runs native SQL on
// that database and brings the result back as a WORK dataset.
func TestPassthroughAssignedLibref(t *testing.T) {
	db := filepath.Join(t.TempDir(), "pt.db")
	src := `libname db sqlite "` + db + `";
data db.sales;
  input region $ amt;
  datalines;
East 100
East 50
West 70
;
run;
proc sql;
  create table work.summ as
    select * from connection to db
      (select region, sum(amt) as total from "sales" group by region order by region);
quit;`
	lib := runSrc(t, src)
	ds, ok := lib.Get("summ")
	if !ok {
		t.Fatal("work.summ not created by pass-through create table")
	}
	if ds.NObs() != 2 {
		t.Fatalf("nobs = %d, want 2", ds.NObs())
	}
	if got := names(ds, "region"); !eq(got, []string{"East", "West"}) {
		t.Errorf("region = %v, want [East West]", got)
	}
	if got := names(ds, "total"); !eq(got, []string{"150", "70"}) {
		t.Errorf("total = %v, want [150 70]", got)
	}
}

// TestPassthroughConnectExecuteDrop covers the explicit `connect to ... (connection=...)`
// path plus `execute (...) by` (native DML) and `drop table <libref>.<member>`
// routing to the external backend.
func TestPassthroughConnectExecuteDrop(t *testing.T) {
	db := filepath.Join(t.TempDir(), "pt.db")
	src := `libname db sqlite "` + db + `";
data db.t;
  input x;
  datalines;
1
2
3
;
run;
proc sql;
  connect to sqlite as c (connection="` + db + `");
  execute (delete from "t" where x = 2) by c;
  create table work.kept as
    select * from connection to c (select x from "t" order by x);
  disconnect from c;
quit;`
	lib := runSrc(t, src)
	kept, ok := lib.Get("kept")
	if !ok {
		t.Fatal("work.kept not created")
	}
	if got := names(kept, "x"); !eq(got, []string{"1", "3"}) {
		t.Errorf("x after delete = %v, want [1 3]", got)
	}

	// drop table <libref>.<member> routes to the external engine.
	drop := `libname db sqlite "` + db + `";
proc sql; drop table db.t; quit;`
	lib2 := runSrc(t, drop)
	if _, found, _ := lib2.Resolve("db.t"); found {
		t.Error("db.t still present after pass-through drop")
	}
}
