package proc_test

import (
	"strings"
	"testing"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/parser"
	"github.com/solifugus/ass/runtime"
	"github.com/solifugus/ass/table"
)

// runSrc executes a program and returns the resulting library.
func runSrc(t *testing.T, src string) *table.Library {
	t.Helper()
	prog := parser.New(src).ParseProgram()
	lib := table.NewLibrary()
	if err := runtime.RunProgram(prog, lib, log.New(&strings.Builder{})); err != nil {
		t.Fatalf("RunProgram error: %v", err)
	}
	return lib
}

// names returns the values of a column down the rows as strings.
func names(ds *table.Dataset, col string) []string {
	out := make([]string, ds.NObs())
	for i, r := range ds.Rows {
		out[i] = ds.Get(r, col).Display()
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSortByNumericInPlace(t *testing.T) {
	src := "data people;\n  input name $ age;\n  datalines;\nJohn 25\nMary 30\nTim 12\nAnn 17\n;\nrun;\n" +
		"proc sort data=people; by age; run;"
	lib := runSrc(t, src)
	ds, _ := lib.Get("people")
	if got := names(ds, "name"); !eq(got, []string{"Tim", "Ann", "John", "Mary"}) {
		t.Errorf("order = %v, want [Tim Ann John Mary]", got)
	}
}

func TestSortDescendingOut(t *testing.T) {
	src := "data people;\n  input name $ age;\n  datalines;\nJohn 25\nMary 30\nTim 12\nAnn 17\n;\nrun;\n" +
		"proc sort data=people out=sorted; by descending age; run;"
	lib := runSrc(t, src)
	sorted, ok := lib.Get("sorted")
	if !ok {
		t.Fatal("OUT= dataset SORTED not created")
	}
	if got := names(sorted, "name"); !eq(got, []string{"Mary", "John", "Ann", "Tim"}) {
		t.Errorf("sorted order = %v, want [Mary John Ann Tim]", got)
	}
	// Source is left in original order.
	people, _ := lib.Get("people")
	if got := names(people, "name"); !eq(got, []string{"John", "Mary", "Tim", "Ann"}) {
		t.Errorf("source order = %v, want original [John Mary Tim Ann]", got)
	}
}

func TestSortNodupkey(t *testing.T) {
	src := "data visits;\n  input id day $;\n  datalines;\n1 mon\n1 tue\n2 mon\n2 wed\n3 fri\n;\nrun;\n" +
		"proc sort data=visits out=unique nodupkey; by id; run;"
	lib := runSrc(t, src)
	u, _ := lib.Get("unique")
	if u.NObs() != 3 {
		t.Fatalf("NObs = %d, want 3 (one per id)", u.NObs())
	}
	if got := names(u, "id"); !eq(got, []string{"1", "2", "3"}) {
		t.Errorf("ids = %v, want [1 2 3]", got)
	}
	if got := names(u, "day"); !eq(got, []string{"mon", "mon", "fri"}) {
		t.Errorf("days = %v, want [mon mon fri] (first per id)", got)
	}
}

func TestSortMultiKeyStable(t *testing.T) {
	// Sort by dept then descending salary; ties keep input order.
	src := "data emp;\n  input dept $ salary name $;\n  datalines;\nA 100 x\nB 90 y\nA 120 z\nA 100 w\n;\nrun;\n" +
		"proc sort data=emp; by dept descending salary; run;"
	lib := runSrc(t, src)
	ds, _ := lib.Get("emp")
	// dept A: 120(z),100(x),100(w) [x before w, stable]; then dept B: 90(y)
	if got := names(ds, "name"); !eq(got, []string{"z", "x", "w", "y"}) {
		t.Errorf("order = %v, want [z x w y]", got)
	}
}

func TestSortMissingRequiresBy(t *testing.T) {
	src := "data t;\n  input x;\n  datalines;\n1\n;\nrun;\n" +
		"proc sort data=t; run;"
	prog := parser.New(src).ParseProgram()
	lib := table.NewLibrary()
	var b strings.Builder
	if err := runtime.RunProgram(prog, lib, log.New(&b)); err != nil {
		t.Fatalf("RunProgram error: %v", err)
	}
	if !strings.Contains(b.String(), "BY statement is required") {
		t.Errorf("expected BY-required error; log:\n%s", b.String())
	}
	// Sanity: the AST really had no BY statement.
	if step, ok := prog.Steps[1].(*ast.ProcStep); !ok || step.Name != "sort" {
		t.Fatal("expected second step to be proc sort")
	}
}
