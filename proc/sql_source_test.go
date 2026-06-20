//go:build cgo

package proc_test

import (
	"path/filepath"
	"testing"
)

// These exercise PROC SQL reading an external LIBNAME member, and a WORK-qualified
// member, as query *sources* (the in-process SQLite engine path, not pass-through).
// The dbio SQLite backend is CGo-only, hence the build tag.

// TestSQLExternalSourceJoin joins an external-libref table (db.emp) with a
// WORK-qualified table (work.dept) and selects through table aliases. It verifies
// (a) an external member loads on demand as a source, (b) a work.-qualified source
// resolves, and (c) alias.column qualifiers (e.name, d.dept, e.id) pass through
// untouched.
func TestSQLExternalSourceJoin(t *testing.T) {
	db := filepath.Join(t.TempDir(), "src.db")
	src := `libname db sqlite "` + db + `";
data db.emp;
  input id name $;
  datalines;
1 Bob
2 Amy
;
run;
data dept;
  input id dept $;
  datalines;
1 Sales
2 Eng
;
run;
proc sql;
  create table work.joined as
    select e.name, d.dept
      from db.emp e join work.dept d on e.id = d.id
      order by e.name;
quit;`
	lib := runSrc(t, src)
	joined, ok := lib.Get("joined")
	if !ok {
		t.Fatal("work.joined not created")
	}
	if got := names(joined, "name"); !eq(got, []string{"Amy", "Bob"}) {
		t.Errorf("name = %v, want [Amy Bob]", got)
	}
	if got := names(joined, "dept"); !eq(got, []string{"Eng", "Sales"}) {
		t.Errorf("dept = %v, want [Eng Sales]", got)
	}
}

// TestSQLExternalSourceCopy copies an external member through a plain
// select * — the simplest external-as-source case.
func TestSQLExternalSourceCopy(t *testing.T) {
	db := filepath.Join(t.TempDir(), "copy.db")
	src := `libname db sqlite "` + db + `";
data db.nums;
  input x;
  datalines;
3
1
2
;
run;
proc sql;
  create table work.out as select x from db.nums where x >= 2 order by x;
quit;`
	lib := runSrc(t, src)
	out, ok := lib.Get("out")
	if !ok {
		t.Fatal("work.out not created")
	}
	if got := names(out, "x"); !eq(got, []string{"2", "3"}) {
		t.Errorf("x = %v, want [2 3]", got)
	}
}

// TestSQLExternalSourceToExternalTarget reads an external member as a source and
// writes the result back to the same external library as a new member, confirming
// source resolution composes with the libref-qualified create-table target path.
func TestSQLExternalSourceToExternalTarget(t *testing.T) {
	db := filepath.Join(t.TempDir(), "both.db")
	src := `libname db sqlite "` + db + `";
data db.base;
  input k $ v;
  datalines;
a 10
b 20
;
run;
proc sql;
  create table db.derived as select k, v * 2 as v2 from db.base order by k;
quit;`
	lib := runSrc(t, src)
	derived, found, err := lib.Resolve("db.derived")
	if err != nil {
		t.Fatalf("Resolve db.derived: %v", err)
	}
	if !found {
		t.Fatal("db.derived not created in external library")
	}
	if got := names(derived, "k"); !eq(got, []string{"a", "b"}) {
		t.Errorf("k = %v, want [a b]", got)
	}
	if got := names(derived, "v2"); !eq(got, []string{"20", "40"}) {
		t.Errorf("v2 = %v, want [20 40]", got)
	}
}
