package runtime

import (
	"testing"

	"github.com/solifugus/ass/ast"
	"github.com/solifugus/ass/parser"
	"github.com/solifugus/ass/table"
)

// runStep parses SAS source, runs its first step as a DATA step, and returns the
// library.
func runStep(t *testing.T, src string) *table.Library {
	t.Helper()
	prog := parser.New(src).ParseProgram()
	if len(prog.Steps) == 0 {
		t.Fatalf("no steps parsed from %q", src)
	}
	ds, ok := prog.Steps[0].(*ast.DataStep)
	if !ok {
		t.Fatalf("first step is %T, want *ast.DataStep", prog.Steps[0])
	}
	lib := table.NewLibrary()
	if err := RunDataStep(ds, lib); err != nil {
		t.Fatalf("RunDataStep error: %v", err)
	}
	return lib
}

func TestDataStepSingleRowAssignments(t *testing.T) {
	lib := runStep(t, `data out; x = 2 + 3; y = x * 10; name = 'hi'; run;`)
	ds, ok := lib.Get("out")
	if !ok {
		t.Fatal("dataset OUT not created")
	}
	if ds.NObs() != 1 {
		t.Fatalf("NObs = %d, want 1", ds.NObs())
	}
	r := ds.Rows[0]
	if got := ds.Get(r, "x"); got.Num != 5 {
		t.Errorf("x = %v, want 5", got.Display())
	}
	if got := ds.Get(r, "y"); got.Num != 50 {
		t.Errorf("y = %v, want 50", got.Display())
	}
	if got := ds.Get(r, "name"); got.Str != "hi" {
		t.Errorf("name = %q, want hi", got.Str)
	}
}

func TestDataStepColumnOrder(t *testing.T) {
	lib := runStep(t, `data out; a = 1; b = 2; c = 3; run;`)
	ds, _ := lib.Get("out")
	want := []string{"a", "b", "c"}
	got := ds.ColumnNames()
	if len(got) != len(want) {
		t.Fatalf("columns = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("column[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDataStepAutomaticVarsNotOutput(t *testing.T) {
	lib := runStep(t, `data out; x = _n_; run;`)
	ds, _ := lib.Get("out")
	if ds.HasColumn("_n_") || ds.HasColumn("_error_") {
		t.Errorf("automatic variables should not be output; columns = %v", ds.ColumnNames())
	}
	// _N_ should still be readable as 1 during the single iteration.
	if got := ds.Get(ds.Rows[0], "x"); got.Num != 1 {
		t.Errorf("x (= _n_) = %v, want 1", got.Display())
	}
}

func TestDataStepExplicitOutputSuppressesImplicit(t *testing.T) {
	// With an explicit output, only the explicitly-output rows appear.
	lib := runStep(t, `data out; x = 1; output; x = 2; output; run;`)
	ds, _ := lib.Get("out")
	if ds.NObs() != 2 {
		t.Fatalf("NObs = %d, want 2 (two explicit outputs)", ds.NObs())
	}
	if got := ds.Get(ds.Rows[0], "x"); got.Num != 1 {
		t.Errorf("row0 x = %v, want 1", got.Display())
	}
	if got := ds.Get(ds.Rows[1], "x"); got.Num != 2 {
		t.Errorf("row1 x = %v, want 2", got.Display())
	}
}

func TestDataStepNoExplicitOutputImplicitOnce(t *testing.T) {
	lib := runStep(t, `data out; x = 99; run;`)
	ds, _ := lib.Get("out")
	if ds.NObs() != 1 {
		t.Errorf("NObs = %d, want 1 (implicit output once)", ds.NObs())
	}
}

func TestDataStepDefaultDatasetName(t *testing.T) {
	lib := runStep(t, `data; x = 1; run;`)
	if !lib.Has("DATA1") {
		t.Errorf("unnamed step should write DATA1; have %v", lib.Names())
	}
}
