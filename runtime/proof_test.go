package runtime

import (
	"bytes"
	"strings"
	"testing"

	"github.com/solifugus/ass/log"
	"github.com/solifugus/ass/parser"
	"github.com/solifugus/ass/table"
)

// TestProofReportRichHTML drives a proof under a rich sink and checks the HTML
// panel carries PASS/FAIL pills, the dataset name, and the offending-obs detail.
func TestProofReportRichHTML(t *testing.T) {
	src := `
data orders;
  input id qty;
  datalines;
1 5
2 0
3 7
;
run;
proc proof data=orders;
  notnull id qty;
  range qty 1 - 1000;
run;`
	prog := parser.New(src).ParseProgram()
	lib := table.NewLibrary()
	var panel string
	logger := log.NewSink(func(ev log.Event) {
		if ev.Kind == "table" && ev.HTML != "" {
			panel += ev.HTML
		}
	})
	if err := RunProgram(prog, lib, logger); err != nil {
		t.Fatalf("RunProgram: %v", err)
	}
	for _, want := range []string{
		"PROC PROOF", "WORK.ORDERS",
		">PASS</span>", ">FAIL</span>", // colored verdict pills
		"offending obs: 2", // qty=0 at obs 2 breaks the range
	} {
		if !strings.Contains(panel, want) {
			t.Errorf("proof panel missing %q; got:\n%s", want, panel)
		}
	}
}

// runProof parses and runs src, returning the library and the logger's error
// count (which drives the CLI's non-zero exit on an error-level proof failure).
func runProofProg(t *testing.T, src string) (*table.Library, int) {
	t.Helper()
	prog := parser.New(src).ParseProgram()
	lib := table.NewLibrary()
	var b bytes.Buffer
	logger := log.New(&b)
	if err := RunProgram(prog, lib, logger); err != nil {
		t.Fatalf("RunProgram: %v\nlog:\n%s", err, b.String())
	}
	return lib, logger.ErrorCount()
}

func TestProofViolationsOutAndExit(t *testing.T) {
	src := `
data orders;
  input id qty shipdate orderdate;
  datalines;
1 5 100 90
2 0 100 110
3 7 100 100
;
run;
proc proof data=orders out=bad;
  require id qty;
  notnull qty;
  range qty 1 - 1000;
  unique id;
  rule "ship after order": shipdate >= orderdate;
run;`
	lib, errs := runProofProg(t, src)
	if errs == 0 {
		t.Fatalf("expected a non-zero error count on error-level failures")
	}
	bad, ok := lib.Get("bad")
	if !ok {
		t.Fatal("out= dataset BAD not created")
	}
	// Only obs 2 violates: range (qty=0) and rule (ship<order) -> 2 records.
	if bad.NObs() != 2 {
		t.Fatalf("BAD nobs = %d, want 2", bad.NObs())
	}
	for _, r := range bad.Rows {
		if got := bad.Get(r, "_obs_"); got.Num != 2 {
			t.Errorf("_obs_ = %v, want 2", got.Num)
		}
	}
	rules := map[string]bool{}
	for _, r := range bad.Rows {
		rules[bad.Get(r, "_rule_").Str] = true
	}
	if !rules["range qty 1 - 1000"] || !rules["ship after order"] {
		t.Errorf("_rule_ values = %v, want range + ship-after-order", rules)
	}
}

func TestProofAllPass(t *testing.T) {
	src := `
data t; input x; datalines;
1
2
;
run;
proc proof data=t out=bad;
  notnull x;
  range x 0 - 10;
run;`
	lib, errs := runProofProg(t, src)
	if errs != 0 {
		t.Errorf("error count = %d, want 0 (all assertions pass)", errs)
	}
	bad, ok := lib.Get("bad")
	if !ok {
		t.Fatal("out= dataset BAD not created")
	}
	if bad.NObs() != 0 {
		t.Errorf("BAD nobs = %d, want 0", bad.NObs())
	}
}

func TestProofWarnDoesNotFailExit(t *testing.T) {
	src := `
data t; input x; datalines;
1
.
;
run;
proc proof data=t;
  notnull x / severity=warn;
run;`
	_, errs := runProofProg(t, src)
	if errs != 0 {
		t.Errorf("warn-level failure raised error count %d, want 0", errs)
	}
}

func TestProofUniqueFlagsAllDuplicates(t *testing.T) {
	src := `
data t; input id; datalines;
1
2
2
;
run;
proc proof data=t out=bad;
  unique id;
run;`
	lib, errs := runProofProg(t, src)
	if errs == 0 {
		t.Error("expected non-zero error count for duplicate key")
	}
	bad, _ := lib.Get("bad")
	if bad == nil || bad.NObs() != 2 {
		t.Fatalf("BAD nobs = %v, want 2 (both rows sharing id=2)", bad)
	}
}

func TestProofValuesAndRange(t *testing.T) {
	src := `
data t; input id state $; datalines;
1 CA
2 NY
3 ZZ
;
run;
proc proof data=t out=bad;
  values state in ("CA" "NY" "TX");
run;`
	lib, errs := runProofProg(t, src)
	if errs == 0 {
		t.Error("expected failure: ZZ not in allowed set")
	}
	bad, _ := lib.Get("bad")
	if bad.NObs() != 1 {
		t.Fatalf("BAD nobs = %d, want 1", bad.NObs())
	}
	if got := bad.Get(bad.Rows[0], "state").Str; got != "ZZ" {
		t.Errorf("offending state = %q, want ZZ", got)
	}
}

func TestProofTypeMismatch(t *testing.T) {
	// id is numeric, name is character. Declaring the opposite types fails.
	src := `
data t; input id name $; datalines;
1 a
;
run;
proc proof data=t;
  type id=num name=char;
run;`
	_, errsOK := runProofProg(t, src)
	if errsOK != 0 {
		t.Errorf("correct type declarations failed: error count %d", errsOK)
	}

	bad := `
data t; input id name $; datalines;
1 a
;
run;
proc proof data=t;
  type id=char name=num;
run;`
	_, errs := runProofProg(t, bad)
	if errs == 0 {
		t.Error("expected type mismatch to fail (id is num, name is char)")
	}
}

func TestProofKeyReferences(t *testing.T) {
	src := `
data regions; input region $; datalines;
east
west
;
run;
data orders; input id region $; datalines;
1 east
2 south
3 west
;
run;
proc proof data=orders out=bad;
  key region references regions(region);
run;`
	lib, errs := runProofProg(t, src)
	if errs == 0 {
		t.Error("expected key violation: 'south' has no parent region")
	}
	bad, _ := lib.Get("bad")
	if bad.NObs() != 1 {
		t.Fatalf("BAD nobs = %d, want 1", bad.NObs())
	}
	if got := bad.Get(bad.Rows[0], "region").Str; got != "south" {
		t.Errorf("offending region = %q, want south", got)
	}
}

func TestProofKeyMissingParentCannotRun(t *testing.T) {
	src := `
data orders; input id region $; datalines;
1 east
;
run;
proc proof data=orders;
  key region references nosuchtable(region);
run;`
	_, errs := runProofProg(t, src)
	if errs != 0 {
		t.Errorf("missing parent table should be 'could not run', not a failure: %d", errs)
	}
}

func TestProofRangeRelational(t *testing.T) {
	src := `
data t; input x; datalines;
5
-3
0
;
run;
proc proof data=t out=bad;
  range x >= 0;
run;`
	lib, errs := runProofProg(t, src)
	if errs == 0 {
		t.Error("expected failure: x=-3 violates range x >= 0")
	}
	bad, _ := lib.Get("bad")
	if bad.NObs() != 1 {
		t.Fatalf("BAD nobs = %d, want 1", bad.NObs())
	}
	if got := bad.Get(bad.Rows[0], "x").Num; got != -3 {
		t.Errorf("offending x = %v, want -3", got)
	}
}

func TestProofRuleDivisionAndSeverityTail(t *testing.T) {
	// The rule expression uses `/` as division and also carries a `/ severity=`
	// tail; the two must not collide. obs1 (10/2=5, not <3) violates at warn level,
	// so the exit code is unaffected.
	src := `
data t; input a b; datalines;
10 2
3 4
;
run;
proc proof data=t;
  rule "ratio under 3": a / b < 3 / severity=warn;
run;`
	_, errs := runProofProg(t, src)
	if errs != 0 {
		t.Errorf("warn-level rule failure raised error count %d, want 0", errs)
	}
}

func TestProofCompositeKey(t *testing.T) {
	src := `
data dim; input region $ yr; datalines;
east 2024
west 2024
;
run;
data f; input region $ yr; datalines;
east 2024
west 2025
;
run;
proc proof data=f out=bad;
  key region yr references dim(region yr);
run;`
	lib, errs := runProofProg(t, src)
	if errs == 0 {
		t.Error("expected composite-key violation: (west,2025) absent from dim")
	}
	bad, _ := lib.Get("bad")
	if bad.NObs() != 1 {
		t.Fatalf("BAD nobs = %d, want 1", bad.NObs())
	}
	if got := bad.Get(bad.Rows[0], "yr").Num; got != 2025 {
		t.Errorf("offending yr = %v, want 2025", got)
	}
}

func TestProofMissingColumnCannotRun(t *testing.T) {
	src := `
data t; input x; datalines;
1
;
run;
proc proof data=t;
  notnull nosuchcol;
run;`
	// A reference to an unknown column should not crash; it is reported as
	// "could not run" and does not fail the exit.
	_, errs := runProofProg(t, src)
	if errs != 0 {
		t.Errorf("unknown-column assertion raised error count %d, want 0", errs)
	}
}
