package runtime

import (
	"strings"
	"testing"
)

// TestInputLinePointer reads one observation across multiple physical records
// with `#n` line pointers: `#1` reads from the first line of the record, `#2`
// from the second. Two observations of two lines each.
func TestInputLinePointer(t *testing.T) {
	src := "data t;\n  input #1 name $ age #2 city $ zip;\n  datalines;\nJohn 25\nBoston 2134\nMary 30\nAustin 78701\n;\nrun;"
	lib := runStep(t, src)
	ds, ok := lib.Get("t")
	if !ok {
		t.Fatal("T not created")
	}
	if ds.NObs() != 2 {
		t.Fatalf("NObs = %d, want 2", ds.NObs())
	}
	if got := names(ds, "name"); !eqStr(got, []string{"John", "Mary"}) {
		t.Errorf("name = %v, want [John Mary]", got)
	}
	if got := names(ds, "age"); !eqStr(got, []string{"25", "30"}) {
		t.Errorf("age = %v, want [25 30]", got)
	}
	if got := names(ds, "city"); !eqStr(got, []string{"Boston", "Austin"}) {
		t.Errorf("city = %v, want [Boston Austin]", got)
	}
	if got := names(ds, "zip"); !eqStr(got, []string{"2134", "78701"}) {
		t.Errorf("zip = %v, want [2134 78701]", got)
	}
}

// TestInputLinePointerColumn confirms `#n` works with column input: each line's
// columns are read independently (the column pointer resets to 1 at each `#n`).
func TestInputLinePointerColumn(t *testing.T) {
	src := "data t;\n  input #1 id 1-3 #2 score 1-3;\n  datalines;\n001\n090\n002\n075\n;\nrun;"
	lib := runStep(t, src)
	ds, _ := lib.Get("t")
	if ds.NObs() != 2 {
		t.Fatalf("NObs = %d, want 2", ds.NObs())
	}
	if got := names(ds, "id"); !eqStr(got, []string{"1", "2"}) {
		t.Errorf("id = %v, want [1 2]", got)
	}
	if got := names(ds, "score"); !eqStr(got, []string{"90", "75"}) {
		t.Errorf("score = %v, want [90 75]", got)
	}
}

// TestPutLinePointer emits several physical output lines from one PUT via `#n`.
func TestPutLinePointer(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/out.txt"
	src := "data _null_;\n  input name $ age;\n  file \"" + path + "\";\n  put #1 name #2 age;\n  datalines;\nJohn 25\n;\nrun;"
	runStep(t, src)
	got := strings.Split(strings.TrimRight(readFile(t, path), "\n"), "\n")
	want := []string{"John", "25"}
	if !eqStr(got, want) {
		t.Errorf("output lines = %v, want %v", got, want)
	}
}
