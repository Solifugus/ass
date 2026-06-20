package proc_test

import (
	"strings"
	"testing"

	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/parser"
	"github.com/solifugus/ass/runtime"
	"github.com/solifugus/ass/table"
)

// TestAppendToExistingBase appends DATA= rows onto an existing BASE= dataset in
// WORK, in order, preserving BASE's columns.
func TestAppendToExistingBase(t *testing.T) {
	src := "data base;\n  input id name $;\n  datalines;\n1 Alice\n2 Bob\n;\nrun;\n" +
		"data more;\n  input id name $;\n  datalines;\n3 Carol\n4 Dave\n;\nrun;\n" +
		"proc append base=base data=more; run;"
	lib := runSrc(t, src)
	ds, ok := lib.Get("base")
	if !ok {
		t.Fatal("BASE not found")
	}
	if ds.NObs() != 4 {
		t.Fatalf("base nobs = %d, want 4", ds.NObs())
	}
	if got := names(ds, "name"); !eq(got, []string{"Alice", "Bob", "Carol", "Dave"}) {
		t.Errorf("names = %v, want [Alice Bob Carol Dave]", got)
	}
}

// TestAppendCreatesMissingBase creates BASE= from DATA= when it does not yet
// exist (the first append in an incremental load).
func TestAppendCreatesMissingBase(t *testing.T) {
	src := "data more;\n  input id name $;\n  datalines;\n1 Alice\n2 Bob\n;\nrun;\n" +
		"proc append base=base data=more; run;"
	lib := runSrc(t, src)
	ds, ok := lib.Get("base")
	if !ok {
		t.Fatal("BASE not created")
	}
	if ds.NObs() != 2 {
		t.Fatalf("base nobs = %d, want 2", ds.NObs())
	}
	if got := names(ds, "name"); !eq(got, []string{"Alice", "Bob"}) {
		t.Errorf("names = %v, want [Alice Bob]", got)
	}
}

// TestAppendExtraVarRefusedWithoutForce refuses to append when DATA= has a
// variable BASE= lacks, unless FORCE is given.
func TestAppendExtraVarRefusedWithoutForce(t *testing.T) {
	base := "data base;\n  input id name $;\n  datalines;\n1 Alice\n;\nrun;\n"
	more := "data more;\n  input id name $ extra;\n  datalines;\n2 Bob 99\n;\nrun;\n"

	// Without FORCE: refused, BASE unchanged.
	lib := runSrc(t, base+more+"proc append base=base data=more; run;")
	ds, _ := lib.Get("base")
	if ds.NObs() != 1 {
		t.Fatalf("without force: base nobs = %d, want 1 (refused)", ds.NObs())
	}

	// With FORCE: appended, the extra variable dropped.
	lib2 := runSrc(t, base+more+"proc append base=base data=more force; run;")
	ds2, _ := lib2.Get("base")
	if ds2.NObs() != 2 {
		t.Fatalf("with force: base nobs = %d, want 2", ds2.NObs())
	}
	if ds2.HasColumn("extra") {
		t.Error("extra variable should have been dropped under FORCE")
	}
	if got := names(ds2, "name"); !eq(got, []string{"Alice", "Bob"}) {
		t.Errorf("names = %v, want [Alice Bob]", got)
	}
}

// TestAppendBaseOnlyVarFilledMissing appends rows where DATA= lacks a variable
// BASE= has: those values become missing, no FORCE required.
func TestAppendBaseOnlyVarFilledMissing(t *testing.T) {
	src := "data base;\n  input id name $ age;\n  datalines;\n1 Alice 30\n;\nrun;\n" +
		"data more;\n  input id name $;\n  datalines;\n2 Bob\n;\nrun;\n" +
		"proc append base=base data=more; run;"
	lib := runSrc(t, src)
	ds, _ := lib.Get("base")
	if ds.NObs() != 2 {
		t.Fatalf("base nobs = %d, want 2", ds.NObs())
	}
	if v := ds.Get(ds.Rows[1], "age"); !v.IsMissing() {
		t.Errorf("Bob age = %v, want missing", v.Display())
	}
}

// TestAppendMissingDataLogsError reports a clear error (and does not panic) when
// DATA= does not exist.
func TestAppendMissingDataLogsError(t *testing.T) {
	src := "data base;\n  input id;\n  datalines;\n1\n;\nrun;\n" +
		"proc append base=base data=nope; run;"
	prog := parser.New(src).ParseProgram()
	lib := table.NewLibrary()
	var b strings.Builder
	if err := runtime.RunProgram(prog, lib, log.New(&b)); err != nil {
		t.Fatalf("RunProgram error: %v", err)
	}
	if !strings.Contains(b.String(), "not found") {
		t.Errorf("expected a not-found error in log; got:\n%s", b.String())
	}
}
