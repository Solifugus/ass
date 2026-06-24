package proc_test

import (
	"path/filepath"
	"testing"
)

// These verify that query pushdown over an external (SQLite) source produces
// values identical to local filtering — pushdown is a transfer optimization, not
// a semantic change. SQLite is the dbio engine that is CGo-only, hence the tag.

// seedPushdownDB returns a SAS prelude binding a SQLite libref and seeding a
// table with a missing numeric, the case where SAS and SQL filter semantics
// differ.
func seedPushdownDB(t *testing.T) string {
	t.Helper()
	db := filepath.Join(t.TempDir(), "pd.db")
	return `libname db sqlite "` + db + `";
data db.t;
  input id x;
  datalines;
1 10
2 .
3 3
;
run;
`
}

// TestPushdownSafeOpEquivalent: a `>=` filter (safe to push) returns the same
// rows whether or not it is pushed — missing is excluded by both SAS and SQL.
func TestPushdownSafeOpEquivalent(t *testing.T) {
	src := seedPushdownDB(t) + `data out;
  set db.t(keep=id x where=(x >= 3));
run;`
	lib := runSrc(t, src)
	out, ok := lib.Get("out")
	if !ok {
		t.Fatal("out not created")
	}
	if got := names(out, "id"); !eq(got, []string{"1", "3"}) {
		t.Errorf("id = %v, want [1 3] (x>=3 excludes the missing row, as in SAS)", got)
	}
	// keep= projected to exactly id and x.
	if len(out.Columns) != 2 {
		t.Errorf("columns = %d, want 2 (keep=id x)", len(out.Columns))
	}
}

// TestPushdownUnsafeOpKeepsMissing: a `<` filter is NOT pushed (SAS keeps missing
// for `<`, SQL would drop it). The missing row must survive — proving the local
// pass remains the source of truth.
func TestPushdownUnsafeOpKeepsMissing(t *testing.T) {
	src := seedPushdownDB(t) + `data out;
  set db.t(where=(x < 5));
run;`
	lib := runSrc(t, src)
	out, ok := lib.Get("out")
	if !ok {
		t.Fatal("out not created")
	}
	// SAS: x=10 dropped; x=. (missing < 5 is true) kept; x=3 kept.
	if got := names(out, "id"); !eq(got, []string{"2", "3"}) {
		t.Errorf("id = %v, want [2 3] (missing kept by SAS `<` semantics)", got)
	}
	if !out.Get(out.Rows[0], "x").IsMissing() {
		t.Errorf("row id=2 x should be missing, got %v", out.Get(out.Rows[0], "x").Display())
	}
}
