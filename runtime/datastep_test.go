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
	if err := RunDataStep(ds, lib, nil); err != nil {
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

// runProgram runs every DATA step in the program against one library.
func runProgram(t *testing.T, src string) *table.Library {
	t.Helper()
	prog := parser.New(src).ParseProgram()
	lib := table.NewLibrary()
	for _, step := range prog.Steps {
		ds, ok := step.(*ast.DataStep)
		if !ok {
			continue
		}
		if err := RunDataStep(ds, lib, nil); err != nil {
			t.Fatalf("RunDataStep error: %v", err)
		}
	}
	return lib
}

func TestDataStepSetCopy(t *testing.T) {
	src := "data a;\n  input name $ age;\n  datalines;\nJohn 25\nJane 30\n;\nrun;\n" +
		"data b;\n  set a;\n  agePlus = age + 1;\n  run;"
	lib := runProgram(t, src)
	b, ok := lib.Get("b")
	if !ok {
		t.Fatal("dataset B not created")
	}
	if b.NObs() != 2 {
		t.Fatalf("B NObs = %d, want 2", b.NObs())
	}
	// SET variables come first, in source order; computed column follows.
	want := []string{"name", "age", "agePlus"}
	if got := b.ColumnNames(); len(got) != 3 || got[0] != want[0] || got[2] != want[2] {
		t.Fatalf("columns = %v, want %v", got, want)
	}
	if got := b.Get(b.Rows[1], "agePlus"); got.Num != 31 {
		t.Errorf("row1 agePlus = %v, want 31", got.Display())
	}
	if got := b.Get(b.Rows[0], "name"); got.Str != "John" {
		t.Errorf("row0 name = %q, want John", got.Str)
	}
}

func TestDataStepSetConcatenates(t *testing.T) {
	src := "data a;\n  input x;\n  datalines;\n1\n2\n;\nrun;\n" +
		"data c;\n  input x;\n  datalines;\n3\n;\nrun;\n" +
		"data all;\n  set a c;\n  run;"
	lib := runProgram(t, src)
	all, _ := lib.Get("all")
	if all.NObs() != 3 {
		t.Fatalf("ALL NObs = %d, want 3 (2 + 1)", all.NObs())
	}
	got := []float64{
		all.Get(all.Rows[0], "x").Num,
		all.Get(all.Rows[1], "x").Num,
		all.Get(all.Rows[2], "x").Num,
	}
	want := []float64{1, 2, 3}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row%d x = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestDataStepInputDatalines(t *testing.T) {
	src := "data people;\n  input name $ age;\n  datalines;\nJohn 25\nJane 30\nBob 40\n;\nrun;"
	lib := runStep(t, src)
	ds, ok := lib.Get("people")
	if !ok {
		t.Fatal("dataset PEOPLE not created")
	}
	if ds.NObs() != 3 {
		t.Fatalf("NObs = %d, want 3", ds.NObs())
	}
	want := []string{"name", "age"}
	if got := ds.ColumnNames(); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("columns = %v, want %v", got, want)
	}
	if got := ds.Get(ds.Rows[0], "name"); got.Str != "John" {
		t.Errorf("row0 name = %q, want John", got.Str)
	}
	if got := ds.Get(ds.Rows[1], "age"); got.Num != 30 {
		t.Errorf("row1 age = %v, want 30", got.Display())
	}
	// Type check: age is numeric.
	for _, c := range ds.Columns {
		if c.Name == "age" && c.Kind != table.Numeric {
			t.Error("age should be numeric")
		}
	}
}

func TestDataStepInputWithComputedColumn(t *testing.T) {
	src := "data out;\n  input x;\n  y = x * 2;\n  datalines;\n1\n2\n3\n;\nrun;"
	lib := runStep(t, src)
	ds, _ := lib.Get("out")
	if ds.NObs() != 3 {
		t.Fatalf("NObs = %d, want 3", ds.NObs())
	}
	if got := ds.Get(ds.Rows[2], "y"); got.Num != 6 {
		t.Errorf("row2 y = %v, want 6", got.Display())
	}
}

func TestDataStepInputMissingField(t *testing.T) {
	// Second record omits age -> numeric missing.
	src := "data people;\n  input name $ age;\n  datalines;\nJohn 25\nJane\n;\nrun;"
	lib := runStep(t, src)
	ds, _ := lib.Get("people")
	if got := ds.Get(ds.Rows[1], "age"); !got.IsMissing() {
		t.Errorf("row1 age = %v, want missing", got.Display())
	}
}

func TestDataStepSubsettingIf(t *testing.T) {
	src := "data adults;\n  input name $ age;\n  if age >= 18;\n  datalines;\nJohn 25\nKid 10\nJane 30\n;\nrun;"
	lib := runStep(t, src)
	ds, _ := lib.Get("adults")
	if ds.NObs() != 2 {
		t.Fatalf("NObs = %d, want 2 (only adults)", ds.NObs())
	}
	for _, r := range ds.Rows {
		if ds.Get(r, "age").Num < 18 {
			t.Errorf("row with age %v should have been dropped", ds.Get(r, "age").Display())
		}
	}
}

func TestDataStepIfThenElse(t *testing.T) {
	src := "data out;\n  input score;\n  if score >= 60 then grade = 'P';\n  else grade = 'F';\n  datalines;\n75\n40\n60\n;\nrun;"
	lib := runStep(t, src)
	ds, _ := lib.Get("out")
	want := []string{"P", "F", "P"}
	if ds.NObs() != 3 {
		t.Fatalf("NObs = %d, want 3", ds.NObs())
	}
	for i, w := range want {
		if got := ds.Get(ds.Rows[i], "grade"); got.Str != w {
			t.Errorf("row%d grade = %q, want %q", i, got.Str, w)
		}
	}
}

func TestDataStepIfThenOutput(t *testing.T) {
	// Explicit output inside a THEN: only matching rows are written.
	src := "data big;\n  input x;\n  if x > 5 then output;\n  datalines;\n3\n7\n9\n1\n;\nrun;"
	lib := runStep(t, src)
	ds, _ := lib.Get("big")
	if ds.NObs() != 2 {
		t.Fatalf("NObs = %d, want 2 (x > 5)", ds.NObs())
	}
	if ds.Get(ds.Rows[0], "x").Num != 7 || ds.Get(ds.Rows[1], "x").Num != 9 {
		t.Errorf("rows = %v %v, want 7 9", ds.Get(ds.Rows[0], "x").Display(), ds.Get(ds.Rows[1], "x").Display())
	}
}

func TestDataStepDoIterativeOutput(t *testing.T) {
	src := "data squares;\n  do i = 1 to 5;\n    sq = i * i;\n    output;\n  end;\n  run;"
	lib := runStep(t, src)
	ds, _ := lib.Get("squares")
	if ds.NObs() != 5 {
		t.Fatalf("NObs = %d, want 5", ds.NObs())
	}
	if got := ds.Get(ds.Rows[2], "sq"); got.Num != 9 {
		t.Errorf("row2 sq = %v, want 9", got.Display())
	}
	if got := ds.Get(ds.Rows[4], "i"); got.Num != 5 {
		t.Errorf("row4 i = %v, want 5", got.Display())
	}
}

func TestDataStepDoIterativeBy(t *testing.T) {
	src := "data evens;\n  do n = 0 to 10 by 2;\n    output;\n  end;\n  run;"
	lib := runStep(t, src)
	ds, _ := lib.Get("evens")
	if ds.NObs() != 6 { // 0,2,4,6,8,10
		t.Fatalf("NObs = %d, want 6", ds.NObs())
	}
	if got := ds.Get(ds.Rows[5], "n"); got.Num != 10 {
		t.Errorf("last n = %v, want 10", got.Display())
	}
}

func TestDataStepDoWhile(t *testing.T) {
	src := "data g;\n  i = 1;\n  do while(i <= 3);\n    output;\n    i = i + 1;\n  end;\n  run;"
	lib := runStep(t, src)
	ds, _ := lib.Get("g")
	if ds.NObs() != 3 {
		t.Fatalf("NObs = %d, want 3", ds.NObs())
	}
}

func TestDataStepDoUntilRunsAtLeastOnce(t *testing.T) {
	src := "data g;\n  i = 10;\n  do until(i >= 3);\n    output;\n    i = i + 1;\n  end;\n  run;"
	lib := runStep(t, src)
	ds, _ := lib.Get("g")
	if ds.NObs() != 1 {
		t.Fatalf("NObs = %d, want 1 (until tests after body)", ds.NObs())
	}
}

func TestDataStepDrop(t *testing.T) {
	src := "data out;\n  input x y z;\n  drop y;\n  datalines;\n1 2 3\n;\nrun;"
	lib := runStep(t, src)
	ds, _ := lib.Get("out")
	if ds.HasColumn("y") {
		t.Errorf("y should be dropped; columns = %v", ds.ColumnNames())
	}
	if !ds.HasColumn("x") || !ds.HasColumn("z") {
		t.Errorf("x and z should remain; columns = %v", ds.ColumnNames())
	}
}

func TestDataStepKeep(t *testing.T) {
	src := "data out;\n  input x y z;\n  keep x z;\n  datalines;\n1 2 3\n;\nrun;"
	lib := runStep(t, src)
	ds, _ := lib.Get("out")
	want := []string{"x", "z"}
	got := ds.ColumnNames()
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("columns = %v, want %v", got, want)
	}
}

func TestDataStepWhereOnInput(t *testing.T) {
	src := "data adults;\n  input name $ age;\n  where age >= 18;\n  datalines;\nJohn 25\nKid 10\nJane 30\n;\nrun;"
	lib := runStep(t, src)
	ds, _ := lib.Get("adults")
	if ds.NObs() != 2 {
		t.Fatalf("NObs = %d, want 2 (where age>=18)", ds.NObs())
	}
	if got := names(ds, "name"); !eqStr(got, []string{"John", "Jane"}) {
		t.Errorf("names = %v, want [John Jane]", got)
	}
}

func TestDataStepWhereOnSet(t *testing.T) {
	src := "data people;\n  input name $ age;\n  datalines;\nJohn 25\nKid 10\nJane 30\n;\nrun;\n" +
		"data adults;\n  set people;\n  where age > 20;\n  run;"
	lib := runProgram(t, src)
	ds, _ := lib.Get("adults")
	if ds.NObs() != 2 {
		t.Fatalf("NObs = %d, want 2", ds.NObs())
	}
	if got := names(ds, "name"); !eqStr(got, []string{"John", "Jane"}) {
		t.Errorf("names = %v, want [John Jane]", got)
	}
}

// names/eqStr are small local helpers mirroring the proc tests.
func names(ds *table.Dataset, col string) []string {
	out := make([]string, ds.NObs())
	for i, r := range ds.Rows {
		out[i] = ds.Get(r, col).Display()
	}
	return out
}

func eqStr(a, b []string) bool {
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

func TestDataStepRetainSum(t *testing.T) {
	src := "data t;\n  retain total 0;\n  input x;\n  total + x;\n  datalines;\n10\n20\n30\n;\nrun;"
	lib := runStep(t, src)
	ds, _ := lib.Get("t")
	if ds.NObs() != 3 {
		t.Fatalf("NObs = %d, want 3", ds.NObs())
	}
	wantTotal := []float64{10, 30, 60}
	wantX := []float64{10, 20, 30}
	for i := range wantTotal {
		if got := ds.Get(ds.Rows[i], "total"); got.Num != wantTotal[i] {
			t.Errorf("row%d total = %v, want %v", i, got.Display(), wantTotal[i])
		}
		if got := ds.Get(ds.Rows[i], "x"); got.Num != wantX[i] {
			t.Errorf("row%d x = %v, want %v", i, got.Display(), wantX[i])
		}
	}
}

func TestDataStepRetainCarriesValue(t *testing.T) {
	// Without retain, a variable not reassigned resets to missing each iteration.
	// With retain, it carries over (here: remember the previous x).
	src := "data t;\n  retain prev;\n  input x;\n  lag = prev;\n  prev = x;\n  datalines;\n5\n8\n2\n;\nrun;"
	lib := runStep(t, src)
	ds, _ := lib.Get("t")
	wantLag := []table.Value{table.MissingNum(), table.Num(5), table.Num(8)}
	for i, w := range wantLag {
		got := ds.Get(ds.Rows[i], "lag")
		if w.IsMissing() {
			if !got.IsMissing() {
				t.Errorf("row%d lag = %v, want missing", i, got.Display())
			}
		} else if got.Num != w.Num {
			t.Errorf("row%d lag = %v, want %v", i, got.Display(), w.Num)
		}
	}
}

func TestDataStepArray(t *testing.T) {
	src := "data t;\n  array s{3} s1 s2 s3;\n  input s1 s2 s3;\n  do i = 1 to 3;\n    s{i} = s{i} * 2;\n  end;\n  drop i;\n  datalines;\n1 2 3\n;\nrun;"
	lib := runStep(t, src)
	ds, _ := lib.Get("t")
	if ds.NObs() != 1 {
		t.Fatalf("NObs = %d, want 1", ds.NObs())
	}
	want := map[string]float64{"s1": 2, "s2": 4, "s3": 6}
	for k, v := range want {
		if got := ds.Get(ds.Rows[0], k); got.Num != v {
			t.Errorf("%s = %v, want %v", k, got.Display(), v)
		}
	}
	if ds.HasColumn("i") {
		t.Errorf("loop var i should be dropped; columns = %v", ds.ColumnNames())
	}
}

func TestDataStepArrayRangeExpansion(t *testing.T) {
	// array x{3} x1-x3; the range expands to x1 x2 x3.
	src := "data t;\n  array x{3} x1-x3;\n  input x1 x2 x3;\n  total = x{1} + x{2} + x{3};\n  datalines;\n4 5 6\n;\nrun;"
	lib := runStep(t, src)
	ds, _ := lib.Get("t")
	if got := ds.Get(ds.Rows[0], "total"); got.Num != 15 {
		t.Errorf("total = %v, want 15", got.Display())
	}
}

func TestDataStepByGroupAggregation(t *testing.T) {
	src := "data sales;\n  input region $ amount;\n  datalines;\neast 10\neast 20\nwest 30\nwest 40\n;\nrun;\n" +
		"data totals;\n  set sales;\n  by region;\n  retain total;\n" +
		"  if first.region then total = 0;\n  total + amount;\n" +
		"  if last.region then output;\n  keep region total;\nrun;"
	lib := runProgram(t, src)
	ds, ok := lib.Get("totals")
	if !ok {
		t.Fatal("TOTALS not created")
	}
	if ds.NObs() != 2 {
		t.Fatalf("NObs = %d, want 2 (one per region)", ds.NObs())
	}
	got := map[string]float64{}
	for _, r := range ds.Rows {
		got[ds.Get(r, "region").Str] = ds.Get(r, "total").Num
	}
	if got["east"] != 30 || got["west"] != 70 {
		t.Errorf("group totals = %v, want east:30 west:70", got)
	}
	if ds.HasColumn("amount") {
		t.Errorf("amount should not be kept; columns = %v", ds.ColumnNames())
	}
}

func mergeInputs() string {
	return "data names;\n  input id name $;\n  datalines;\n1 John\n2 Mary\n3 Tim\n;\nrun;\n" +
		"data scores;\n  input id score;\n  datalines;\n1 95\n2 85\n4 70\n;\nrun;\n"
}

func TestDataStepMergeFull(t *testing.T) {
	src := mergeInputs() +
		"data m;\n  merge names(in=n) scores(in=s);\n  by id;\nrun;"
	lib := runProgram(t, src)
	ds, _ := lib.Get("m")
	if ds.NObs() != 4 { // ids 1,2,3,4
		t.Fatalf("NObs = %d, want 4", ds.NObs())
	}
	byID := map[float64]table.Row{}
	for _, r := range ds.Rows {
		byID[ds.Get(r, "id").Num] = r
	}
	if got := ds.Get(byID[1], "name"); got.Str != "John" {
		t.Errorf("id1 name = %q, want John", got.Str)
	}
	if got := ds.Get(byID[1], "score"); got.Num != 95 {
		t.Errorf("id1 score = %v, want 95", got.Display())
	}
	if got := ds.Get(byID[3], "score"); !got.IsMissing() {
		t.Errorf("id3 score = %v, want missing (not in scores)", got.Display())
	}
	if got := ds.Get(byID[4], "name"); !got.IsMissing() {
		t.Errorf("id4 name = %v, want missing (not in names)", got.Display())
	}
	if ds.HasColumn("n") || ds.HasColumn("s") {
		t.Errorf("in= flags should not be output; columns = %v", ds.ColumnNames())
	}
}

func TestDataStepMergeInnerJoin(t *testing.T) {
	src := mergeInputs() +
		"data both;\n  merge names(in=n) scores(in=s);\n  by id;\n  if n and s;\nrun;"
	lib := runProgram(t, src)
	ds, _ := lib.Get("both")
	if ds.NObs() != 2 { // only ids 1 and 2 are in both
		t.Fatalf("NObs = %d, want 2", ds.NObs())
	}
	for _, r := range ds.Rows {
		id := ds.Get(r, "id").Num
		if id != 1 && id != 2 {
			t.Errorf("unexpected id %v in inner join", id)
		}
	}
}

func TestDataStepMergeLeftJoin(t *testing.T) {
	src := mergeInputs() +
		"data left;\n  merge names(in=n) scores(in=s);\n  by id;\n  if n;\nrun;"
	lib := runProgram(t, src)
	ds, _ := lib.Get("left")
	if ds.NObs() != 3 { // ids 1,2,3 (all names)
		t.Fatalf("NObs = %d, want 3", ds.NObs())
	}
}

func TestDataStepDefaultDatasetName(t *testing.T) {
	lib := runStep(t, `data; x = 1; run;`)
	if !lib.Has("DATA1") {
		t.Errorf("unnamed step should write DATA1; have %v", lib.Names())
	}
}
